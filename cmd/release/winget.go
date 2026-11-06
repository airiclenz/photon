package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// The repository the winget step submits to, and the directory holding the package's manifests
// there, one subdirectory per version: the ones its probe reads.
const (
	wingetRepo        = "microsoft/winget-pkgs"
	wingetManifestDir = "manifests/a/AiricLenz/Photon"
)

// komacCommand is the tool the winget step submits with: it opens the winget-pkgs pull request.
const komacCommand = "komac"

// wingetWorkDirPrefix names the temporary directory a first submission renders its manifests in.
const wingetWorkDirPrefix = "photon-winget-"

// probeWinget reports winget done once the version's manifest is published in winget-pkgs or an
// open pull request of the signed-in account submits it, and pending otherwise — including while
// the package is not in winget-pkgs at all: publish submits the first manifests itself with
// `komac submit`. While the package is not listed and an open pull request already submits
// another version of it, winget is setup-pending: that first submission must merge before
// `komac update` can follow it, and a second `komac submit` would only open a rival new-package
// pull request.
func probeWinget(ctx context.Context, env probeEnv, version releaseVersion) stepReport {
	bare := bareRelease(version)
	versions, isListed, err := env.channels.ListDirs(ctx, wingetRepo, wingetManifestDir)
	switch {
	case errors.Is(err, errUnparsableAnswer):
		return unknownStep(stepWinget, "could not parse the %s listing: %v", wingetManifestDir, err)
	case err != nil:
		return unknownStep(stepWinget, "could not read %s from %s: %v",
			wingetManifestDir, wingetRepo, err)
	case slices.Contains(versions, bare):
		return stepReport{Name: stepWinget, State: stateDone, Detail: fmt.Sprintf(
			"published: %s/%s in %s", wingetManifestDir, bare, wingetRepo)}
	}

	pulls, err := env.channels.OpenPulls(ctx, wingetRepo, wingetPackageID+" in:title")
	switch {
	case errors.Is(err, errUnparsableAnswer):
		return unknownStep(stepWinget, "could not parse gh pr list output: %v", err)
	case err != nil:
		return unknownStep(stepWinget, "could not list the open %s pull requests: %v",
			wingetRepo, err)
	}
	for _, pull := range pulls {
		if titleNamesRelease(pull.title, bare) {
			return stepReport{Name: stepWinget, State: stateDone, Detail: fmt.Sprintf(
				"submitted: pull request #%d %s", pull.number, pull.url)}
		}
	}
	if !isListed {
		return unlistedWingetStep(pulls, bare)
	}
	return stepReport{Name: stepWinget, State: statePending, Detail: fmt.Sprintf(
		"no %s %s manifest or open pull request in %s", wingetPackageID, bare, wingetRepo)}
}

// unlistedWingetStep is the winget step while winget-pkgs does not have the package yet and no
// open pull request submits the version: setup-pending while one submits another version as the
// package's first, pending otherwise.
func unlistedWingetStep(pulls []pullRequest, bare string) stepReport {
	for _, pull := range pulls {
		if titleNamesPackage(pull.title) {
			return stepReport{Name: stepWinget, State: stateSetupPending, Detail: fmt.Sprintf(
				"%s is not in %s yet: its first submission, pull request #%d %s, has not merged",
				wingetPackageID, wingetRepo, pull.number, pull.url)}
		}
	}
	return stepReport{Name: stepWinget, State: statePending, Detail: fmt.Sprintf(
		"%s is not in %s yet and no open pull request submits %s: "+
			"publish submits it with `komac submit`",
		wingetPackageID, wingetRepo, bare)}
}

// titleNamesRelease reports whether a winget pull request's title names the package and exactly
// the version — a search for 0.24.1 also finds 0.24.11, which must not count.
func titleNamesRelease(title, bare string) bool {
	return titleNamesPackage(title) && slices.Contains(titleWords(title), bare)
}

// titleNamesPackage reports whether a winget pull request's title names the package as a whole
// word — AiricLenz.PhotonX must not count.
func titleNamesPackage(title string) bool {
	return slices.Contains(titleWords(title), wingetPackageID)
}

