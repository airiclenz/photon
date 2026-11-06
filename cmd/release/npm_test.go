package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// npmGoldenDir holds the staged npm text files for goldenVersion, one directory per package.
var npmGoldenDir = filepath.Join("testdata", "npm")

// npmWantPackages are the seven packages staging returns for any version, in publish order: the
// six platform packages in distTargets' order, then the wrapper.
var npmWantPackages = []string{
	"@airiclenz/photon-linux-x64",
	"@airiclenz/photon-linux-arm64",
	"@airiclenz/photon-darwin-x64",
	"@airiclenz/photon-darwin-arm64",
	"@airiclenz/photon-win32-x64",
	"@airiclenz/photon-win32-arm64",
	"@airiclenz/photon",
}

// npmFixtureRelease runs dist over a source tree at goldenVersion with the fake toolchain and
// returns the directory holding the six archives and the SHA256SUMS body dist wrote beside them.
func npmFixtureRelease(t *testing.T) (string, string) {
	t.Helper()
	archiveDir := runTestDist(t, fakeToolchain(), testSourceTreeAt(t, goldenVersion+"\n"))
	sums, err := os.ReadFile(filepath.Join(archiveDir, checksumsFile))
	if err != nil {
		t.Fatal(err)
	}
	return archiveDir, string(sums)
}

// stageNpmFixture stages goldenVersion's packages from a fixture release into a fresh directory
// and returns it with the staged packages.
func stageNpmFixture(t *testing.T, version string) (string, []npmPackage) {
	t.Helper()
	archiveDir, sums := npmFixtureRelease(t)
	outDir := filepath.Join(t.TempDir(), "npm")
	staged, err := stageNpmPackages(version, archiveDir, sums, outDir)
	if err != nil {
		t.Fatalf("stageNpmPackages(%q) = %v", version, err)
	}
	return outDir, staged
}

// npmStagedFiles lists every regular file under dir by slash-separated relative path.
func npmStagedFiles(t *testing.T, dir string) map[string]fs.FileMode {
	t.Helper()
	files := map[string]fs.FileMode{}
	err := filepath.WalkDir(dir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, filePath)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = info.Mode().Perm()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestStageNpmPackagesMatchesTheGoldenTrees stages goldenVersion, tagged and bare, and expects
// the seven package trees: each manifest and the launcher byte for byte as under testdata/npm,
// each platform package's binary extracted from its own archive, and nothing else.
func TestStageNpmPackagesMatchesTheGoldenTrees(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"v" + goldenVersion, goldenVersion} {
		outDir, _ := stageNpmFixture(t, version)

		wantFiles := map[string]fs.FileMode{
			"photon/package.json":  documentMode,
			"photon/bin/photon.js": binaryMode,
		}
		for _, target := range distTargets {
			platform := npmPlatformDir(target)
			wantFiles[platform+"/package.json"] = documentMode
			wantFiles[platform+"/bin/"+target.binaryFile()] = binaryMode
		}
		gotFiles := npmStagedFiles(t, outDir)
		if runtime.GOOS == "windows" {
			// Windows keeps no Unix permission bits, so only the file set is compared there.
			for name := range wantFiles {
				wantFiles[name] = gotFiles[name]
			}
		}
		if !reflect.DeepEqual(gotFiles, wantFiles) {
			t.Errorf("stageNpmPackages(%q) staged %v; want %v", version, gotFiles, wantFiles)
		}

		for name := range wantFiles {
			got := readTestFile(t, filepath.Join(outDir, filepath.FromSlash(name)))
			want := npmWantContent(t, name)
			if got != want {
				t.Errorf("stageNpmPackages(%q): %s =\n%s\nwant\n%s", version, name, got, want)
			}
		}
	}
}

// npmPlatformDir is the staged directory of target's platform package.
func npmPlatformDir(target distTarget) string {
	return "photon-" + npmOS[target.goos] + "-" + npmCPU[target.goarch]
}

// npmWantContent is the content a staged file must hold: a binary is the fake toolchain's stub
// for its target, every other file its golden under testdata/npm.
func npmWantContent(t *testing.T, name string) string {
	t.Helper()
	for _, target := range distTargets {
		if name == npmPlatformDir(target)+"/bin/"+target.binaryFile() {
			return stubBinary([]string{"CGO_ENABLED=0", "GOOS=" + target.goos, "GOARCH=" + target.goarch})
		}
	}
	return readTestFile(t, filepath.Join(npmGoldenDir, filepath.FromSlash(name)))
}

