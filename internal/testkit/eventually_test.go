package testkit

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// pollTestInterval keeps the polling tests fast.
const pollTestInterval = time.Millisecond

func TestPoll_ConditionMetInTime_ReturnsNil(t *testing.T) {
	t.Parallel()
	calls := 0
	condition := func() error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	}

	err := poll(5*time.Second, pollTestInterval, condition)

	if err != nil {
		t.Fatalf("poll = %v, want nil once the condition is met", err)
	}
	if calls != 3 {
		t.Errorf("condition ran %d times, want 3: poll must stop at the first success", calls)
	}
}

func TestPoll_DeadlineMissed_ReportsTheLastState(t *testing.T) {
	t.Parallel()
	condition := func() error { return errors.New("screen still blank") }

	err := poll(20*time.Millisecond, pollTestInterval, condition)

	if !errors.Is(err, errDeadline) {
		t.Fatalf("poll = %v, want an error wrapping errDeadline", err)
	}
	if !strings.Contains(err.Error(), "screen still blank") {
		t.Errorf("poll = %q, want it to name the condition's last error", err)
	}
}

func TestEventually_ConditionMet_Passes(t *testing.T) {
	t.Parallel()
	start := time.Now()

	Eventually(t, 5*time.Second, func() error {
		if time.Since(start) < 30*time.Millisecond {
			return errors.New("too early")
		}
		return nil
	})
}