// titleWords is a pull request title's words, stripped of the punctuation around them.
func titleWords(title string) []string {
	words := strings.Fields(title)
	for i, word := range words {
		words[i] = strings.Trim(word, ".,:;()[]")
	}
	return words
}

// githubTokenVariable is the environment variable komac reads its GitHub token from, and the one
// publish takes the token from first.
const githubTokenVariable = "GITHUB_TOKEN"

// redactedToken stands in for the GitHub token wherever a command line is printed: the token
// reaches komac through its environment alone, never its argv or this program's output.
const redactedToken = "<redacted>"

// komacInstallHint names the ways to install komac, for the step that finds it missing.
const komacInstallHint = "install it with `cargo install --locked komac`, `brew install komac` " +
	"or `winget install komac` (https://github.com/russellbanks/Komac)"

// wingetSubmittedDetail is what the winget step reports when it has run: komac opens a pull
// request — `komac update` and `komac submit` alike — which merges later after winget's
// validation and review, so the release is submitted to winget, never published there, when
// publish returns.
const wingetSubmittedDetail = "submitted by this run: the pull request merges in " + wingetRepo +
	" after its validation and review"

// resolveWingetPackage records in the run whether winget-pkgs already has the package, which
// picks the winget step's komac path. It asks only while the winget step is pending: a done,
// setup-pending or unknown step never reaches the winget step's converge. It reads the listing
// the winget probe reads; a listing that is not there means the package is not there yet.
func (r *publishRun) resolveWingetPackage(ctx context.Context, steps []stepReport) error {
	if stepStateOf(steps, stepWinget) != statePending {
		return nil
	}
	_, isListed, err := r.channels.ListDirs(ctx, wingetRepo, wingetManifestDir)
	if err != nil {
		return fmt.Errorf("could not tell whether %s has %s: %w",
			wingetRepo, wingetManifestDir, err)
	}
	r.isWingetPackageListed = isListed
	return nil
}

// describeWinget says what the winget step does, with the komac command line it runs — the
// token shown redacted, as it never appears there.
func describeWinget(run *publishRun) string {
	bare := bareRelease(run.version)
	if !run.isWingetPackageListed {
		tempDir := filepath.Join("<temp>", filepath.FromSlash(wingetManifestDir), bare)
		return fmt.Sprintf("render the first %s %s manifests into a temporary %s/%s and submit "+
			"them to %s (submitted, not published, until the pull request merges): %s",
			wingetPackageID, bare, wingetManifestDir, bare, wingetRepo,
			komacCommandLine(komacSubmitArgs(tempDir)))
	}
	return fmt.Sprintf("submit the %s %s manifest update to %s "+
		"(submitted, not published, until the pull request merges): %s",
		wingetPackageID, bare, wingetRepo, komacCommandLine(komacArgs(run.version)))
}

// komacCommandLine is komac's command line as publish prints it, the token redacted.
func komacCommandLine(args []string) string {
	return githubTokenVariable + "=" + redactedToken + " " + komacCommand + " " +
		strings.Join(args, " ")
}

// convergeWinget submits the version to winget-pkgs with komac, which opens the pull request:
// `komac update` when the package is listed there, otherwise `komac submit` of the first
// manifests. A missing komac or GitHub token is a setup the maintainer has to do, so the step
// reports setup-pending rather than failing the run. The GitHub token reaches komac through its
// environment only, and komac's stdin is closed, so it can never wait on a prompt.
func convergeWinget(ctx context.Context, run *publishRun) error {
	dir := run.options.dir
	if _, err := run.commands.run(ctx, dir, nil, komacCommand, "--version"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return setupPendingError{detail: "komac is not installed — " + komacInstallHint}
		}
		return fmt.Errorf("run komac: %w", err)
	}
	token, err := githubToken(ctx, run)
	if err != nil {
		return setupPendingError{detail: err.Error()}
	}

	env := append([]string{githubTokenVariable + "=" + token}, nonInteractiveEnv...)
	if !run.isWingetPackageListed {
		return submitWingetManifests(ctx, run, env, token)
	}
	_, err = run.commands.run(ctx, dir, env, komacCommand, komacArgs(run.version)...)
	if err != nil {
		// A failing command's error carries its stderr; scrub the token in case komac echoed it.
		return fmt.Errorf("submit the %s %s manifest update: %s",
			wingetPackageID, bareRelease(run.version),
			strings.ReplaceAll(err.Error(), token, redactedToken))
	}
	return nil
}

