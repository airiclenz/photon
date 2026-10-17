package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// channelFile is the generated file publish writes in each channel repository.
var channelFile = map[string]string{tapRepo: formulaPath, bucketRepo: bucketPath}

// testReleaseSums is a SHA256SUMS body listing every archive of v0.24.11, one fixture hash each.
func testReleaseSums() string {
	var sums strings.Builder
	for index, name := range releaseAssetsV02411[:len(distTargets)] {
		sums.WriteString(strings.Repeat(fmt.Sprintf("%x", index+10), 64) + "  " + name + "\n")
	}
	return sums.String()
}

// releasedFake is a resumed run of publishVersion: the tag and the whole release are on GitHub,
// and the tap's formula and the bucket's manifest pin the previous release — so every channel step
// is pending.
func releasedFake() *publishFake {
	fake := newPublishFake()
	fake.tagged()
	fake.repo.releases[publishVersion] = releaseAssetsV02411
	fake.remote = channelReposOf(
		tapHolding(strings.Replace(testFormula, "0.24.11", "0.24.10", 1)),
		bucketHolding(`{"version": "0.24.10"}`),
	)
	fake.repo.assetContent[checksumsFile] = testReleaseSums()
	return fake
}

// channelRun is a publish run of publishVersion over fake, its channel repositories recording
// what they are pushed, for a test to drive the tap and bucket steps' converge through directly;
// output holds what the run prints.
func channelRun(t *testing.T, fake *publishFake) (run *publishRun, output *strings.Builder) {
	t.Helper()
	commands := fake.runner()
	output = &strings.Builder{}
	run = &publishRun{commands: commands, repo: fake.repo, channels: fake.remote, output: output,
		version: mustVersion(t, publishVersion), options: publishOptions{dir: t.TempDir()}}
	return run, output
}

// publishedRepos returns the repository of each file the channel repositories were pushed, in
// order.
func publishedRepos(channels memChannelRepos) []string {
	var repos []string
	for _, write := range channels.publishedFiles() {
		repos = append(repos, write.repo)
	}
	return repos
}

// TestPublishCommitsAChannelFileOnlyWhenItChanges pins the tap and the bucket: each takes the
// rendered file, committed as `photon X.Y.Z` and pushed to its branch, only when that changes the
// file; a repository already holding it is pushed nothing, and the step says so.
func TestPublishCommitsAChannelFileOnlyWhenItChanges(t *testing.T) {
	t.Parallel()
	sums := testReleaseSums()
	formula, err := renderFormula(publishVersion, sums)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := renderScoopManifest(publishVersion, sums)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		tap, bucket string
		wantPushed  []string
	}{
		"both behind": {
			wantPushed: []string{tapRepo, bucketRepo},
		},
		"tap already current": {
			tap:        formula,
			wantPushed: []string{bucketRepo},
		},
		"bucket already current": {
			bucket:     manifest,
			wantPushed: []string{tapRepo},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := releasedFake()
			if tc.tap != "" {
				fake.remote = channelReposOf(fake.remote, tapHolding(tc.tap))
			}
			if tc.bucket != "" {
				fake.remote = channelReposOf(fake.remote, bucketHolding(tc.bucket))
			}
			run, output := channelRun(t, fake)

			for _, converge := range []func(context.Context, *publishRun) error{convergeHomebrew, convergeScoop} {
				if err := converge(t.Context(), run); err != nil {
					t.Fatal(err)
				}
			}

			if pushed := publishedRepos(fake.remote); !slices.Equal(pushed, tc.wantPushed) {
				t.Errorf("pushed to %q, want %q", pushed, tc.wantPushed)
			}
			for _, write := range fake.remote.publishedFiles() {
				if write.subject != "photon 0.24.11" {
					t.Errorf("%s: committed as %q, want %q", write.repo, write.subject, "photon 0.24.11")
				}
			}
			for repo, current := range map[string]string{tapRepo: tc.tap, bucketRepo: tc.bucket} {
				said := repo + " already holds this " + channelFile[repo] + ": nothing to push"
				if isSaid := strings.Contains(output.String(), said); isSaid != (current != "") {
					t.Errorf("%s: output says %q: %t, want %t:\n%s", repo, said, isSaid, current != "", output)
				}
			}
		})
	}
}

// TestPublishWritesTheRenderedFileToTheChannel pins what the tap and the bucket receive: exactly
// the file rendered from the release's SHA256SUMS, at its path.
func TestPublishWritesTheRenderedFileToTheChannel(t *testing.T) {
	t.Parallel()
	fake := releasedFake()
	run, _ := channelRun(t, fake)

	for _, converge := range []func(context.Context, *publishRun) error{convergeHomebrew, convergeScoop} {
		if err := converge(t.Context(), run); err != nil {
			t.Fatal(err)
		}
	}

	formula, err := renderFormula(publishVersion, testReleaseSums())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := renderScoopManifest(publishVersion, testReleaseSums())
	if err != nil {
		t.Fatal(err)
	}
	want := []memChannelPublish{
		{repo: tapRepo, path: formulaPath, content: formula, subject: "photon 0.24.11"},
		{repo: bucketRepo, path: bucketPath, content: manifest, subject: "photon 0.24.11"},
	}
	if got := fake.remote.publishedFiles(); !slices.Equal(got, want) {
		t.Errorf("pushed:\n%+v\nwant the rendered formula and manifest:\n%+v", got, want)
	}
}

