package main

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
)

// readmePath is the README, relative to this package's directory, where go test runs.
const readmePath = "../../README.md"

// skippedTag is the yaml tag of a field that is never read from a file.
const skippedTag = "-"

// flagName finds each long flag in a command's flag usage text.
var flagName = regexp.MustCompile(`--([a-z][a-z-]*)`)

// TestREADMEDocumentsEveryField fails when a storyboard or config field is missing from the
// README's reference: every yaml tag the two schemas decode must appear there in code, as `tag`
// or as the last part of a dotted path such as `frame.tag`.
func TestREADMEDocumentsEveryField(t *testing.T) {
	readme := readREADME(t)
	for _, schema := range []struct {
		name string
		root reflect.Type
	}{
		{"storyboard", reflect.TypeFor[storyboard.Storyboard]()},
		{"config", reflect.TypeFor[config.Config]()},
	} {
		t.Run(schema.name, func(t *testing.T) {
			for _, tag := range yamlTags(schema.root) {
				if !strings.Contains(readme, "`"+tag+"`") && !strings.Contains(readme, "."+tag+"`") {
					t.Errorf("README.md does not document the %s field `%s`", schema.name, tag)
				}
			}
		})
	}
}

// TestREADMEDocumentsEveryCommand fails when a subcommand or one of its flags is missing from the
// README, so the README and each command's --help name the same surface.
func TestREADMEDocumentsEveryCommand(t *testing.T) {
	readme := readREADME(t)
	for _, cmd := range newRootCommand().Commands() {
		t.Run(cmd.Name(), func(t *testing.T) {
			if !strings.Contains(readme, "`"+programName+" "+cmd.Name()+" ") {
				t.Errorf("README.md does not document `%s %s`", programName, cmd.Name())
			}
			for _, match := range flagName.FindAllStringSubmatch(cmd.Flags().FlagUsages(), -1) {
				if !strings.Contains(readme, "`--"+match[1]) {
					t.Errorf("README.md does not document %s's flag --%s", cmd.Name(), match[1])
				}
			}
		})
	}
}

// readREADME returns the README's text.
func readREADME(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read the README: %v", err)
	}
	return string(raw)
}

// yamlTags returns every yaml tag of root's fields and of the structs they hold — through
// pointers, slices and map values — sorted and without duplicates.
func yamlTags(root reflect.Type) []string {
	var tags []string
	visited := make(map[reflect.Type]bool)
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || visited[typ] {
			return
		}
		visited[typ] = true
		for field := range typ.Fields() {
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			if name == "" || name == skippedTag {
				continue
			}
			tags = append(tags, name)
			walk(field.Type)
		}
	}
	walk(root)
	slices.Sort(tags)
	return slices.Compact(tags)
}