// submitWingetManifests renders the version's three winget manifests from the release's
// SHA256SUMS into <temp>/manifests/a/AiricLenz/Photon/<X.Y.Z>/ and submits them with
// `komac submit` — `komac new` cannot run unattended for a zip installer. The directory is
// removed once the submission is made; when komac fails it is kept and the failure is reported
// as setup pending, naming the directory and the command to rerun it with, the token redacted.
func submitWingetManifests(ctx context.Context, run *publishRun, env []string, token string) error {
	sums, err := downloadReleaseSums(ctx, run)
	if err != nil {
		return err
	}
	manifests, err := renderWingetManifests(run.version.String(), sums)
	if err != nil {
		return err
	}
	workDir, err := os.MkdirTemp("", wingetWorkDirPrefix)
	if err != nil {
		return fmt.Errorf("create the winget manifest directory: %w", err)
	}
	manifestDir := filepath.Join(workDir, filepath.FromSlash(wingetManifestDir),
		bareRelease(run.version))
	if err := writeWingetManifests(manifestDir, manifests); err != nil {
		_ = os.RemoveAll(workDir)
		return err
	}

	args := komacSubmitArgs(manifestDir)
	if _, err := run.commands.run(ctx, run.options.dir, env, komacCommand, args...); err != nil {
		return setupPendingError{detail: fmt.Sprintf(
			"the first %s submission failed (%s); the manifests are kept in %s: "+
				"submit them with `%s`",
			wingetPackageID, strings.ReplaceAll(err.Error(), token, redactedToken),
			manifestDir, komacCommandLine(args))}
	}
	if err := os.RemoveAll(workDir); err != nil {
		_, _ = fmt.Fprintf(run.output, "  could not remove %s: %v\n", workDir, err)
	}
	return nil
}

// writeWingetManifests writes the rendered manifests into manifestDir, creating it.
func writeWingetManifests(manifestDir string, manifests []renderedFile) error {
	if err := os.MkdirAll(manifestDir, directoryMode); err != nil {
		return fmt.Errorf("create %s: %w", manifestDir, err)
	}
	for _, manifest := range manifests {
		path := filepath.Join(manifestDir, manifest.name)
		if err := os.WriteFile(path, []byte(manifest.content), documentMode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// komacSubmitArgs is komac's argv for submitting the manifests in manifestDir as they are, with
// --yes answering the confirmation it would otherwise ask for.
func komacSubmitArgs(manifestDir string) []string {
	return []string{"submit", manifestDir, "--yes"}
}

// komacArgs is komac's argv for the version's manifest update: the two Windows archives' release
// URLs, which komac downloads to hash, and --submit to open the pull request.
func komacArgs(version releaseVersion) []string {
	bare := bareRelease(version)
	args := []string{"update", wingetPackageID, "--version", bare, "--urls"}
	for _, target := range distTargets {
		if target.isWindows() {
			args = append(args, releaseDownloadBase+version.String()+"/"+target.archiveName(bare))
		}
	}
	return append(args, "--submit")
}

// githubToken is the token komac opens its pull request with: GITHUB_TOKEN when set, else the
// one gh is signed in with. Every error it returns means there is no token to use.
func githubToken(ctx context.Context, run *publishRun) (string, error) {
	if token := strings.TrimSpace(run.options.getenv(githubTokenVariable)); token != "" {
		return token, nil
	}
	noToken := fmt.Sprintf("no GitHub token — set %s or sign in with `gh auth login`",
		githubTokenVariable)
	token, err := run.commands.run(ctx, run.options.dir, nonInteractiveEnv, "gh", "auth", "token")
	if err != nil {
		return "", fmt.Errorf("%s: %w", noToken, err)
	}
	if token = strings.TrimSpace(token); token == "" {
		return "", errors.New(noToken)
	}
	return token, nil
}
