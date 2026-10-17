package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// errBoom is the failure the throwaway subcommand's body returns.
var errBoom = errors.New("boom")

// usageHeading opens Cobra's usage text.
const usageHeading = "Usage:"

func TestExitCodeFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "no error", err: nil, want: exitOK},
		{name: "run failure", err: runError{err: errBoom}, want: exitRunFailed},
		{name: "wrapped run failure", err: fmt.Errorf("context: %w", runError{err: errBoom}), want: exitRunFailed},
		{name: "command-line error", err: errors.New("unknown flag: --bogus"), want: exitBadUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := exitCodeFor(tt.err)

			if got != tt.want {
				t.Errorf("exitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
		wantUsage  bool
	}{
		{name: "help", args: []string{"--help"}, wantCode: exitOK, wantUsage: true},
		{
			name:       "unknown flag",
			args:       []string{"--bogus"},
			wantCode:   exitBadUsage,
			wantStderr: "photon: unknown flag: --bogus\n",
			wantUsage:  true,
		},
		{
			name:       "subcommand given an argument it refuses",
			args:       []string{"fail", "extra"},
			wantCode:   exitBadUsage,
			wantStderr: "photon: unknown command \"extra\" for \"photon fail\"\n",
			wantUsage:  true,
		},
		{name: "subcommand body fails", args: []string{"fail"}, wantCode: exitRunFailed, wantStderr: "photon: boom\n"},
		{name: "subcommand body succeeds", args: []string{"pass"}, wantCode: exitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := newTestRoot()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)

			code := execute(context.Background(), root, tt.args, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", got, tt.wantStderr)
			}
			// Cobra writes the usage text to the command's output writer when one is set, so it
			// is looked for in stdout here; in production it reaches stderr.
			if got := strings.Contains(stdout.String(), usageHeading); got != tt.wantUsage {
				t.Errorf("usage printed = %t, want %t (stdout %q)", got, tt.wantUsage, stdout.String())
			}
		})
	}
}

// newTestRoot returns the real root command with two throwaway subcommands: `fail`, whose body
// returns errBoom, and `pass`, whose body succeeds. Neither accepts an argument.
func newTestRoot() *cobra.Command {
	root := newRootCommand()
	root.AddCommand(&cobra.Command{
		Use:  "fail",
		Args: cobra.NoArgs,
		RunE: runE(func(*cobra.Command, []string) error { return errBoom }),
	})
	root.AddCommand(&cobra.Command{
		Use:  "pass",
		Args: cobra.NoArgs,
		RunE: runE(func(*cobra.Command, []string) error { return nil }),
	})
	return root
}
