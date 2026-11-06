package browser_test

import (
	"github.com/airiclenz/photon/internal/browser"
	"github.com/airiclenz/photon/internal/engine"
)

// A browser is a source the engine can drive: it types, presses keys and dispatches pointer
// events as the pty terminal does.
var _ engine.Source = (*browser.Browser)(nil)
