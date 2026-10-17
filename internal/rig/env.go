package rig

import (
	"fmt"
	"path"
	"strings"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/service"
)

// launchEnv is the launch's whole environment: environ, the operator's, less every variable a
// strip glob of launch matches, then launch's env with its ${PHOTON_*} references expanded from
// vars, then vars themselves — photon's own PHOTON_WORK and the services' ports — which win over
// an entry of the same name. Each variable appears once.
func launchEnv(environ []string, launch config.Launch, vars []string) ([]string, error) {
	env, err := service.ExpandEnv(launch.Env, vars)
	if err != nil {
		return nil, fmt.Errorf("launch %w", err)
	}
	return withVars(withVars(stripped(environ, launch.Strip), env), vars), nil
}

// stripped is environ less every entry whose name matches one of globs.
func stripped(environ, globs []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		if !matchesAny(nameOf(entry), globs) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// matchesAny reports whether name matches one of globs. A malformed glob, which a loaded config
// never holds, matches nothing.
func matchesAny(name string, globs []string) bool {
	for _, glob := range globs {
		if matched, err := path.Match(glob, name); err == nil && matched {
			return true
		}
	}
	return false
}

// withVars is environ with every entry of vars set over it: an environ entry vars also names is
// dropped, and vars follow in their own order.
func withVars(environ, vars []string) []string {
	set := make(map[string]bool, len(vars))
	for _, entry := range vars {
		set[nameOf(entry)] = true
	}
	merged := make([]string, 0, len(environ)+len(vars))
	for _, entry := range environ {
		if !set[nameOf(entry)] {
			merged = append(merged, entry)
		}
	}
	return append(merged, vars...)
}

// nameOf is the variable name of an environ entry.
func nameOf(entry string) string {
	name, _, _ := strings.Cut(entry, "=")
	return name
}
