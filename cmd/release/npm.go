package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The npm channel publishes seven packages per release: the wrapper npmWrapperName, which installs
// the `photon` launcher, and one platform package per release target, each carrying that
// target's binary and limited by its os and cpu fields to the platform it runs on. The wrapper
// lists all six as optional dependencies at the same version, so npm installs only the one that
// matches the host.
const (
	npmScope        = "@airiclenz"
	npmWrapperName  = npmScope + "/" + binaryName
	npmManifestFile = "package.json"
	npmLauncherFile = "photon.js"
	npmBinDir       = "bin"
)

// The npm command the npm step and its smoke check run, the registry they publish to and install
// from, and the environment variable publish takes the registry token from.
const (
	npmCommand       = "npm"
	npmRegistryHost  = "registry.npmjs.org"
	npmRegistryURL   = "https://" + npmRegistryHost + "/"
	npmTokenVariable = "NPM_TOKEN"
)

// npmWorkDirPrefix names the temporary directory the npm step downloads and stages a release in.
const npmWorkDirPrefix = "photon-npm-"

// npmInstallHint names where npm comes from, for the step that finds it missing.
const npmInstallHint = "install Node.js, which ships npm (https://nodejs.org)"

// npmLauncherDir is where `npm install --prefix <dir>` links a package's bin entries, under dir.
var npmLauncherDir = filepath.Join("node_modules", ".bin")

// npmOS and npmCPU translate a release target's GOOS and GOARCH to the `process.platform` and
// `process.arch` values npm's os and cpu fields, and the launcher, name it by.
var (
	npmOS  = map[string]string{"linux": "linux", "darwin": "darwin", "windows": "win32"}
	npmCPU = map[string]string{"amd64": "x64", "arm64": "arm64"}
)

// errNpmArchiveMismatch is a release archive whose hash differs from the one SHA256SUMS lists.
var errNpmArchiveMismatch = errors.New("archive does not match its SHA256SUMS hash")

// npmPlatform is the platform package of one release target, as the npm templates name it.
type npmPlatform struct {
	// Name is the package name, @airiclenz/photon-<os>-<cpu>.
	Name string
	// OS is the package's npm os value, a `process.platform` name.
	OS string
	// CPU is the package's npm cpu value, a `process.arch` name.
	CPU string
	// Binary is the binary's file name inside the package's bin directory.
	Binary string
	target distTarget
}

// npmTemplateData is what the npm templates render from.
type npmTemplateData struct {
	// Version is the bare release version, X.Y.Z, every package is published at.
	Version string
	// Wrapper is the wrapper package's name.
	Wrapper string
	// Platforms are the six platform packages, in distTargets' order.
	Platforms []npmPlatform
	// Platform is the platform package being rendered; only the platform template reads it.
	Platform npmPlatform
}

// npmPackage is one staged npm package: its name and the directory holding it, ready for
// `npm publish`.
type npmPackage struct {
	name string
	dir  string
}

// npmPlatforms returns the platform packages of the release targets, in distTargets' order.
func npmPlatforms() []npmPlatform {
	platforms := make([]npmPlatform, 0, len(distTargets))
	for _, target := range distTargets {
		osName, cpu := npmOS[target.goos], npmCPU[target.goarch]
		platforms = append(platforms, npmPlatform{
			Name:   npmWrapperName + "-" + osName + "-" + cpu,
			OS:     osName,
			CPU:    cpu,
			Binary: target.binaryFile(),
			target: target,
		})
	}
	return platforms
}