// TestPublishSkipsChannelsWhoseSetupIsPending pins the channels' one-time setup: a missing tap or
// bucket repository is skipped and named, touched never, and the run succeeds.
func TestPublishSkipsChannelsWhoseSetupIsPending(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		missingRepo string
		skipped     string
		updatedRepo string
	}{
		"no tap repository": {
			missingRepo: tapRepo,
			skipped:     "homebrew: skipped, its one-time setup is pending",
			updatedRepo: bucketRepo,
		},
		"no bucket repository": {
			missingRepo: bucketRepo,
			skipped:     "scoop: skipped, its one-time setup is pending",
			updatedRepo: tapRepo,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := releasedFake()
			delete(fake.remote.repos, tc.missingRepo)

			result := runTestPublish(t, fake, false)

			if result.err != nil {
				t.Fatalf("publish failed: %v\n%s", result.err, result.output)
			}
			if !strings.Contains(result.output, tc.skipped) {
				t.Errorf("output lacks %q:\n%s", tc.skipped, result.output)
			}
			pushed := publishedRepos(fake.remote)
			if slices.Contains(pushed, tc.missingRepo) {
				t.Errorf("pushed to %s, a channel whose setup is pending", tc.missingRepo)
			}
			if !slices.Contains(pushed, tc.updatedRepo) {
				t.Errorf("%s was not updated; pushed to %q", tc.updatedRepo, pushed)
			}
		})
	}
}

// TestPublishLeavesAChannelPinnedToANewerVersionAlone pins the no-downgrade rule: a rerun of
// publish for an older release finds the tap and the bucket already on a newer one, reports both
// done, and never publishes to either.
func TestPublishLeavesAChannelPinnedToANewerVersionAlone(t *testing.T) {
	t.Parallel()
	fake := releasedFake()
	newerFormula := strings.Replace(testFormula, "0.24.11", "0.24.12", 1)
	newerManifest := `{"version": "0.24.12"}`
	fake.remote = channelReposOf(fake.remote, tapHolding(newerFormula), bucketHolding(newerManifest))

	result := runTestPublish(t, fake, false)

	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	lines := commandLines(result.commands)
	for _, unwanted := range []string{channelsCallName + " " + methodPublishFile} {
		if index := lineIndex(lines, unwanted); index >= 0 {
			t.Errorf("ran %q over a channel pinned to a newer version", lines[index])
		}
	}
	for step, detail := range map[string]string{stepHomebrew: "formula pins newer 0.24.12", stepScoop: "manifest pins newer 0.24.12"} {
		if !hasRow(result.output, step, "done", detail) {
			t.Errorf("the %s row does not report %q as done:\n%s", step, detail, result.output)
		}
	}
}

// TestPublishChannelsConvergeFromRemoteStateOnceTheTagExists pins the resume for the channel
// steps: days later, with VERSION moved on and a dirty tree — either of which refuses a run that
// consults them — publish still updates the pending tap without consulting the gate or VERSION at
// HEAD, and pushes nothing to main.
func TestPublishChannelsConvergeFromRemoteStateOnceTheTagExists(t *testing.T) {
	t.Parallel()
	fake := releasedFake()
	fake.remote = channelReposOf(fake.remote, bucketHolding(`{"version": "0.24.11"}`))
	fake.repo.isDirty = true
	fake.repo.files[versionFile] = "v0.24.12\n"

	result := runTestPublish(t, fake, false)

	if result.err != nil {
		t.Fatalf("publish failed: %v\n%s", result.err, result.output)
	}
	if index := lineIndex(commandLines(result.commands), "repo PushMain"); index >= 0 {
		t.Errorf("pushed main once the tag exists")
	}
	if pushed := publishedRepos(fake.remote); !slices.Equal(pushed, []string{tapRepo}) {
		t.Errorf("pushed to %q, want only the pending tap", pushed)
	}
}

// TestPublishChannelStepFailureStopsTheRun characterises a tap or bucket the publish fails for:
// publish stops at that channel's step with an error naming the step and the failure, prints the
// step table as the remote now holds it, and never reaches the smoke step. Which step of the
// publish itself failed — the clone, the history read, the commit or the push — is
// execChannelRepos' to name (channelrepos_test.go).
func TestPublishChannelStepFailureStopsTheRun(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		repo      string
		wantError string
	}{
		"tap": {
			repo:      tapRepo,
			wantError: "the homebrew step failed: push " + formulaPath + " to " + tapRepo + ": remote rejected the request",
		},
		"bucket": {
			repo:      bucketRepo,
			wantError: "the scoop step failed: push " + bucketPath + " to " + bucketRepo + ": remote rejected the request",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := releasedFake()
			if tc.repo == bucketRepo {
				fake.remote = channelReposOf(fake.remote, tapHolding(testFormula))
			}
			failure := fmt.Errorf("push %s to %s: remote rejected the request", channelFile[tc.repo], tc.repo)
			fake.remote = fake.remote.withFailure(methodPublishFile, tc.repo, failure)

			result := runTestPublish(t, fake, false)

			if result.err == nil || !strings.Contains(result.err.Error(), tc.wantError) {
				t.Fatalf("publish error = %v, want one naming %q", result.err, tc.wantError)
			}
			if !strings.Contains(result.output, "steps") {
				t.Errorf("the step table was not printed:\n%s", result.output)
			}
			if len(result.smokes) != 0 {
				t.Errorf("the smoke step ran %d times after the %s step failed", len(result.smokes), tc.repo)
			}
		})
	}
}
