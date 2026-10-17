package rig

import (
	"slices"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/config"
)

func TestLaunchEnv(t *testing.T) {
	t.Parallel()
	vars := []string{"PHOTON_WORK=/rig/work", "PHOTON_PORT_BACKEND=4000"}
	tests := []struct {
		name    string
		environ []string
		launch  config.Launch
		want    []string
	}{
		{
			name:    "a strip glob removes every variable it matches, and nothing else",
			environ: []string{"APP_CONFIG=~/.app", "APP_KEY=secret", "APPLE=1", "HOME=/home/me"},
			launch:  config.Launch{Strip: []string{"APP_*"}},
			want:    []string{"APPLE=1", "HOME=/home/me", "PHOTON_WORK=/rig/work", "PHOTON_PORT_BACKEND=4000"},
		},
		{
			name:    "an exact strip name removes that variable",
			environ: []string{"TOKEN=secret", "TOKENS=kept"},
			launch:  config.Launch{Strip: []string{"TOKEN"}},
			want:    []string{"TOKENS=kept", "PHOTON_WORK=/rig/work", "PHOTON_PORT_BACKEND=4000"},
		},
		{
			name:    "launch env expands PHOTON references and nothing else",
			environ: []string{"HOME=/home/me"},
			launch: config.Launch{Env: map[string]string{
				"APP_HOME": "${PHOTON_WORK}/home",
				"APP_URL":  "http://127.0.0.1:${PHOTON_PORT_BACKEND}",
				"RAW":      "$HOME and ${HOME}",
			}},
			want: []string{
				"HOME=/home/me",
				"APP_HOME=/rig/work/home", "APP_URL=http://127.0.0.1:4000", "RAW=$HOME and ${HOME}",
				"PHOTON_WORK=/rig/work", "PHOTON_PORT_BACKEND=4000",
			},
		},
		{
			name:    "launch env wins over the operator's value of the same name",
			environ: []string{"LANG=de_DE.UTF-8", "HOME=/home/me"},
			launch:  config.Launch{Env: map[string]string{"LANG": "C.UTF-8"}},
			want:    []string{"HOME=/home/me", "LANG=C.UTF-8", "PHOTON_WORK=/rig/work", "PHOTON_PORT_BACKEND=4000"},
		},
		{
			name:    "photon's own work dir wins over the operator's and launch env's",
			environ: []string{"PHOTON_WORK=/elsewhere"},
			launch:  config.Launch{Env: map[string]string{"PHOTON_WORK": "/also/elsewhere"}},
			want:    []string{"PHOTON_WORK=/rig/work", "PHOTON_PORT_BACKEND=4000"},
		},
		{
			name:    "a strip glob does not remove what launch env sets",
			environ: []string{"APP_MODE=dev"},
			launch:  config.Launch{Strip: []string{"APP_*"}, Env: map[string]string{"APP_MODE": "demo"}},
			want:    []string{"APP_MODE=demo", "PHOTON_WORK=/rig/work", "PHOTON_PORT_BACKEND=4000"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := launchEnv(test.environ, test.launch, vars)

			if err != nil {
				t.Fatalf("launchEnv: %v", err)
			}
			if !slices.Equal(got, test.want) {
				t.Errorf("launchEnv =\n  %q\nwant\n  %q", got, test.want)
			}
		})
	}
}

func TestLaunchEnv_UnsetReferenceFails(t *testing.T) {
	t.Parallel()
	launch := config.Launch{Env: map[string]string{"APP_URL": "http://127.0.0.1:${PHOTON_PORT_NOPE}"}}

	_, err := launchEnv(nil, launch, []string{"PHOTON_WORK=/rig/work"})

	if err == nil {
		t.Fatal("launchEnv succeeded with a reference to an unset variable")
	}
	for _, want := range []string{"launch env APP_URL", "PHOTON_PORT_NOPE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}
