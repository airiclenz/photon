package engine

import (
	"strings"
	"time"
)

// The humanized-typing profile. A take types each character, then waits the gap drawn for it;
// the gaps come from four fixed bands so the rhythm can be asserted, and from a seeded generator
// so two machines recording the same storyboard type on identical beats.
//
// The generator is MINSTD (Lehmer, multiplier 16807, modulus 2^31-1) spelled out rather than
// delegated to math/rand, because the sequence is the contract: math/rand may change its
// algorithm between Go releases, and a changed rhythm would move every beat of every take.
const (
	// typingSeed is the seed every humanized string is typed at.
	typingSeed = 4242

	minstdMultiplier = 16807
	minstdModulus    = 2147483647

	letterGapMinMS      = 25
	letterGapMaxMS      = 45
	spaceGapMinMS       = 60
	spaceGapMaxMS       = 90
	punctuationGapMinMS = 90
	punctuationGapMaxMS = 140
	thinkingPauseMinMS  = 300
	thinkingPauseMaxMS  = 500

	// punctuationCharacters take the longer gap after them. The set is deliberately ASCII only:
	// an em dash types in the letter band.
	punctuationCharacters = ".,-!"

	// A thinking pause replaces the gap after a space one time in eight, and never more than
	// twice in one string — a third reads as the program stalling rather than the user thinking.
	thinkingPauseOdds  = 8
	thinkingPauseLimit = 2
)

// minstd is the Lehmer generator the profile draws from. Every product stays under 2^46, so the
// int64 arithmetic is exact.
type minstd struct {
	state int64
}

// advance steps the generator and returns its new state.
func (g *minstd) advance() int64 {
	g.state = (minstdMultiplier * g.state) % minstdModulus
	return g.state
}

// draw returns a value in [low, high], inclusive.
func (g *minstd) draw(low, high int64) int64 {
	return low + g.advance()%(high-low+1)
}

// humanize returns the gap to wait after each character of text but the last: element i is the
// pause between rune i and rune i+1, so a string of n runes yields n-1 gaps and a one-rune or
// empty string yields none. It draws from a fresh generator per call, seeded with typingSeed and
// with its first value discarded, so the rhythm of one string never depends on another's. The
// band is chosen by the rune just typed; a space's gap may instead be a thinking pause, and the
// odds draw for that is consumed on every space whether or not it fires.
func humanize(text string) []time.Duration {
	runes := []rune(text)
	if len(runes) < 2 {
		return nil
	}

	generator := minstd{state: typingSeed}
	generator.advance() // a small seed would otherwise open on a small gap

	gaps := make([]time.Duration, 0, len(runes)-1)
	thinkingPauses := 0
	for _, character := range runes[:len(runes)-1] {
		var gapMS int64
		switch {
		case character == ' ':
			if generator.draw(1, thinkingPauseOdds) == 1 && thinkingPauses < thinkingPauseLimit {
				gapMS = generator.draw(thinkingPauseMinMS, thinkingPauseMaxMS)
				thinkingPauses++
			} else {
				gapMS = generator.draw(spaceGapMinMS, spaceGapMaxMS)
			}
		case strings.ContainsRune(punctuationCharacters, character):
			gapMS = generator.draw(punctuationGapMinMS, punctuationGapMaxMS)
		default:
			gapMS = generator.draw(letterGapMinMS, letterGapMaxMS)
		}
		gaps = append(gaps, time.Duration(gapMS)*time.Millisecond)
	}
	return gaps
}
