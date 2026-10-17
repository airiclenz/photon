package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/airiclenz/photon"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "version subcommand", args: []string{"version"}, wantCode: exitOK},
		{name: "version flag", args: []string{"--version"}, wantCode: exitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := newRootCommand()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)

			code := execute(context.Background(), root, tt.args, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}
			if want := photon.Version() + "\n"; stdout.String() != want {
				t.Errorf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
}
