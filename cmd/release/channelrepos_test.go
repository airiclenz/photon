package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestExecChannelReposRunsEachMethodsArgv pins the gh argv behind every execChannelRepos method,
// run in its directory, and what each makes of the answer: a yes, and the "no" a 404 — a missing
// repository or path, or an empty repository — reads as.
func TestExecChannelReposRunsEachMethodsArgv(t *testing.T) {
	t.Parallel()

	tapRepoCall := recordedCall{name: "gh", args: []string{"api", "repos/airiclenz/homebrew-tap", "--jq", ".full_name"}}
	formulaCall := recordedCall{name: "gh", args: []string{"api", "repos/airiclenz/homebrew-tap/contents/Formula/photon.rb", "--jq", ".content"}}
	exists := func(ctx context.Context, channels execChannelRepos) (any, error) {
		return channels.Exists(ctx, tapRepo)
	}
	readFile := func(ctx context.Context, channels execChannelRepos) (any, error) {
		content, isFound, err := channels.ReadFile(ctx, tapRepo, formulaPath)
		return []any{content, isFound}, err
	}

	cases := map[string]struct {
		call     func(ctx context.Context, channels execChannelRepos) (any, error)
		output   string
		err      error
		want     any
		wantCall recordedCall
	}{
		"Exists yes":                      {call: exists, output: tapRepo + "\n", want: true, wantCall: tapRepoCall},
		"Exists no":                       {call: exists, err: errors.New(ghNotFound), want: false, wantCall: tapRepoCall},
		"ReadFile yes":                    {call: readFile, output: contentsAPIBase64(testFormula), want: []any{testFormula, true}, wantCall: formulaCall},
		"ReadFile no":                     {call: readFile, err: errors.New(ghNotFound), want: []any{"", false}, wantCall: formulaCall},
		"ReadFile in an empty repository": {call: readFile, err: errors.New(ghEmptyRepository), want: []any{"", false}, wantCall: formulaCall},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			commands := answering(tc.output, tc.err)
			channels := execChannelRepos{commands: commands, dir: "/work"}

			got, err := tc.call(t.Context(), channels)

			if err != nil {
				t.Fatalf("%s failed: %v", name, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s = %#v, want %#v", name, got, tc.want)
			}
			wantCall := tc.wantCall
			wantCall.dir = "/work"
			if len(commands.calls) != 1 || !reflect.DeepEqual(commands.calls[0], wantCall) {
				t.Errorf("calls = %+v, want exactly %+v", commands.calls, wantCall)
			}
		})
	}
}

// TestExecChannelReposTellsANoFromABrokenCommand pins execChannelRepos' classification: a gh
// failure other than a 404 is the runner's error, and an answer the adapter cannot parse is an
// error wrapping errUnparsableAnswer that reads as the parser's own message.
func TestExecChannelReposTellsANoFromABrokenCommand(t *testing.T) {
	t.Parallel()
	brokenCall := errors.New("exit status 1: gh: Server Error (HTTP 500)")

	cases := map[string]struct {
		call           func(ctx context.Context, channels execChannelRepos) error
		output         string
		err            error
		wantError      string
		wantUnparsable bool
	}{
		"Exists on a broken call": {
			call: func(ctx context.Context, channels execChannelRepos) error {
				_, err := channels.Exists(ctx, bucketRepo)
				return err
			},
			err:       brokenCall,
			wantError: brokenCall.Error(),
		},
		"ReadFile on a broken call": {
			call: func(ctx context.Context, channels execChannelRepos) error {
				_, _, err := channels.ReadFile(ctx, bucketRepo, bucketPath)
				return err
			},
			err:       brokenCall,
			wantError: brokenCall.Error(),
		},
		"ReadFile on content that is not base64": {
			call: func(ctx context.Context, channels execChannelRepos) error {
				_, _, err := channels.ReadFile(ctx, bucketRepo, bucketPath)
				return err
			},
			output:         "not base64!\n",
			wantError:      "decode bucket/photon.json: illegal base64 data",
			wantUnparsable: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			channels := execChannelRepos{commands: answering(tc.output, tc.err), dir: "."}

			err := tc.call(t.Context(), channels)

			if err == nil || !strings.HasPrefix(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want one starting %q", err, tc.wantError)
			}
			if errors.Is(err, errUnparsableAnswer) != tc.wantUnparsable {
				t.Errorf("errors.Is(%v, errUnparsableAnswer) = %t, want %t", err, !tc.wantUnparsable, tc.wantUnparsable)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Errorf("error %v does not wrap the runner's %v", err, tc.err)
			}
		})
	}
}

// publishFileAnswers is what the fake clone answers PublishFile with: the history `git rev-list`
// prints, the paths `git diff --cached` names, and the one command made to fail with failure.
type publishFileAnswers struct {
	history  string
	staged   string
	failing  string
	failure  error
	contents map[string]string
}

// publishFileRunner answers PublishFile's commands from answers, recording what the clone holds
// at path when the commit runs.
func publishFileRunner(t *testing.T, answers *publishFileAnswers, path string) *fakeRunner {
	t.Helper()
	return &fakeRunner{respond: func(call recordedCall) (string, error) {
		command := call.name + " " + call.args[0]
		if command == answers.failing {
			return "", answers.failure
		}
		switch command {
		case "git rev-list":
			return answers.history, nil
		case "git diff":
			return answers.staged, nil
		case "git commit":
			content, err := os.ReadFile(filepath.Join(call.dir, filepath.FromSlash(path)))
			if err != nil {
				return "", err
			}
			answers.contents[call.dir] = string(content)
		}
		return "", nil
	}}
}

