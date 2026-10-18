package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// channelRepos is the semantic seam over the package channels' GitHub repositories — the tap and
// the bucket: what the channel probes ask of them and what publish writes to them, in their own
// terms rather than as gh and git argv. execChannelRepos is the production adapter.
// A "no" answer — a repository or file that does not exist, or an empty repository —
// is never an error; every method fails with an error when its answer cannot be read, and one
// wrapping errUnparsableAnswer when the answer came back in a shape it cannot parse.
type channelRepos interface {
	// Exists reports whether the repository exists.
	Exists(ctx context.Context, repo string) (bool, error)
	// ReadFile returns the content of the file at path in the repository's default branch, and
	// whether there is one: false for a missing file, repository or commit.
	ReadFile(ctx context.Context, repo, path string) (string, bool, error)
	// PublishFile writes content to the file at path in the repository's branch and, only when
	// that changes the file, commits it with subject and pushes the commit straight to the branch
	// — no pull request. It reports whether it pushed a commit; every failure names the
	// repository and the step that failed.
	PublishFile(ctx context.Context, repo, path, content, subject string) (bool, error)
}

// errUnparsableAnswer marks a channel repository answer the adapter could not parse. The error
// carrying it reads as the parser's own message alone, so a caller wording the failure itself
// keeps the parser's detail.
var errUnparsableAnswer = errors.New("unparsable channel repository answer")

// unparsableAnswerError is a parse failure wrapping errUnparsableAnswer and the parser's error.
type unparsableAnswerError struct {
	cause error
}

func (e unparsableAnswerError) Error() string { return e.cause.Error() }

func (e unparsableAnswerError) Unwrap() []error { return []error{errUnparsableAnswer, e.cause} }

// execChannelRepos is the production channelRepos: it owns the gh argv for the channel
// repositories, run in dir, and the git argv of the clone PublishFile writes in, and reads
// GitHub's 404 — a missing repository or path, or an empty repository — as a "no".
type execChannelRepos struct {
	commands runner
	dir      string
}

// channelReposOrExec returns channels, or, when it is nil — a verb's run options built without
// one, as a test driving a run function directly builds them — the execChannelRepos over commands
// in dir.
func channelReposOrExec(channels channelRepos, commands runner, dir string) channelRepos {
	if channels == nil {
		return execChannelRepos{commands: commands, dir: dir}
	}
	return channels
}

// gh runs one gh command in dir.
func (r execChannelRepos) gh(ctx context.Context, args ...string) (string, error) {
	return r.commands.run(ctx, r.dir, nil, "gh", args...)
}

// Exists asks the repositories API for the repository's full name.
func (r execChannelRepos) Exists(ctx context.Context, repo string) (bool, error) {
	_, err := r.gh(ctx, "api", "repos/"+repo, "--jq", ".full_name")
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ReadFile reads the file through the contents API.
func (r execChannelRepos) ReadFile(ctx context.Context, repo, path string) (string, bool, error) {
	encoded, err := r.gh(ctx, "api", "repos/"+repo+"/contents/"+path, "--jq", ".content")
	if isNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	// The contents API wraps its base64 at 60 columns.
	decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(encoded), ""))
	if err != nil {
		return "", false, fmt.Errorf("decode %s: %w", path, unparsableAnswerError{cause: err})
	}
	return string(decoded), true, nil
}

// PublishFile clones the repository into a temporary directory, writes content to path there and,
// when `git diff --cached` shows the file changed, commits it and pushes HEAD. A repository with
// no commits yet has no branch to push to, so its first commit is pushed to main.
func (r execChannelRepos) PublishFile(
	ctx context.Context,
	repo, path, content, subject string,
) (bool, error) {
	workDir, err := os.MkdirTemp("", "photon-channel-")
	if err != nil {
		return false, fmt.Errorf("create the %s clone directory: %w", repo, err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	cloneDir := filepath.Join(workDir, "repo")
	if _, err := r.commands.run(
		ctx, workDir, nonInteractiveEnv, "gh", "repo", "clone", repo, cloneDir, "--", "--quiet",
	); err != nil {
		return false, fmt.Errorf("clone %s: %w", repo, err)
	}
	history, err := r.commands.run(ctx, cloneDir, nil, "git", "rev-list", "--all", "--max-count=1")
	if err != nil {
		return false, fmt.Errorf("read %s's history: %w", repo, err)
	}

	filePath := filepath.Join(cloneDir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(filePath), directoryMode); err != nil {
		return false, fmt.Errorf("create %s's directory in the %s clone: %w", path, repo, err)
	}
	if err := os.WriteFile(filePath, []byte(content), documentMode); err != nil {
		return false, fmt.Errorf("write %s in the %s clone: %w", path, repo, err)
	}
	if _, err := r.commands.run(ctx, cloneDir, nil, "git", "add", "--", path); err != nil {
		return false, fmt.Errorf("stage %s in the %s clone: %w", path, repo, err)
	}
	staged, err := r.commands.run(ctx, cloneDir, nil, "git", "diff", "--cached", "--name-only")
	if err != nil {
		return false, fmt.Errorf("compare %s with %s's: %w", path, repo, err)
	}
	if strings.TrimSpace(staged) == "" {
		return false, nil
	}

	if _, err := r.commands.run(
		ctx, cloneDir, nil, "git", "commit", "--quiet", "-m", subject,
	); err != nil {
		return false, fmt.Errorf("commit %s to %s: %w", path, repo, err)
	}
	pushRef := "HEAD"
	if strings.TrimSpace(history) == "" {
		pushRef = "HEAD:" + mainBranch
	}
	if _, err := r.commands.run(
		ctx, cloneDir, nonInteractiveEnv, "git", "push", "--quiet", remoteName, pushRef,
	); err != nil {
		return false, fmt.Errorf("push %s to %s: %w", path, repo, err)
	}
	return true, nil
}
