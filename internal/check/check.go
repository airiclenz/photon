package check

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// Verdict is what one row of the check table reports.
type Verdict string

// The two verdicts.
const (
	Pass Verdict = "PASS"
	Fail Verdict = "FAIL"
)

// takeSubject is the subject of every take-level row.
const takeSubject = "take"

// screenTimeRound is the precision a matching screen's time is printed at.
const screenTimeRound = time.Millisecond

// Row is one line of the check table: its Subject (`beat N`, or `take` for a take-level
// expect), its Verdict and the Detail that says what was judged.
type Row struct {
	Subject string
	Verdict Verdict
	Detail  string
}

// Options is what a caller adds to a judgment. Env is extra `KEY=VALUE` entries every `run`
// expect gets, such as the ports of the take's services; photon's own PHOTON_WORK, PHOTON_TAKE
// and PHOTON_BEAT win over an entry of the same name.
type Options struct {
	Env []string
}

// Judge judges every expect of board against recorded, the take at takePath: first each beat's
// expects in storyboard order, then the take-level ones, one [Row] each.
//
// A `seen` expect passes when its regex matches the text resolver reads from its area (the whole
// screen without one) on at least one screen shown during the beat — every snapshot whose display
// interval overlaps the beat's span — or, take-level, on any screen of the take. A screen on which
// the area cannot be found does not match. A `run` expect runs `sh -c <command>` in work, which
// is created first, with PHOTON_WORK and PHOTON_TAKE (both absolute) and, for a beat's expect,
// PHOTON_BEAT in its env; exit 0 passes, and its trimmed output is the detail.
//
// It is an error, before any expect is judged, when the take's beats and the storyboard's do not
// agree — a storyboard beat the take never started, or a take beat the storyboard lacks, each
// named — and when a `seen` expect names an area resolver does not know. A failing expect is a
// FAIL row, never an error.
func Judge(
	ctx context.Context,
	board *storyboard.Storyboard,
	recorded *take.Take,
	resolver target.Resolver,
	work, takePath string,
	options Options,
) ([]Row, error) {
	screens, err := beatScreens(board, recorded)
	if err != nil {
		return nil, err
	}
	runner, err := newRunner(work, takePath, options.Env)
	if err != nil {
		return nil, err
	}
	if hasRun(board) {
		if err := runner.makeWork(); err != nil {
			return nil, err
		}
	}
	judge := judgement{ctx: ctx, resolver: resolver, runner: runner}

	var rows []Row
	for _, beat := range board.Beats {
		beatRows, err := judge.expects(beatSubject(beat.ID), beat.Expect, screens[beat.ID], &beat.ID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, beatRows...)
	}
	takeRows, err := judge.expects(takeSubject, board.Expect, recorded.Snapshots, nil)
	if err != nil {
		return nil, err
	}
	return append(rows, takeRows...), nil
}

// WriteTable prints rows as `subject | verdict | detail`, one per line, the subjects padded to
// the widest so the verdicts line up.
func WriteTable(w io.Writer, rows []Row) error {
	width := 0
	for _, row := range rows {
		width = max(width, len(row.Subject))
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%-*s | %s | %s\n", width, row.Subject, row.Verdict, row.Detail); err != nil {
			return fmt.Errorf("write the check table: %w", err)
		}
	}
	return nil
}

// Failed counts the FAIL rows.
func Failed(rows []Row) int {
	failed := 0
	for _, row := range rows {
		if row.Verdict == Fail {
			failed++
		}
	}
	return failed
}

// hasRun reports whether any expect of board, a beat's or the take's, is a `run` expect.
func hasRun(board *storyboard.Storyboard) bool {
	expects := slices.Clone(board.Expect)
	for _, beat := range board.Beats {
		expects = append(expects, beat.Expect...)
	}
	return slices.ContainsFunc(expects, func(expect storyboard.Expect) bool { return expect.Run != "" })
}

// beatSubject is the subject of a beat's rows.
func beatSubject(id int) string {
	return fmt.Sprintf("beat %d", id)
}