// TestStageNpmPackagesReturnsThePublishOrder expects the seven packages named and located in
// publish order: the six platform packages first, the wrapper that depends on them last.
func TestStageNpmPackagesReturnsThePublishOrder(t *testing.T) {
	t.Parallel()

	outDir, staged := stageNpmFixture(t, goldenVersion)

	gotNames := make([]string, 0, len(staged))
	for _, stagedPackage := range staged {
		gotNames = append(gotNames, stagedPackage.name)
		wantDir := filepath.Join(outDir, filepath.Base(stagedPackage.name))
		if stagedPackage.dir != wantDir {
			t.Errorf("%s staged at %s; want %s", stagedPackage.name, stagedPackage.dir, wantDir)
		}
	}
	if !reflect.DeepEqual(gotNames, npmWantPackages) {
		t.Errorf("stageNpmPackages returned %v; want %v", gotNames, npmWantPackages)
	}
}

// npmManifest is the part of a staged package.json the tests assert.
type npmManifest struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Bin                  map[string]string `json:"bin"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	OS                   []string          `json:"os"`
	CPU                  []string          `json:"cpu"`
	Files                []string          `json:"files"`
}

// readNpmManifest parses the package.json staged in dir.
func readNpmManifest(t *testing.T, dir string) npmManifest {
	t.Helper()
	var manifest npmManifest
	if err := json.Unmarshal([]byte(readTestFile(t, filepath.Join(dir, "package.json"))), &manifest); err != nil {
		t.Fatalf("%s/package.json is not valid JSON: %v", dir, err)
	}
	return manifest
}

// TestStageNpmPackagesManifestsWireTheWrapperToEveryPlatform asserts the manifests field by
// field: the wrapper installs the launcher as `photon` and depends optionally on all six platform
// packages at its own version; each platform package is limited to its os and cpu and ships bin/.
func TestStageNpmPackagesManifestsWireTheWrapperToEveryPlatform(t *testing.T) {
	t.Parallel()
	const version = "9.8.7"

	archiveDir := runTestDist(t, fakeToolchain(), testSourceTreeAt(t, version+"\n"))
	sums := readTestFile(t, filepath.Join(archiveDir, checksumsFile))
	staged, err := stageNpmPackages("v"+version, archiveDir, sums, t.TempDir())
	if err != nil {
		t.Fatalf("stageNpmPackages = %v", err)
	}

	wrapper := readNpmManifest(t, staged[len(staged)-1].dir)
	wantDependencies := map[string]string{}
	for i, stagedPackage := range staged[:len(staged)-1] {
		wantDependencies[stagedPackage.name] = version
		platform := readNpmManifest(t, stagedPackage.dir)
		target := distTargets[i]
		want := npmManifest{
			Name:    stagedPackage.name,
			Version: version,
			OS:      []string{npmOS[target.goos]},
			CPU:     []string{npmCPU[target.goarch]},
			Files:   []string{"bin/"},
		}
		if !reflect.DeepEqual(platform, want) {
			t.Errorf("%s manifest = %+v; want %+v", stagedPackage.name, platform, want)
		}
	}
	want := npmManifest{
		Name:                 "@airiclenz/photon",
		Version:              version,
		Bin:                  map[string]string{"photon": "bin/photon.js"},
		OptionalDependencies: wantDependencies,
		Files:                []string{"bin/"},
	}
	if !reflect.DeepEqual(wrapper, want) {
		t.Errorf("wrapper manifest = %+v; want %+v", wrapper, want)
	}
}

// TestStageNpmPackagesRefusesABadRelease covers what staging refuses before it writes anything:
// an archive whose bytes differ from its SHA256SUMS hash, a hash SHA256SUMS omits or garbles, a
// missing archive and a version that is not a release.
func TestStageNpmPackagesRefusesABadRelease(t *testing.T) {
	t.Parallel()
	const tampered = "photon_0.0.0_darwin_arm64.tar.gz"

	tests := map[string]struct {
		version string
		corrupt func(t *testing.T, archiveDir, sums string) string
		wantErr error
	}{
		"hash mismatch": {
			version: goldenVersion,
			corrupt: func(t *testing.T, archiveDir, sums string) string {
				appendToFile(t, filepath.Join(archiveDir, tampered), "x")
				return sums
			},
			wantErr: errNpmArchiveMismatch,
		},
		"hash missing": {
			version: goldenVersion,
			corrupt: func(*testing.T, string, string) string { return "" },
			wantErr: errNoChecksum,
		},
		"hash malformed": {
			version: goldenVersion,
			corrupt: func(_ *testing.T, _, sums string) string {
				return "zz  " + tampered + "\n" + sums
			},
			wantErr: errMalformedChecksum,
		},
		"archive missing": {
			version: goldenVersion,
			corrupt: func(t *testing.T, archiveDir, sums string) string {
				if err := os.Remove(filepath.Join(archiveDir, tampered)); err != nil {
					t.Fatal(err)
				}
				return sums
			},
			wantErr: fs.ErrNotExist,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			archiveDir, sums := npmFixtureRelease(t)
			sums = test.corrupt(t, archiveDir, sums)
			outDir := filepath.Join(t.TempDir(), "npm")

			_, err := stageNpmPackages(test.version, archiveDir, sums, outDir)

			if !errors.Is(err, test.wantErr) {
				t.Errorf("stageNpmPackages = %v; want %v", err, test.wantErr)
			}
			if _, statErr := os.Stat(outDir); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("stageNpmPackages wrote %s before refusing (stat: %v)", outDir, statErr)
			}
		})
	}

	t.Run("not a release version", func(t *testing.T) {
		t.Parallel()
		archiveDir, sums := npmFixtureRelease(t)
		if _, err := stageNpmPackages("v1.2", archiveDir, sums, t.TempDir()); err == nil {
			t.Error("stageNpmPackages(v1.2) = nil; want a not-a-release-version error")
		}
	})
}

// appendToFile appends text to the file at filePath.
func appendToFile(t *testing.T, filePath, text string) {
	t.Helper()
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

// TestExtractNpmBinaryReadsTarGzAndZip extracts the binary entry from a fake tar.gz and a fake
// zip holding a document beside it, and refuses an archive that lacks the entry.
func TestExtractNpmBinaryReadsTarGzAndZip(t *testing.T) {
	t.Parallel()
	const binaryContent = "the binary\n"

	tests := map[string]struct {
		target    distTarget
		entryName string
		wantErr   error
	}{
		"tar.gz":         {target: distTarget{goos: "linux", goarch: "arm64"}, entryName: "stem/photon"},
		"zip":            {target: distTarget{goos: "windows", goarch: "amd64"}, entryName: "stem/photon.exe"},
		"tar.gz lacking": {target: distTarget{goos: "darwin", goarch: "amd64"}, entryName: "stem/other", wantErr: errNoEntry},
		"zip lacking":    {target: distTarget{goos: "windows", goarch: "arm64"}, entryName: "stem/other", wantErr: errNoEntry},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			archivePath := writeNpmTestArchive(t, test.target, binaryContent)
			destination := filepath.Join(t.TempDir(), test.target.binaryFile())

			err := extractNpmBinary(test.target, archivePath, test.entryName, destination)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("extractNpmBinary = %v; want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				return
			}
			if got := readTestFile(t, destination); got != binaryContent {
				t.Errorf("extracted %q; want %q", got, binaryContent)
			}
		})
	}
}

// writeNpmTestArchive packs an archive of target's kind holding stem/<binary> with content and a
// stem/LICENSE beside it, and returns its path.
func writeNpmTestArchive(t *testing.T, target distTarget, content string) string {
	t.Helper()
	sourceDir := t.TempDir()
	binarySource := filepath.Join(sourceDir, "binary")
	licenseSource := filepath.Join(sourceDir, "LICENSE")
	if err := os.WriteFile(binarySource, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(licenseSource, []byte(testLicense), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []archiveEntry{
		{name: "stem/LICENSE", source: licenseSource, mode: documentMode},
		{name: "stem/" + target.binaryFile(), source: binarySource, mode: binaryMode},
	}

	archivePath := filepath.Join(t.TempDir(), target.archiveName("0.0.0"))
	commitTime := time.Unix(1760000000, 0)
	err := writeArchive(archivePath, func(file io.Writer) error {
		if target.isWindows() {
			return packZip(file, "stem", entries, commitTime)
		}
		return packTarGz(file, "stem", entries, commitTime)
	})
	if err != nil {
		t.Fatal(err)
	}
	return archivePath
}
