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
	"sync"
	"sync/atomic"
	"testing"
)

// The token the winget tests hand publish, the release URLs komac is given for v0.24.11, and
// the manifest directory a first submission renders under its temporary directory.
const (
	sentinelToken     = "sentinel-token-must-not-print"
	testReleaseBase   = "https://github.com/airiclenz/photon/releases/download/v0.24.11/"
	testWingetCommand = "GITHUB_TOKEN=<redacted> komac update AiricLenz.Photon --version 0.24.11 --urls " +
		testReleaseBase + "photon_0.24.11_windows_amd64.zip " + testReleaseBase + "photon_0.24.11_windows_arm64.zip --submit"
	testManifestSubdir = "manifests/a/AiricLenz/Photon/0.24.11"
)

// testWingetManifests are the file names a first winget submission's directory holds.
var testWingetManifests = []string{
	"AiricLenz.Photon.installer.yaml",
	"AiricLenz.Photon.locale.en-US.yaml",
	"AiricLenz.Photon.yaml",
}

// fakeChannels is what the winget step meets beyond winget-pkgs: whether komac is installed,
// whether `komac submit` or `komac update` fails, and the token gh is signed in with ("" for
// none).
type fakeChannels struct {
	isKomacMissing       bool
	isKomacSubmitFailing bool
	isKomacUpdateFailing bool
	ghToken              string
	// submitted lists the files the manifest directory held when `komac submit` ran.
	submitted []string

	mu sync.Mutex
}

// emptyEnv is an environment with no variable set.
func emptyEnv(string) string { return "" }

// respondTools answers the commands the channel steps run beside the channel repositories: komac
// and `gh auth token`. Any other command is unexpected.
func (f *publishFake) respondTools(call recordedCall) (string, error) {
	f.channels.mu.Lock()
	defer f.channels.mu.Unlock()
	args := call.args
	command := call.name + " " + strings.Join(args, " ")
	switch {
	case command == "komac --version":
		if f.channels.isKomacMissing {
			return "", fmt.Errorf("komac --version: %w", &exec.Error{Name: "komac", Err: exec.ErrNotFound})
		}
		return "komac 2.11.2\n", nil
	case call.name == "komac" && len(args) == 3 && args[0] == "submit":
		return f.submitManifests(call)
	case call.name == "komac" && len(args) > 0 && args[0] == "update" && f.channels.isKomacUpdateFailing:
		return "", komacCredentialError(call)
	case call.name == "komac":
		return "", nil
	case command == "gh auth token":
		if f.channels.ghToken == "" {
			return "", errors.New("gh auth token: exit status 1: no oauth token found for github.com")
		}
		return f.channels.ghToken + "\n", nil
	}
	return "", fmt.Errorf("unexpected command %s", command)
}

// submitManifests answers `komac submit <dir> --yes`: it records the files the directory holds,
// and when the submission is set to fail, fails echoing the token from komac's environment, as
// a credential error might.
func (f *publishFake) submitManifests(call recordedCall) (string, error) {
	entries, err := os.ReadDir(call.args[1])
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		f.channels.submitted = append(f.channels.submitted, entry.Name())
	}
	if !f.channels.isKomacSubmitFailing {
		return "", nil
	}
	return "", komacCredentialError(call)
}

// komacCredentialError is a failing komac call's error echoing the token from its environment,
// as a credential error might.
func komacCredentialError(call recordedCall) error {
	var token string
	for _, variable := range call.env {
		if value, isToken := strings.CutPrefix(variable, githubTokenVariable+"="); isToken {
			token = value
		}
	}
	return fmt.Errorf("komac %s: exit status 1: bad credentials %s", strings.Join(call.args, " "), token)
}

// wingetPendingFake is releasedFake with winget-pkgs a version behind: the tap and the bucket
// already pin the release, so the winget step is the one channel step pending, and it holds
// versions as the package's manifest directories (nil for a package not in winget-pkgs yet) and
// pulls as its open pull requests.
func wingetPendingFake(versions []string, pulls ...pullRequest) *publishFake {
	fake := releasedFake()
	fake.remote = channelReposOf(fake.remote,
		tapHolding(testFormula),
		bucketHolding(`{"version": "0.24.11"}`),
		wingetHolding(versions, pulls...),
	)
	return fake
}

