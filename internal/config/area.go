package config

import (
	"fmt"
	"regexp"
)

// Area is a named band of screen rows a target or a `seen` expect is searched in. Its Top and
// Bottom edges are row anchors; an omitted edge is the screen's own edge, so an area with
// neither is the whole screen. Both edges are inclusive. The rows are found on each snapshot by
// the target resolver, not here.
type Area struct {
	Top    *Anchor `yaml:"top"`
	Bottom *Anchor `yaml:"bottom"`
}

// Anchor picks one screen row: the rows whose text Match (a regex) finds, the first or last of
// them by Pick, moved by Offset rows (negative is up). Above or Below, at most one of them,
// limits the search to the rows strictly above or below the row another anchor picks, so an edge
// can be "the last rule above the input box".
type Anchor struct {
	Match  string  `yaml:"match"`
	Pick   Pick    `yaml:"pick"`
	Above  *Anchor `yaml:"above"`
	Below  *Anchor `yaml:"below"`
	Offset int     `yaml:"offset"`
}

// Pick says which of an anchor's matching rows it takes. [Load] sets an omitted one to
// [PickFirst].
type Pick string

// The two picks: the topmost matching row or the bottommost.
const (
	PickFirst Pick = "first"
	PickLast  Pick = "last"
)

// AreaAny is the area name meaning the whole screen. It needs no declaration, so a config may
// not declare it.
const AreaAny = "any"

// areaName is the spelling of an area name, the same spelling a storyboard's area must have.
var areaName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// validate checks the area's edges.
func (a Area) validate(report reporter) {
	if a.Top != nil {
		a.Top.validate(prefixed(report, "top."))
	}
	if a.Bottom != nil {
		a.Bottom.validate(prefixed(report, "bottom."))
	}
}

// validate checks the anchor's regex and pick, that it is limited on at most one side, and the
// anchor it is limited by.
func (a Anchor) validate(report reporter) {
	validateRegex("match", a.Match, report)
	if a.Pick != "" && a.Pick != PickFirst && a.Pick != PickLast {
		report("pick: want %s or %s, got %q", PickFirst, PickLast, a.Pick)
	}
	if a.Above != nil && a.Below != nil {
		report("want at most one of above or below")
	}
	if a.Above != nil {
		a.Above.validate(prefixed(report, "above."))
	}
	if a.Below != nil {
		a.Below.validate(prefixed(report, "below."))
	}
}

// applyDefaults sets an omitted pick, here and in the anchor this one is limited by, to
// [PickFirst].
func (a *Anchor) applyDefaults() {
	if a == nil {
		return
	}
	if a.Pick == "" {
		a.Pick = PickFirst
	}
	a.Above.applyDefaults()
	a.Below.applyDefaults()
}

// validateAreas checks every declared area's name and edges, in name order.
func validateAreas(areas map[string]Area, report reporter) {
	for _, name := range sortedKeys(areas) {
		areaReport := prefixed(report, fmt.Sprintf("areas.%s: ", name))
		switch {
		case name == AreaAny:
			areaReport("%s is the whole screen and needs no declaration", AreaAny)
		case !areaName.MatchString(name):
			areaReport("want a name of letters, digits, - and _ starting with a letter")
		}
		areas[name].validate(areaReport)
	}
}
