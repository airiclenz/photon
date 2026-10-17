package testkit

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// pollInterval is how long Eventually waits between two checks of its condition.
const pollInterval = 10 * time.Millisecond

// errDeadline marks a condition that was still unmet when its deadline passed.
var errDeadline = errors.New("deadline passed")

// Eventually checks condition at once and then every pollInterval until it returns nil, and fails
// t when timeout passes first. condition reports what is still missing as its error, so the
// failure names the last state seen rather than only that time ran out.
func Eventually(t testing.TB, timeout time.Duration, condition func() error) {
	t.Helper()

	if err := poll(timeout, pollInterval, condition); err != nil {
		t.Fatal(err)
	}
}

// poll is Eventually's core: it returns nil once condition does, or an error wrapping errDeadline
// and condition's last error when timeout passes first. condition always runs at least once.
func poll(timeout, interval time.Duration, condition func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := condition()
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w after %v: %w", errDeadline, timeout, err)
		}
		time.Sleep(interval)
	}
}
