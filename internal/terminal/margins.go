//go:build !windows

package terminal

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// The CSI final bytes whose margins clampMargins bounds.
const (
	// setTopBottomMargins is DECSTBM: CSI top ; bottom r.
	setTopBottomMargins = 'r'
	// setLeftRightMargins is DECSLRM: CSI left ; right s, honoured once DECLRMM (mode ?69) is set.
	setLeftRightMargins = 's'
)

// marginEnd is the index of the far margin (bottom or right) among a margin sequence's parameters.
const marginEnd = 1

// clampMargins keeps the emulator's scroll margins inside the buffer they index. x/vt applies a
// DECSTBM bottom margin, or a DECSLRM right margin, without bounding it by the screen; the next
// line operation inside such a margin (a delete-line, an insert-line) then indexes past the end of
// the buffer and panics on the goroutine feeding the emulator. A program that lays a frame out for
// a bigger terminal than the one it got is enough to send one.
//
// Each guard is a CSI handler registered after x/vt's own, so it runs first (handlers are called
// last-registered first), and returns false so the real handler still does the work, on
// parameters clamped in place: the parser hands a handler a slice over its own parameter storage,
// which is where x/vt's handler reads them back from. A program asking for more rows than the
// terminal has gets the margin a real terminal gives it: the last row.
func clampMargins(emulator *vt.Emulator) {
	emulator.RegisterCsiHandler(setTopBottomMargins, func(params ansi.Params) bool {
		clampParam(params, marginEnd, emulator.Height())
		return false
	})
	emulator.RegisterCsiHandler(setLeftRightMargins, func(params ansi.Params) bool {
		clampParam(params, marginEnd, emulator.Width())
		return false
	})
}

// clampParam lowers the parameter at index to limit when it asks for more. A missing or in-range
// parameter is left as it is: the default the handler behind this one uses for a missing
// parameter is the limit itself.
func clampParam(params ansi.Params, index, limit int) {
	if index < 0 || index >= len(params) {
		return
	}
	if params[index].Param(limit) > limit {
		params[index] = ansi.Param(ansi.Parameter(limit, params[index].HasMore()))
	}
}