// stageNpmPackages stages the seven npm packages of version (vX.Y.Z or a bare X.Y.Z) as
// directories under outDir, from archiveDir holding the release's six archives and sums, the
// release's SHA256SUMS body. Every archive is checked against its listed hash before anything is
// written; a mismatch, a hash SHA256SUMS omits or an archive missing its binary refuses the whole
// stage. Each platform package gets its manifest and the binary extracted from its archive; the
// wrapper gets its manifest and the launcher. It neither builds nor downloads anything, and
// returns the packages in the order they publish in: the six platform packages first, then the
// wrapper that depends on them.
func stageNpmPackages(version, archiveDir, sums, outDir string) ([]npmPackage, error) {
	parsed, err := parseReleaseVersion(version)
	if err != nil {
		return nil, err
	}
	data := npmTemplateData{
		Version:   bareRelease(parsed),
		Wrapper:   npmWrapperName,
		Platforms: npmPlatforms(),
	}
	if err := verifyNpmArchives(data.Version, archiveDir, sums); err != nil {
		return nil, err
	}

	staged := make([]npmPackage, 0, len(data.Platforms)+1)
	for _, platform := range data.Platforms {
		data.Platform = platform
		stagedPlatform, err := stageNpmPlatform(data, archiveDir, outDir)
		if err != nil {
			return nil, err
		}
		staged = append(staged, stagedPlatform)
	}
	wrapper, err := stageNpmWrapper(data, outDir)
	if err != nil {
		return nil, err
	}
	return append(staged, wrapper), nil
}

// verifyNpmArchives checks each of the six archives of bareVersion in archiveDir against the hash
// sums lists for it.
func verifyNpmArchives(bareVersion, archiveDir, sums string) error {
	for _, target := range distTargets {
		name := target.archiveName(bareVersion)
		want, err := checksumFor(sums, name)
		if err != nil {
			return err
		}
		got, err := fileSHA256(filepath.Join(archiveDir, name))
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("%w: %s", errNpmArchiveMismatch, name)
		}
	}
	return nil
}

// stageNpmPlatform stages data.Platform's package: its manifest and its binary, extracted from
// the target's archive in archiveDir.
func stageNpmPlatform(data npmTemplateData, archiveDir, outDir string) (npmPackage, error) {
	platform := data.Platform
	staged := npmPackage{
		name: platform.Name,
		dir:  filepath.Join(outDir, npmPackageDir(platform.Name)),
	}
	if err := writeNpmTemplate(
		npmPlatformTemplate, data, staged.dir, npmManifestFile, documentMode,
	); err != nil {
		return npmPackage{}, err
	}

	binDir := filepath.Join(staged.dir, npmBinDir)
	if err := os.MkdirAll(binDir, directoryMode); err != nil {
		return npmPackage{}, fmt.Errorf("stage %s: %w", platform.Name, err)
	}
	archiveName := platform.target.archiveName(data.Version)
	entryName := path.Join(platform.target.stem(data.Version), platform.Binary)
	if err := extractNpmBinary(
		platform.target,
		filepath.Join(archiveDir, archiveName),
		entryName,
		filepath.Join(binDir, platform.Binary),
	); err != nil {
		return npmPackage{}, fmt.Errorf(
			"stage %s: read %s from %s: %w", platform.Name, entryName, archiveName, err,
		)
	}
	return staged, nil
}

// stageNpmWrapper stages the wrapper package: its manifest and the launcher.
func stageNpmWrapper(data npmTemplateData, outDir string) (npmPackage, error) {
	staged := npmPackage{
		name: npmWrapperName,
		dir:  filepath.Join(outDir, npmPackageDir(npmWrapperName)),
	}
	if err := writeNpmTemplate(
		npmWrapperTemplate, data, staged.dir, npmManifestFile, documentMode,
	); err != nil {
		return npmPackage{}, err
	}
	launcherDir := filepath.Join(staged.dir, npmBinDir)
	if err := writeNpmTemplate(
		npmLauncherTemplate, data, launcherDir, npmLauncherFile, binaryMode,
	); err != nil {
		return npmPackage{}, err
	}
	return staged, nil
}

// npmPackageDir is the directory a package is staged in: its name without the scope.
func npmPackageDir(name string) string {
	return path.Base(name)
}

