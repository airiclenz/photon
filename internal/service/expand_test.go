package service

import (
	"slices"
	"strings"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	vars := []string{"PHOTON_WORK=/work", "PHOTON_PORT_API=4100", "PHOTON_PORT=4200", "PHOTON_PORT=4300"}
	tests := []struct {
		name      string
		value     string
		want      string
		wantError string
	}{
		{"plain value", "plain", "plain", ""},
		{"one reference", "${PHOTON_WORK}/home", "/work/home", ""},
		{"two references", "http://localhost:${PHOTON_PORT_API}${PHOTON_WORK}", "http://localhost:4100/work", ""},
		{"later entry wins", "${PHOTON_PORT}", "4300", ""},
		{"non-photon reference stays", "${HOME}/x", "${HOME}/x", ""},
		{"unbraced reference stays", "$PHOTON_WORK", "$PHOTON_WORK", ""},
		{"empty value", "", "", ""},
		{"unset photon reference", "${PHOTON_PORT_LATER}", "", "env V: ${PHOTON_PORT_LATER} is not set here"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ExpandEnv(map[string]string{"V": test.value}, vars)

			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("ExpandEnv error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpandEnv: %v", err)
			}
			if want := []string{"V=" + test.want}; !slices.Equal(got, want) {
				t.Errorf("ExpandEnv = %q, want %q", got, want)
			}
		})
	}
}

func TestExpandEnv_ListsEntriesInNameOrder(t *testing.T) {
	env := map[string]string{"ZETA": "z", "ALPHA": "${PHOTON_WORK}", "MID": "m"}

	got, err := ExpandEnv(env, []string{"PHOTON_WORK=/work"})

	if err != nil {
		t.Fatalf("ExpandEnv: %v", err)
	}
	if want := "ALPHA=/work MID=m ZETA=z"; strings.Join(got, " ") != want {
		t.Errorf("ExpandEnv = %q, want %q", got, want)
	}
}
