//go:build !linux && !windows

package terminal

import (
	"errors"
	"os"
)

// errOwnerUnknown is why [ttyOwners] lists nobody off Linux.
var errOwnerUnknown = errors.New("owner unknown off linux")

// ttyOwners has no portable source outside Linux's /proc, so the probe reports the owner unknown.
func ttyOwners(*os.File) ([]string, error) {
	return nil, errOwnerUnknown
}