// judgement is what judging one take's expects shares: the context `run` expects run under, the
// resolver `seen` expects read areas with, and the runner that runs them.
type judgement struct {
	ctx      context.Context
	resolver target.Resolver
	runner   *runner
}

// expects judges one beat's expects (beat is its id), or the take-level ones (beat nil), against
// screens, one row each.
func (j judgement) expects(subject string, expects []storyboard.Expect, screens []take.Snapshot, beat *int) ([]Row, error) {
	rows := make([]Row, 0, len(expects))
	for _, expect := range expects {
		row := Row{Subject: subject}
		if expect.Run != "" {
			row.Verdict, row.Detail = j.runner.run(j.ctx, expect.Run, beat)
			rows = append(rows, row)
			continue
		}
		verdict, detail, err := j.seen(expect, screens, subject)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", subject, err)
		}
		row.Verdict, row.Detail = verdict, detail
		rows = append(rows, row)
	}
	return rows, nil
}

// seen judges a `seen` expect on screens: PASS naming the first screen whose area text the regex
// matches, on the take's clock, or FAIL counting the screens searched. Only an area the resolver
// does not know is an error; an area missing from one screen leaves that screen unmatched.
func (j judgement) seen(expect storyboard.Expect, screens []take.Snapshot, subject string) (Verdict, string, error) {
	where := ""
	if expect.Area != "" {
		where = " in area " + expect.Area
	}
	pattern, err := regexp.Compile(expect.Seen)
	if err != nil {
		return Fail, fmt.Sprintf("seen /%s/: %v", expect.Seen, err), nil
	}
	for _, screen := range screens {
		text, err := j.resolver.Text(screen, expect.Area)
		if errors.Is(err, target.ErrNotOnScreen) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		if pattern.MatchString(text) {
			return Pass, fmt.Sprintf("seen /%s/%s at %s", expect.Seen, where, screen.At.Round(screenTimeRound)), nil
		}
	}
	owner := "the take"
	if subject != takeSubject {
		owner = "the beat"
	}
	return Fail, fmt.Sprintf("not seen /%s/%s on any of %s's %d screen(s)", expect.Seen, where, owner, len(screens)), nil
}

// beatScreens groups the take's snapshots by the beat they were on screen during, after checking
// that the take started exactly the storyboard's beats. A snapshot belongs to every beat whose
// span its display interval — from its own time to the next snapshot's — overlaps, so the screen
// showing when a beat starts counts for that beat. Spans are half-open except the last, which
// keeps the take's final instant.
func beatScreens(board *storyboard.Storyboard, recorded *take.Take) (map[int][]take.Snapshot, error) {
	spans, err := recorded.BeatSpans()
	if err != nil {
		return nil, err
	}
	if err := matchBeats(board, spans); err != nil {
		return nil, err
	}

	snapshots := recorded.Snapshots
	screens := make(map[int][]take.Snapshot, len(spans))
	for index, span := range spans {
		isLast := index == len(spans)-1
		for at, snapshot := range snapshots {
			shownUntil := time.Duration(math.MaxInt64)
			if at+1 < len(snapshots) {
				shownUntil = snapshots[at+1].At
			}
			startsInside := snapshot.At < span.End || (isLast && snapshot.At <= span.End)
			if startsInside && shownUntil > span.Start {
				screens[span.Beat] = append(screens[span.Beat], snapshot)
			}
		}
	}
	return screens, nil
}

// matchBeats reports every storyboard beat with no span in the take and every span whose beat
// the storyboard lacks, each by id.
func matchBeats(board *storyboard.Storyboard, spans []take.BeatSpan) error {
	started := make(map[int]bool, len(spans))
	for _, span := range spans {
		started[span.Beat] = true
	}
	declared := make(map[int]bool, len(board.Beats))
	var problems []error
	for _, beat := range board.Beats {
		declared[beat.ID] = true
		if !started[beat.ID] {
			problems = append(problems, fmt.Errorf("beat %d: the take never started it", beat.ID))
		}
	}
	for _, span := range spans {
		if !declared[span.Beat] {
			problems = append(problems, fmt.Errorf("beat %d: the take started it but the storyboard has no such beat", span.Beat))
		}
	}
	return errors.Join(problems...)
}
