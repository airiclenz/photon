package service

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// photonReference is a ${PHOTON_*} reference in a config env value; group 1 is the variable name.
var photonReference = regexp.MustCompile(`\$\{(PHOTON_[A-Za-z0-9_]*)\}`)

// ExpandEnv turns a config env map into environ entries (NAME=value), in name order, with every
// ${PHOTON_*} reference in a value replaced by that variable's value in vars, itself a list of
// environ entries such as [Set.Env]. Nothing else is expanded: $PHOTON_WORK without braces,
// ${HOME} and every other $ stay as written, for the shell the command runs in to read from its
// env. A reference to a PHOTON_ variable vars does not hold is an error naming the env entry and
// the variable, so a typo or a service's own later port never reaches a command as an empty
// string.
func ExpandEnv(env map[string]string, vars []string) ([]string, error) {
	values := environValues(vars)
	expanded := make([]string, 0, len(env))
	for _, name := range slices.Sorted(maps.Keys(env)) {
		value, err := expand(env[name], values)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", name, err)
		}
		expanded = append(expanded, name+"="+value)
	}
	return expanded, nil
}

// expand replaces every ${PHOTON_*} reference in value by its entry in values.
func expand(value string, values map[string]string) (string, error) {
	var missing string
	expanded := photonReference.ReplaceAllStringFunc(value, func(reference string) string {
		name := photonReference.FindStringSubmatch(reference)[1]
		replacement, isSet := values[name]
		if !isSet && missing == "" {
			missing = name
		}
		return replacement
	})
	if missing != "" {
		return "", fmt.Errorf("${%s} is not set here", missing)
	}
	return expanded, nil
}

// environValues maps each environ entry's name to its value; a later entry wins, as in a process
// env.
func environValues(environ []string) map[string]string {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	return values
}
