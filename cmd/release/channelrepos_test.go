package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	listingCall := recordedCall{name: "gh", args: []string{"api", "repos/microsoft/winget-pkgs/contents/manifests/a/AiricLenz/Photon"}}
	exists := func(ctx context.Context, channels execChannelRepos) (any, error) {
		return channels.Exists(ctx, tapRepo)
	}
	readFile := func(ctx context.Context, channels execChannelRepos) (any, error) {
		content, isFound, err := channels.ReadFile(ctx, tapRepo, formulaPath)
		return []any{content, isFound}, err
	}
	listDirs := func(ctx context.Context, channels execChannelRepos) (any, error) {
		dirs, isFound, err := channels.ListDirs(ctx, "microsoft/winget-pkgs", "manifests/a/AiricLenz/Photon")
		return []any{dirs, isFound}, err
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
		"ListDirs yes keeps only the directories": {call: listDirs,
			output:   `[{"name":"0.24.10","type":"dir"},{"name":"README.md","type":"file"},{"name":"0.24.11","type":"dir"}]` + "\n",
			want:     []any{[]string{"0.24.10", "0.24.11"}, true},
			wantCall: listingCall},
		"ListDirs no":                     {call: listDirs, err: errors.New(ghNotFound), want: []any{[]string(nil), false}, wantCall: listingCall},
		"ListDirs in an empty repository": {call: listDirs, err: errors.New(ghEmptyRepository), want: []any{[]string(nil), false}, wantCall: listingCall},
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

// TestExecChannelReposOpenPullsReadsTheViewersPullsBesideTheSearch pins OpenPulls' two gh
// calls — the search listing of the signed-in account's own pull requests, then its open pull
// requests through GraphQL, which does not wait on the search index — and how it merges them: the
// viewer's pull requests in the repository alone, each pull request once.
func TestExecChannelReposOpenPullsReadsTheViewersPullsBesideTheSearch(t *testing.T) {
	t.Parallel()
	const pullsRepo = "microsoft/winget-pkgs"

	wantCalls := []recordedCall{
		{dir: "/work", name: "gh", args: []string{"pr", "list", "--repo", pullsRepo, "--state", "open",
			"--author", "@me", "--search", "AiricLenz.Photon 0.24.11 in:title", "--json", "number,title,url"}},
		{dir: "/work", env: nonInteractiveEnv, name: "gh", args: []string{"api", "graphql", "-f",
			"query=query { viewer { pullRequests(states: OPEN, first: 100, orderBy: {field: CREATED_AT, direction: DESC}) " +
				"{ nodes { number title url repository { nameWithOwner } } } } }"}},
	}
	pull := func(number int) pullRequest {
		return pullRequest{number: number, title: "New version: AiricLenz.Photon version 0.24.11",
			url: fmt.Sprintf("https://github.com/microsoft/winget-pkgs/pull/%d", number)}
	}

	cases := map[string]struct {
		search string
		viewer string
		want   []pullRequest
	}{
		"none":                 {search: "[]\n", viewer: viewerPullsAnswer(0, ""), want: []pullRequest{}},
		"found by search only": {search: searchedPullsAnswer(312345), viewer: viewerPullsAnswer(0, ""), want: []pullRequest{pull(312345)}},
		"the viewer's pull request search has not indexed": {search: "[]\n",
			viewer: viewerPullsAnswer(312346, pullsRepo), want: []pullRequest{pull(312346)}},
		"one pull request both find, once": {search: searchedPullsAnswer(312345),
			viewer: viewerPullsAnswer(312345, pullsRepo), want: []pullRequest{pull(312345)}},
		"the viewer's pull request in another repository left out": {search: "[]\n",
			viewer: viewerPullsAnswer(312346, "airiclenz/winget-pkgs"), want: []pullRequest{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			commands := pullsAnswering(tc.search, tc.viewer, nil)
			channels := execChannelRepos{commands: commands, dir: "/work"}

			got, err := channels.OpenPulls(t.Context(), pullsRepo, "AiricLenz.Photon 0.24.11 in:title")

			if err != nil {
				t.Fatalf("OpenPulls failed: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("OpenPulls = %#v, want %#v", got, tc.want)
			}
			if !reflect.DeepEqual(commands.calls, wantCalls) {
				t.Errorf("calls = %+v, want exactly %+v", commands.calls, wantCalls)
			}
		})
	}
}

// searchedPullsAnswer is the `gh pr list --json number,title,url` answer listing one pull request.
func searchedPullsAnswer(number int) string {
	return fmt.Sprintf(`[{"number":%d,"title":"New version: AiricLenz.Photon version 0.24.11",`+
		`"url":"https://github.com/microsoft/winget-pkgs/pull/%d"}]`+"\n", number, number)
}

// viewerPullsAnswer is the `gh api graphql` answer listing the signed-in account's open pull
// requests: one against repo, or none when number is 0.
func viewerPullsAnswer(number int, repo string) string {
	nodes := ""
	if number != 0 {
		nodes = fmt.Sprintf(`{"number":%d,"title":"New version: AiricLenz.Photon version 0.24.11",`+
			`"url":"https://github.com/microsoft/winget-pkgs/pull/%d","repository":{"nameWithOwner":%q}}`,
			number, number, repo)
	}
	return `{"data":{"viewer":{"pullRequests":{"nodes":[` + nodes + "]}}}}\n"
}

// pullsAnswering answers OpenPulls' two calls: `gh pr list` with search, and `gh api graphql`
// with viewer, or with viewerErr when it is set.
func pullsAnswering(search, viewer string, viewerErr error) *fakeRunner {
	return &fakeRunner{respond: func(call recordedCall) (string, error) {
		if call.args[0] == "pr" {
			return search, nil
		}
		return viewer, viewerErr
	}}
}

// TestExecChannelReposTellsANoFromABrokenCommand pins execChannelRepos' classification: a gh
// failure other than a 404 is the runner's error, and an answer the adapter cannot parse is an
// error wrapping errUnparsableAnswer that reads as the parser's own message.
func TestExecChannelReposTellsANoFromABrokenCommand(t *testing.T) {
	t.Parallel()
	brokenCall := errors.New("exit status 1: gh: Server Error (HTTP 500)")
	listDirs := func(ctx context.Context, channels execChannelRepos) error {
		_, _, err := channels.ListDirs(ctx, "microsoft/winget-pkgs", "manifests/a/AiricLenz/Photon")
		return err
	}
	openPulls := func(ctx context.Context, channels execChannelRepos) error {
		_, err := channels.OpenPulls(ctx, "microsoft/winget-pkgs", "AiricLenz.Photon")
		return err
	}

	cases := map[string]struct {
		call func(ctx context.Context, channels execChannelRepos) error
		// commands answers the calls when set; otherwise every call answers output and err.
		commands       *fakeRunner
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
		"ListDirs on a broken call": {call: listDirs, err: brokenCall, wantError: brokenCall.Error()},
		"ListDirs on a listing that is not JSON": {
			call:           listDirs,
			output:         "not json",
			wantError:      "invalid character 'o' in literal null (expecting 'u')",
			wantUnparsable: true,
		},
		"OpenPulls on a broken call": {call: openPulls, err: brokenCall, wantError: brokenCall.Error()},
		"OpenPulls on output that is not JSON": {
			call:           openPulls,
			output:         "not json",
			wantError:      "invalid character 'o' in literal null (expecting 'u')",
			wantUnparsable: true,
		},
		"OpenPulls on a broken viewer query": {
			call:      openPulls,
			commands:  pullsAnswering("[]\n", "", brokenCall),
			err:       brokenCall,
			wantError: brokenCall.Error(),
		},
		"OpenPulls on a viewer answer that is not JSON": {
			call:           openPulls,
			commands:       pullsAnswering("[]\n", "not json", nil),
			wantError:      "invalid character 'o' in literal null (expecting 'u')",
			wantUnparsable: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			commands := tc.commands
			if commands == nil {
				commands = answering(tc.output, tc.err)
			}
			channels := execChannelRepos{commands: commands, dir: "."}

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

// TestExecChannelReposHasPackageReadsTheRegistry pins HasPackage's npm argv, run in its
// directory, and what it makes of the answer: the version printed is a yes, an empty answer (a
// version of the package the registry lacks) and E404 (a package it lacks) are a no, a missing npm
// is an error wrapping exec.ErrNotFound, and any other failure is an error.
func TestExecChannelReposHasPackageReadsTheRegistry(t *testing.T) {
	t.Parallel()
	wantCall := recordedCall{dir: "/work", name: "npm", args: []string{
		"view", "@airiclenz/photon@0.24.11", "version", "--prefer-online", "--registry", "https://registry.npmjs.org/",
	}}
	notFound := errors.New("npm view @airiclenz/photon@0.24.11 version: exit status 1: npm error code E404\n" +
		"npm error 404 Not Found - GET https://registry.npmjs.org/@airiclenz%2fphoton - Not found")
	offline := errors.New("npm view @airiclenz/photon@0.24.11 version: exit status 1: npm error code ENOTFOUND")
	missing := fmt.Errorf("npm view: %w", &exec.Error{Name: "npm", Err: exec.ErrNotFound})

	cases := map[string]struct {
		output         string
		err            error
		want           bool
		wantErr        string
		wantMissingNpm bool
	}{
		"the version is published":       {output: "0.24.11\n", want: true},
		"the package lacks the version":  {output: "", want: false},
		"the registry lacks the package": {err: notFound, want: false},
		"the registry cannot be reached": {err: offline, wantErr: "ENOTFOUND"},
		"npm is not installed":           {err: missing, wantErr: "executable file not found", wantMissingNpm: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			commands := answering(tc.output, tc.err)
			channels := execChannelRepos{commands: commands, dir: "/work"}

			got, err := channels.HasPackage(t.Context(), npmWrapperName, "0.24.11")

			if tc.wantErr == "" && err != nil {
				t.Fatalf("HasPackage failed: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("HasPackage error = %v, want one containing %q", err, tc.wantErr)
			}
			if isMissing := errors.Is(err, exec.ErrNotFound); isMissing != tc.wantMissingNpm {
				t.Errorf("HasPackage error wraps exec.ErrNotFound: %t, want %t", isMissing, tc.wantMissingNpm)
			}
			if got != tc.want {
				t.Errorf("HasPackage = %t, want %t", got, tc.want)
			}
			if len(commands.calls) != 1 || !reflect.DeepEqual(commands.calls[0], wantCall) {
				t.Errorf("calls = %+v, want exactly %+v", commands.calls, wantCall)
			}
		})
	}
}

// TestExecChannelReposPublishPackageKeepsTheTokenOutOfArgv pins PublishPackage's npm argv: the
// package directory published publicly to the registry through a temporary userconfig that names
// NPM_TOKEN rather than holding the token, the token in npm's environment alone, and the
// userconfig removed once npm has run; a failing npm's error names the package with the token it
// echoed redacted.
func TestExecChannelReposPublishPackageKeepsTheTokenOutOfArgv(t *testing.T) {
	t.Parallel()

	for _, isFailing := range []bool{false, true} {
		t.Run(fmt.Sprintf("failing %t", isFailing), func(t *testing.T) {
			t.Parallel()
			var userconfig, userconfigContent string
			commands := &fakeRunner{respond: func(call recordedCall) (string, error) {
				userconfig = call.args[len(call.args)-1]
				content, err := os.ReadFile(userconfig)
				if err != nil {
					return "", err
				}
				userconfigContent = string(content)
				if isFailing {
					return "", fmt.Errorf("npm publish: exit status 1: npm error code E401 bad token %s", sentinelToken)
				}
				return "", nil
			}}
			channels := execChannelRepos{commands: commands, dir: "/work"}

			err := channels.PublishPackage(t.Context(), npmWrapperName, "0.24.11", "/stage/photon", sentinelToken)

			if (err != nil) != isFailing {
				t.Fatalf("PublishPackage error = %v, want an error: %t", err, isFailing)
			}
			if isFailing && (strings.Contains(err.Error(), sentinelToken) ||
				!strings.Contains(err.Error(), "publish @airiclenz/photon@0.24.11 to the npm registry") ||
				!strings.Contains(err.Error(), redactedToken)) {
				t.Errorf("PublishPackage error = %q, want it naming the package with the token redacted", err)
			}
			if len(commands.calls) != 1 {
				t.Fatalf("calls = %+v, want exactly one npm publish", commands.calls)
			}
			call := commands.calls[0]
			wantArgs := []string{"publish", "/stage/photon", "--access", "public",
				"--registry", "https://registry.npmjs.org/", "--userconfig", userconfig}
			if call.name != "npm" || call.dir != "/work" || !reflect.DeepEqual(call.args, wantArgs) {
				t.Errorf("call = %s %q in %s, want npm %q in /work", call.name, call.args, call.dir, wantArgs)
			}
			if want := []string{"NPM_TOKEN=" + sentinelToken}; !reflect.DeepEqual(call.env, want) {
				t.Errorf("env = %q, want %q", call.env, want)
			}
			if want := "//registry.npmjs.org/:_authToken=${NPM_TOKEN}\n"; userconfigContent != want {
				t.Errorf("userconfig = %q, want %q", userconfigContent, want)
			}
			if _, statErr := os.Stat(userconfig); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("the userconfig %s outlived the publish: %v", userconfig, statErr)
			}
		})
	}
}
