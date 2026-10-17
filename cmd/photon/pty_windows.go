//go:build windows

package main

import "github.com/airiclenz/photon/internal/terminal"

// refuseWithoutPTY is [terminal.ErrNoPTY]: record and setup refuse before touching the rig, so a
// Windows run neither creates a work dir nor runs a project command it could not use.
func refuseWithoutPTY() error { return terminal.ErrNoPTY }