// runChannelPublish runs publish of publishVersion over fake with a written notes file and an
// environment whose GITHUB_TOKEN is token ("" for unset).
func runChannelPublish(t *testing.T, fake *publishFake, isDryRun bool, token string) publishRunResult {
	t.Helper()
	notesPath := filepath.Join(t.TempDir(), "notes.md")
	writeTestFile(t, notesPath, "## What's new\n\n- A release.\n")
	getenv := func(key string) string {
		if key == githubTokenVariable {
			return token
		}
		return ""
	}
	return runTestPublishWith(t, fake, publishOptions{notesFile: notesPath, isDryRun: isDryRun, getenv: getenv})
}

// komacCalls returns the komac calls the runner was asked for whose first argument is verb.
func komacCalls(commands *fakeRunner, verb string) []recordedCall {
	return slices.DeleteFunc(commands.recorded("komac"), func(call recordedCall) bool {
		return len(call.args) == 0 || call.args[0] != verb
	})
}

// TestPublishWingetDryRunPrintsTheKomacCommandWithTheTokenRedacted pins the dry run of an update:
// the plan shows komac's exact command line with the token redacted, the token itself is printed
// nowhere, and nothing — not even komac's version check — is run.
func TestPublishWingetDryRunPrintsTheKomacCommandWithTheTokenRedacted(t *testing.T) {
	t.Parallel()

	result := runChannelPublish(t, wingetPendingFake([]string{"0.24.10"}), true, sentinelToken)

	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, testWingetCommand) {
		t.Errorf("the dry run's plan lacks the komac command\n%s\nin:\n%s", testWingetCommand, result.output)
	}
	if strings.Contains(result.output, sentinelToken) {
		t.Errorf("the dry run printed the GITHUB_TOKEN value:\n%s", result.output)
	}
	if calls := result.commands.recorded("komac"); len(calls) > 0 {
		t.Errorf("the dry run ran komac %d times, want never", len(calls))
	}
	if mutating := mutatingLines(result.commands); len(mutating) > 0 {
		t.Errorf("issued mutating commands %q, want none", mutating)
	}
}

// TestPublishWingetSubmitsWithTheTokenInKomacsEnvironment pins the update of a package winget-pkgs
// already lists: it runs `komac update` with the dry run's argv, the token reaches komac through
// GITHUB_TOKEN in its environment — taken from the environment, else from `gh auth token` — and
// never through its argv or the output, and the step reports the release submitted, never
// published.
func TestPublishWingetSubmitsWithTheTokenInKomacsEnvironment(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		envToken string
		ghToken  string
	}{
		"GITHUB_TOKEN set":         {envToken: sentinelToken, ghToken: "gh-token-unused"},
		"token from gh auth token": {ghToken: sentinelToken},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := wingetPendingFake([]string{"0.24.10"})
			fake.channels.ghToken = tc.ghToken

			result := runChannelPublish(t, fake, false, tc.envToken)

			if result.err != nil {
				t.Fatalf("publish failed: %v\n%s", result.err, result.output)
			}
			updates := komacCalls(result.commands, "update")
			if len(updates) != 1 {
				t.Fatalf("ran komac update %d times, want once", len(updates))
			}
			argv := "komac " + strings.Join(updates[0].args, " ")
			if want := strings.TrimPrefix(testWingetCommand, "GITHUB_TOKEN=<redacted> "); argv != want {
				t.Errorf("komac argv:\n got: %s\nwant: %s", argv, want)
			}
			if !slices.Contains(updates[0].env, githubTokenVariable+"="+sentinelToken) {
				t.Errorf("komac's environment %q lacks GITHUB_TOKEN=<the token>", updates[0].env)
			}
			if len(komacCalls(result.commands, "submit")) > 0 {
				t.Errorf("ran komac submit for a package winget-pkgs already lists")
			}
			if strings.Contains(result.output, sentinelToken) {
				t.Errorf("publish printed the token:\n%s", result.output)
			}
			if !hasRow(result.output, "winget", "done", wingetSubmittedDetail) {
				t.Errorf("the winget row does not report the release submitted:\n%s", result.output)
			}
		})
	}
}