// writeNpmTemplate renders the named template from data into dir/fileName with mode, creating
// dir.
func writeNpmTemplate(
	name string,
	data npmTemplateData,
	dir, fileName string,
	mode fs.FileMode,
) error {
	var rendered bytes.Buffer
	if err := packageTemplates.ExecuteTemplate(&rendered, name, data); err != nil {
		return fmt.Errorf("render %s for %s: %w", name, data.Version, err)
	}
	if err := os.MkdirAll(dir, directoryMode); err != nil {
		return fmt.Errorf("stage %s: %w", dir, err)
	}

	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, rendered.Bytes(), mode); err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}
	// WriteFile's mode is narrowed by the umask; the staged mode is what npm packs.
	if err := os.Chmod(filePath, mode); err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}
	return nil
}

// extractNpmBinary copies the entry entryName out of target's archive at archivePath, a .zip for
// Windows and a .tar.gz otherwise, to an executable file at destination.
func extractNpmBinary(target distTarget, archivePath, entryName, destination string) error {
	if target.isWindows() {
		return extractZipEntry(archivePath, entryName, destination)
	}
	return extractTarGzEntry(archivePath, entryName, destination)
}

// probeNpm reports npm done once the registry holds the wrapper at the version: publish
// publishes the wrapper last, so its presence means all seven packages are there, and the six
// platform packages are probed only while it is absent, to name the ones publish still has to
// publish. A host without npm is setup-pending; a registry read that fails otherwise is unknown.
func probeNpm(ctx context.Context, env probeEnv, version releaseVersion) stepReport {
	bare := bareRelease(version)
	isPublished, err := env.channels.HasPackage(ctx, npmWrapperName, bare)
	if err != nil {
		return npmProbeFailure(npmWrapperName, bare, err)
	}
	if isPublished {
		return stepReport{Name: stepNpm, State: stateDone, Detail: fmt.Sprintf(
			"published: %s@%s on %s", npmWrapperName, bare, npmRegistryHost)}
	}

	var missing []string
	for _, platform := range npmPlatforms() {
		isPublished, err := env.channels.HasPackage(ctx, platform.Name, bare)
		if err != nil {
			return npmProbeFailure(platform.Name, bare, err)
		}
		if !isPublished {
			missing = append(missing, platform.Name)
		}
	}
	return stepReport{Name: stepNpm, State: statePending, Detail: fmt.Sprintf(
		"%s lacks %s@%s and %d of its %d platform packages%s",
		npmRegistryHost, npmWrapperName, bare, len(missing), len(distTargets),
		npmListSuffix(missing))}
}

// npmListSuffix is ": " and the names, comma-separated, or nothing for none.
func npmListSuffix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return ": " + strings.Join(names, ", ")
}

// npmProbeFailure is the npm step whose registry read of name@bare failed with err:
// setup-pending when npm is not installed, unknown otherwise.
func npmProbeFailure(name, bare string, err error) stepReport {
	if isMissingTool(err) {
		return stepReport{Name: stepNpm, State: stateSetupPending,
			Detail: "npm is not installed — " + npmInstallHint}
	}
	return unknownStep(stepNpm, "could not read %s@%s from %s: %v",
		name, bare, npmRegistryHost, err)
}

// describeNpm says what the npm step does, with the npm command line it publishes each package
// with — the token shown redacted, as it never appears there.
func describeNpm(run *publishRun) string {
	return fmt.Sprintf("download the %s archives, stage the %d platform packages and %s at %s "+
		"and publish the ones %s lacks, the wrapper last: %s=%s %s publish <package> "+
		"--access public",
		run.version, len(distTargets), npmWrapperName, bareRelease(run.version), npmRegistryHost,
		npmTokenVariable, redactedToken, npmCommand)
}