// TestExecChannelReposPublishFileRunsTheCloneArgv pins PublishFile's gh and git argv: the clone
// into a temporary directory, the history read, the write staged at its path and compared, and
// only for a changed file the commit and the push — of HEAD, or of HEAD to main in a repository
// with no commits yet.
func TestExecChannelReposPublishFileRunsTheCloneArgv(t *testing.T) {
	t.Parallel()
	const subject = "photon 0.24.11"

	cases := map[string]struct {
		history    string
		staged     string
		wantPushed bool
		// wantTail is the commands after `git diff --cached --name-only`, in the clone.
		wantTail []recordedCall
	}{
		"a changed file": {history: "81eec1b3ac7d27f7ef38fe2368b59b89cbd8832b\n", staged: bucketPath + "\n", wantPushed: true,
			wantTail: []recordedCall{
				{name: "git", args: []string{"commit", "--quiet", "-m", subject}},
				{env: nonInteractiveEnv, name: "git", args: []string{"push", "--quiet", "origin", "HEAD"}},
			}},
		"an unchanged file": {history: "81eec1b3ac7d27f7ef38fe2368b59b89cbd8832b\n", staged: "\n", wantPushed: false},
		"an empty repository": {history: "", staged: bucketPath + "\n", wantPushed: true,
			wantTail: []recordedCall{
				{name: "git", args: []string{"commit", "--quiet", "-m", subject}},
				{env: nonInteractiveEnv, name: "git", args: []string{"push", "--quiet", "origin", "HEAD:main"}},
			}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			answers := &publishFileAnswers{history: tc.history, staged: tc.staged, contents: map[string]string{}}
			commands := publishFileRunner(t, answers, bucketPath)
			channels := execChannelRepos{commands: commands, dir: "/work"}

			isPushed, err := channels.PublishFile(t.Context(), bucketRepo, bucketPath, testFormula, subject)

			if err != nil {
				t.Fatalf("PublishFile failed: %v", err)
			}
			if isPushed != tc.wantPushed {
				t.Errorf("PublishFile pushed = %t, want %t", isPushed, tc.wantPushed)
			}
			if len(commands.calls) == 0 || len(commands.calls[0].args) < 4 {
				t.Fatalf("calls = %+v, want the clone first", commands.calls)
			}
			cloneDir := commands.calls[0].args[3]
			wantCalls := []recordedCall{
				{dir: filepath.Dir(cloneDir), env: nonInteractiveEnv, name: "gh", args: []string{"repo", "clone", bucketRepo, cloneDir, "--", "--quiet"}},
				{dir: cloneDir, name: "git", args: []string{"rev-list", "--all", "--max-count=1"}},
				{dir: cloneDir, name: "git", args: []string{"add", "--", bucketPath}},
				{dir: cloneDir, name: "git", args: []string{"diff", "--cached", "--name-only"}},
			}
			for _, call := range tc.wantTail {
				call.dir = cloneDir
				wantCalls = append(wantCalls, call)
			}
			if !reflect.DeepEqual(commands.calls, wantCalls) {
				t.Errorf("calls = %+v, want exactly %+v", commands.calls, wantCalls)
			}
			if tc.wantPushed && answers.contents[cloneDir] != testFormula {
				t.Errorf("the clone's %s at the commit = %q, want the content given", bucketPath, answers.contents[cloneDir])
			}
			if _, err := os.Stat(filepath.Dir(cloneDir)); !os.IsNotExist(err) {
				t.Errorf("the clone directory %s was left behind: %v", filepath.Dir(cloneDir), err)
			}
		})
	}
}

// TestExecChannelReposPublishFileNamesTheFailingStep pins PublishFile's failures: a clone,
// history read, commit or push that fails is an error naming the repository and the step and
// wrapping the runner's error, and nothing after the failing step runs.
func TestExecChannelReposPublishFileNamesTheFailingStep(t *testing.T) {
	t.Parallel()
	failure := errors.New("exit status 1")

	cases := map[string]struct {
		failing   string
		wantError string
	}{
		"clone":   {failing: "gh repo", wantError: "clone " + tapRepo + ": "},
		"history": {failing: "git rev-list", wantError: "read " + tapRepo + "'s history: "},
		"commit":  {failing: "git commit", wantError: "commit " + formulaPath + " to " + tapRepo + ": "},
		"push":    {failing: "git push", wantError: "push " + formulaPath + " to " + tapRepo + ": "},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			answers := &publishFileAnswers{history: "81eec1b\n", staged: formulaPath + "\n", failing: tc.failing,
				failure: failure, contents: map[string]string{}}
			commands := publishFileRunner(t, answers, formulaPath)
			channels := execChannelRepos{commands: commands, dir: "/work"}

			isPushed, err := channels.PublishFile(t.Context(), tapRepo, formulaPath, testFormula, "photon 0.24.11")

			if err == nil || !strings.HasPrefix(err.Error(), tc.wantError) {
				t.Fatalf("PublishFile error = %v, want one starting %q", err, tc.wantError)
			}
			if !errors.Is(err, failure) {
				t.Errorf("error %v does not wrap the runner's %v", err, failure)
			}
			if isPushed {
				t.Error("PublishFile reported a push after a failed step")
			}
			last := commands.calls[len(commands.calls)-1]
			if got := last.name + " " + last.args[0]; got != tc.failing {
				t.Errorf("the last command run = %q, want the failing %q", got, tc.failing)
			}
		})
	}
}