// TestPublishWingetUpdateFailureRedactsTheToken pins a failed `komac update` whose error echoes
// the token: the winget step fails naming the manifest update, and the token reaches neither the
// error nor the output.
func TestPublishWingetUpdateFailureRedactsTheToken(t *testing.T) {
	t.Parallel()
	fake := wingetPendingFake([]string{"0.24.10"})
	fake.channels.isKomacUpdateFailing = true

	result := runChannelPublish(t, fake, false, sentinelToken)

	if result.err == nil {
		t.Fatalf("publish succeeded with komac update failing:\n%s", result.output)
	}
	const want = "the winget step failed: submit the AiricLenz.Photon 0.24.11 manifest update"
	if !strings.Contains(result.err.Error(), want) {
		t.Errorf("publish error = %v, want it to contain %q", result.err, want)
	}
	if strings.Contains(result.err.Error(), sentinelToken) {
		t.Errorf("publish error carries the token: %v", result.err)
	}
	if strings.Contains(result.output, sentinelToken) {
		t.Errorf("publish printed the token:\n%s", result.output)
	}
}

// TestPublishWingetWithoutKomacOrTokenIsSetupPending pins the maintainer's own setup: with komac
// not installed, or no GitHub token in the environment or from gh, the winget step submits
// nothing and reads setup-pending, naming what is missing, and the run goes on to the smoke step
// and succeeds.
func TestPublishWingetWithoutKomacOrTokenIsSetupPending(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		isKomacMissing bool
		envToken       string
		wantDetail     string
	}{
		"komac missing": {isKomacMissing: true, envToken: sentinelToken,
			wantDetail: "komac is not installed — install it with `cargo install --locked komac`"},
		"no token": {wantDetail: "no GitHub token — set GITHUB_TOKEN or sign in with `gh auth login`"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := wingetPendingFake([]string{"0.24.10"})
			fake.channels.isKomacMissing = tc.isKomacMissing

			result := runChannelPublish(t, fake, false, tc.envToken)

			if result.err != nil {
				t.Fatalf("publish failed: %v\n%s", result.err, result.output)
			}
			if !strings.Contains(result.output, "winget: its one-time setup is pending: "+tc.wantDetail) ||
				!hasRow(result.output, "winget", "setup-pending", tc.wantDetail) {
				t.Errorf("the winget row does not report setup pending with %q:\n%s", tc.wantDetail, result.output)
			}
			for _, verb := range []string{"update", "submit"} {
				if calls := komacCalls(result.commands, verb); len(calls) > 0 {
					t.Errorf("ran komac %s %d times, want never", verb, len(calls))
				}
			}
			if len(result.smokes) != 1 {
				t.Errorf("smoke ran %d times, want once", len(result.smokes))
			}
			if strings.Contains(result.output, sentinelToken) {
				t.Errorf("publish printed the token:\n%s", result.output)
			}
		})
	}
}

// TestPublishWingetSubmitsTheFirstManifestsWhenThePackageIsAbsent pins the first submission: with
// no AiricLenz.Photon in winget-pkgs, publish renders the three manifests into
// manifests/a/AiricLenz/Photon/<X.Y.Z>/ and runs `komac submit <dir> --yes` with the token in
// komac's environment only, never `komac update`; the directory is removed once it is submitted.
func TestPublishWingetSubmitsTheFirstManifestsWhenThePackageIsAbsent(t *testing.T) {
	t.Parallel()
	fake := wingetPendingFake(nil)

	result := runChannelPublish(t, fake, false, sentinelToken)

	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	submits := komacCalls(result.commands, "submit")
	if len(submits) != 1 {
		t.Fatalf("ran komac submit %d times, want once; komac calls: %v", len(submits), result.commands.recorded("komac"))
	}
	args := submits[0].args
	if len(args) != 3 || args[2] != "--yes" || !strings.HasSuffix(filepath.ToSlash(args[1]), "/"+testManifestSubdir) {
		t.Errorf("komac argv = %q, want submit <temp>/%s --yes", args, testManifestSubdir)
	}
	if !slices.Contains(submits[0].env, githubTokenVariable+"="+sentinelToken) {
		t.Errorf("komac's environment %q lacks GITHUB_TOKEN=<the token>", submits[0].env)
	}
	if len(komacCalls(result.commands, "update")) > 0 {
		t.Errorf("ran komac update for a package winget-pkgs does not have")
	}
	slices.Sort(fake.channels.submitted)
	if !slices.Equal(fake.channels.submitted, testWingetManifests) {
		t.Errorf("the submitted directory held %q, want %q", fake.channels.submitted, testWingetManifests)
	}
	if _, err := os.Stat(args[1]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the manifest directory %s outlived its submission: %v", args[1], err)
	}
	if strings.Contains(result.output, sentinelToken) {
		t.Errorf("publish printed the token:\n%s", result.output)
	}
	if !hasRow(result.output, "winget", "done", wingetSubmittedDetail) {
		t.Errorf("the winget row does not report the release submitted:\n%s", result.output)
	}
}