// convergeNpm publishes the version's npm packages that the registry lacks — the six platform
// packages first, then the wrapper that depends on them — so a run interrupted part-way resumes
// with only the missing ones. It stages them from the release's archives, downloaded and checked
// against its SHA256SUMS. A missing NPM_TOKEN is a setup the maintainer has to do, so the step
// reports setup-pending, before anything is downloaded.
func convergeNpm(ctx context.Context, run *publishRun) error {
	token := strings.TrimSpace(run.options.getenv(npmTokenVariable))
	if token == "" {
		return setupPendingError{detail: fmt.Sprintf(
			"no npm token — set %s to an npm access token that may publish %s packages",
			npmTokenVariable, npmScope)}
	}
	workDir, err := os.MkdirTemp("", npmWorkDirPrefix)
	if err != nil {
		return fmt.Errorf("create the npm staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	packages, err := stageReleaseNpmPackages(ctx, run, workDir)
	if err != nil {
		return err
	}
	bare := bareRelease(run.version)
	for _, staged := range packages {
		isPublished, err := run.channels.HasPackage(ctx, staged.name, bare)
		if err != nil {
			return fmt.Errorf("could not tell whether %s has %s@%s: %w",
				npmRegistryHost, staged.name, bare, err)
		}
		if isPublished {
			_, _ = fmt.Fprintf(run.output, "  %s@%s is already on %s\n",
				staged.name, bare, npmRegistryHost)
			continue
		}
		err = run.channels.PublishPackage(ctx, staged.name, bare, staged.dir, token)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(run.output, "  published %s@%s\n", staged.name, bare)
	}
	return nil
}

// stageReleaseNpmPackages downloads the release's six archives into workDir and stages its seven
// npm packages there from them, refusing an archive that does not match the release's SHA256SUMS.
func stageReleaseNpmPackages(
	ctx context.Context,
	run *publishRun,
	workDir string,
) ([]npmPackage, error) {
	sums, err := downloadReleaseSums(ctx, run)
	if err != nil {
		return nil, err
	}
	archiveDir := filepath.Join(workDir, "archives")
	if err := os.MkdirAll(archiveDir, directoryMode); err != nil {
		return nil, fmt.Errorf("create the npm archive directory: %w", err)
	}
	tag := run.version.String()
	for _, target := range distTargets {
		name := target.archiveName(bareRelease(run.version))
		if err := run.repo.DownloadAsset(ctx, tag, name, archiveDir); err != nil {
			return nil, fmt.Errorf("download %s from GitHub release %s: %w", name, tag, err)
		}
	}
	return stageNpmPackages(tag, archiveDir, sums, filepath.Join(workDir, "packages"))
}

// checkNpm installs the wrapper at the version from the registry into a fresh prefix and requires
// the photon it installs to report the version. It skips while npm is missing or the registry
// does not hold that version yet: a release whose npm step is setup-pending is not a failure.
func (r *smokeRun) checkNpm(ctx context.Context) {
	bare := bareRelease(r.version)
	r.step("npm installs %s@%s, whose photon reports %s", npmWrapperName, bare, r.version)
	isPublished, err := r.channels.HasPackage(ctx, npmWrapperName, bare)
	switch {
	case isMissingTool(err):
		r.skip("npm is not installed on this host")
		return
	case err != nil:
		r.skip("could not read %s@%s from %s: %v", npmWrapperName, bare, npmRegistryHost, err)
		return
	case !isPublished:
		r.skip("%s@%s is not on %s", npmWrapperName, bare, npmRegistryHost)
		return
	}

	prefix := filepath.Join(r.workDir, "npm")
	if err := os.MkdirAll(prefix, directoryMode); err != nil {
		r.fail("create %s: %v", prefix, err)
		return
	}
	_, err = r.commands.run(ctx, r.workDir, nil, npmCommand,
		"install", "--prefix", prefix, npmWrapperName+"@"+bare)
	if err != nil {
		r.fail("npm install %s@%s: %v", npmWrapperName, bare, err)
		return
	}
	launcher := filepath.Join(prefix, npmLauncherDir, binaryName)
	if r.options.goos == "windows" {
		// npm links a bin entry on Windows as a .cmd shim, which runs the launcher through node.
		launcher += ".cmd"
	}
	r.checkReportsVersion(ctx, launcher, "npm's photon")
}
