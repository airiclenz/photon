package config

import "testing"

func TestLoadRejectsAreas(t *testing.T) {
	t.Parallel()
	const footer = "  footer:\n    top: {match: '^─+$', pick: last, offset: 1}\n"
	cases := []rejectCase{
		{"selector on a command launch", footer, "  footer:\n    selector: '#footer'\n",
			"areas.footer: selector: only a url launch takes a selector area", false},
	}
	assertRejects(t, validBase, cases)
}

func TestLoadRejectsAreasOnAURLLaunch(t *testing.T) {
	t.Parallel()
	const selector = `'nav[aria-label="Main"] > ul'`
	cases := []rejectCase{
		{"selector unbalanced", selector, `'nav[aria-label="Main" > ul'`, "areas.sidebar: selector: unclosed '['", false},
		{"selector blank", selector, "' '", `areas.sidebar: selector: want a CSS selector, got " "`, false},
		{"selector empty", selector, "''", `areas.sidebar: selector: want a CSS selector, got ""`, false},
		{"selector with an edge", selector, selector + "\n    top: {match: Inbox}",
			"areas.sidebar: selector: want a selector or top and bottom edges, not both", false},
		{"anchor offset", "bottom: {match: '^Sent$'}", "bottom: {match: '^Sent$', offset: -1}",
			"areas.thread: bottom.offset: a url launch has no rows to move by, so takes no offset", false},
		{"nested anchor offset", "below: {match: 'Search'}", "below: {match: 'Search', offset: 1}",
			"areas.thread: top.below.offset: a url launch has no rows to move by, so takes no offset", false},
	}
	assertRejects(t, urlBase, cases)
}

func TestLoadSelectorArea(t *testing.T) {
	t.Parallel()

	config, err := Load(writeConfig(t, urlBase))

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sidebar := config.Areas["sidebar"]
	if sidebar.Selector == nil || *sidebar.Selector != `nav[aria-label="Main"] > ul` ||
		sidebar.Top != nil || sidebar.Bottom != nil {
		t.Errorf("sidebar: want the selector kept as written and no edges, got %+v", sidebar)
	}
	if thread := config.Areas["thread"]; thread.Selector != nil || thread.Top == nil || thread.Bottom == nil {
		t.Errorf("thread: want no selector and both edges, got %+v", thread)
	}
}