// TestPublishWingetSubmitFailureIsSetupPending pins a failed first submission: it does not fail
// the run — the winget row reads setup-pending, naming the kept manifest directory and the
// command to submit it with, the token redacted — and the smoke step still runs.
func TestPublishWingetSubmitFailureIsSetupPending(t *testing.T) {
	t.Parallel()
	fake := wingetPendingFake(nil)
	fake.channels.isKomacSubmitFailing = true

	result := runChannelPublish(t, fake, false, sentinelToken)

	submits := komacCalls(result.commands, "submit")
	if len(submits) == 0 {
		t.Fatalf("komac submit never ran:\n%s", result.output)
	}
	manifestDir := submits[0].args[1]
	t.Cleanup(func() {
		suffix := string(filepath.Separator) + filepath.FromSlash(testManifestSubdir)
		_ = os.RemoveAll(strings.TrimSuffix(manifestDir, suffix))
	})
	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	wantCommand := "`GITHUB_TOKEN=<redacted> komac submit " + manifestDir + " --yes`"
	if !hasRow(result.output, "winget", "setup-pending", "the first AiricLenz.Photon submission failed") ||
		!strings.Contains(result.output, wantCommand) {
		t.Errorf("the winget row does not report setup pending with %s:\n%s", wantCommand, result.output)
	}
	if strings.Contains(result.output, sentinelToken) {
		t.Errorf("publish printed the token:\n%s", result.output)
	}
	if len(result.smokes) != 1 {
		t.Errorf("smoke ran %d times after the failed submission, want once", len(result.smokes))
	}
	if kept := listDir(t, manifestDir); !slices.Equal(kept, testWingetManifests) {
		t.Errorf("the manifest directory holds %q after the run, want %q", kept, testWingetManifests)
	}
}

// TestPublishWingetDryRunForAnAbsentPackage pins the dry run of a first submission: the plan names
// `komac submit` with the token redacted, and nothing is rendered or submitted.
func TestPublishWingetDryRunForAnAbsentPackage(t *testing.T) {
	t.Parallel()

	result := runChannelPublish(t, wingetPendingFake(nil), true, sentinelToken)

	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "GITHUB_TOKEN=<redacted> komac submit") {
		t.Errorf("the dry run's plan lacks the komac submit command:\n%s", result.output)
	}
	if strings.Contains(result.output, sentinelToken) {
		t.Errorf("the dry run printed the GITHUB_TOKEN value:\n%s", result.output)
	}
	if calls := result.commands.recorded("komac"); len(calls) > 0 {
		t.Errorf("the dry run ran komac %d times, want never", len(calls))
	}
	if mutating := mutatingLines(result.commands); len(mutating) > 0 {
		t.Errorf("issued mutating commands %q, want none", mutating)
	}
}

// TestPublishLeavesASubmittedWingetReleaseAlone pins the winget probe's hold over komac: a
// published manifest directory, or an open pull request of the account for the version — a
// submission already made — leaves the step done, and komac is never run.
func TestPublishLeavesASubmittedWingetReleaseAlone(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		fake       *publishFake
		wantDetail string
	}{
		"manifest published": {fake: wingetPendingFake([]string{"0.24.10", "0.24.11"}),
			wantDetail: "published: manifests/a/AiricLenz/Photon/0.24.11 in microsoft/winget-pkgs"},
		"pull request open": {fake: wingetPendingFake([]string{"0.24.10"}, komacPull(301234, "New version", "0.24.11")),
			wantDetail: "submitted: pull request #301234"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := runChannelPublish(t, tc.fake, false, sentinelToken)

			if result.err != nil {
				t.Fatalf("publish failed: %v\n%s", result.err, result.output)
			}
			if calls := result.commands.recorded("komac"); len(calls) > 0 {
				t.Errorf("ran komac %d times over a submitted release, want never", len(calls))
			}
			if !hasRow(result.output, "winget", "done", tc.wantDetail) {
				t.Errorf("the winget row does not report %q:\n%s", tc.wantDetail, result.output)
			}
		})
	}
}

// TestPublishWingetWaitsForAnOpenFirstSubmission pins the one new-package pull request: while
// AiricLenz.Photon is not in winget-pkgs and an open pull request already submits an earlier
// version as the first one, publish runs no komac at all — a second `komac submit` would open a
// rival new-package pull request — and reports winget setup-pending, naming that pull request.
func TestPublishWingetWaitsForAnOpenFirstSubmission(t *testing.T) {
	t.Parallel()
	fake := wingetPendingFake(nil, komacPull(301234, "New package", "0.24.10"))

	result := runChannelPublish(t, fake, false, sentinelToken)

	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	if calls := result.commands.recorded("komac"); len(calls) > 0 {
		t.Errorf("ran komac %d times while the first submission is open, want never: %v", len(calls), calls)
	}
	if !strings.Contains(result.output, "winget: skipped, its one-time setup is pending") {
		t.Errorf("output does not report winget skipped:\n%s", result.output)
	}
	if !hasRow(result.output, "winget", "setup-pending",
		"AiricLenz.Photon is not in microsoft/winget-pkgs yet: its first submission, pull request #301234") {
		t.Errorf("the winget row does not name the open first submission:\n%s", result.output)
	}
}

// TestPublishWingetConvergesFromRemoteStateOnceTheTagExists pins the resume for the winget step:
// days later, with VERSION moved on and a dirty tree — either of which refuses a run that
// consults them — publish still submits the pending winget manifest without consulting the gate
// or VERSION at HEAD, and pushes nothing.
func TestPublishWingetConvergesFromRemoteStateOnceTheTagExists(t *testing.T) {
	t.Parallel()
	fake := wingetPendingFake([]string{"0.24.10"})
	fake.repo.isDirty = true
	fake.repo.files[versionFile] = "v0.24.12\n"

	result := runChannelPublish(t, fake, false, sentinelToken)

	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	lines := commandLines(result.commands)
	for _, unwanted := range []string{"repo PushMain", channelsCallName + " " + methodPublishFile} {
		if index := lineIndex(lines, unwanted); index >= 0 {
			t.Errorf("ran %q, want only the winget submission once the tag exists", lines[index])
		}
	}
	if index := lineIndex(lines, "komac update "+wingetPackageID); index < 0 {
		t.Errorf("winget was not submitted; commands: %q", lines)
	}
}

// laterListingFailing is channel repositories whose ListDirs answers its first call and fails
// with err from its second on: the winget probe's listing reads, and the package lookup's read
// after it does not, which a memChannelRepos' per-repository failure cannot tell apart.
type laterListingFailing struct {
	memChannelRepos
	err   error
	calls *atomic.Int32
}

// ListDirs fails with err once it has been called before, and reads through otherwise.
func (c laterListingFailing) ListDirs(ctx context.Context, repo, path string) ([]string, bool, error) {
	if c.calls.Add(1) > 1 {
		return nil, false, c.err
	}
	return c.memChannelRepos.ListDirs(ctx, repo, path)
}

// TestPublishWingetRefusesAnUnreadablePackageListing pins resolveWingetPackage's error: with the
// winget step pending and the package unlisted, a listing read that fails after the probe's
// succeeded refuses the run, naming the listing it could not read, and runs no komac command.
func TestPublishWingetRefusesAnUnreadablePackageListing(t *testing.T) {
	t.Parallel()
	listingErr := errors.New("listing unreadable")
	fake := newPublishFake()
	channels := laterListingFailing{
		memChannelRepos: channelReposOf(tapHolding(testFormula), wingetHolding(nil)),
		err:             listingErr,
		calls:           &atomic.Int32{},
	}
	notesPath := filepath.Join(t.TempDir(), "notes.md")
	writeTestFile(t, notesPath, "## What's new\n\n- A release.\n")

	result := runTestPublishWith(t, fake, publishOptions{notesFile: notesPath, channels: channels})

	if !errors.Is(result.err, listingErr) {
		t.Fatalf("publish error = %v, want it to wrap %v\n%s", result.err, listingErr, result.output)
	}
	const want = "could not tell whether " + wingetRepo + " has " + wingetManifestDir
	if !strings.Contains(result.err.Error(), want) {
		t.Errorf("publish error = %v, want it to contain %q", result.err, want)
	}
	if calls := channels.calls.Load(); calls != 2 {
		t.Errorf("ListDirs called %d times, want 2: the probe's, then the package lookup's", calls)
	}
	if calls := result.commands.recorded("komac"); len(calls) > 0 {
		t.Errorf("ran komac %v after the listing failed", calls)
	}
}
