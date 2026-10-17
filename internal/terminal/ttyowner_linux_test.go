package terminal

import (
	"fmt"
	"os/exec"
	"syscall"
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestParseProcStat(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		line        string
		wantComm    string
		wantSession uint64
		wantTTY     uint64
		wantParsed  bool
	}{
		{name: "a process on a pty", line: "4242 (bash) S 4200 4242 4242 34819 4242 4194560 0 0",
			wantComm: "bash", wantSession: 4242, wantTTY: 34819, wantParsed: true},
		{name: "a name with spaces and parentheses", line: "77 (my (odd) prog) R 1 77 70 34820 77 0",
			wantComm: "my (odd) prog", wantSession: 70, wantTTY: 34820, wantParsed: true},
		{name: "no controlling terminal", line: "1 (init) S 0 1 1 0 -1 4194560",
			wantComm: "init", wantSession: 1, wantTTY: 0, wantParsed: true},
		{name: "no name", line: "1 init S 0 1 1 0 -1"},
		{name: "a name never closed", line: "1 (init S 0 1 1 0 -1"},
		{name: "too few fields", line: "1 (init) S 0 1"},
		{name: "a session that is not a number", line: "1 (init) S 0 1 x 0 -1"},
		{name: "a negative tty", line: "1 (init) S 0 1 1 -5 -1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			comm, session, ttyNr, isParsed := parseProcStat(test.line)
			if isParsed != test.wantParsed {
				t.Fatalf("parsed = %v, want %v", isParsed, test.wantParsed)
			}
			if comm != test.wantComm || session != test.wantSession || ttyNr != test.wantTTY {
				t.Errorf("got (%q, %d, %d), want (%q, %d, %d)",
					comm, session, ttyNr, test.wantComm, test.wantSession, test.wantTTY)
			}
		})
	}
}

func TestEncodeTTYNr(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		major, minor uint32
		want         uint64
	}{
		{name: "/dev/pts/3", major: 136, minor: 3, want: 136<<8 | 3},
		{name: "a minor past one byte", major: 136, minor: 300, want: 0x2c | 136<<8 | 0x100<<12},
		{name: "/dev/tty1", major: 4, minor: 1, want: 4<<8 | 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := encodeTTYNr(unix.Mkdev(test.major, test.minor)); got != test.want {
				t.Errorf("encodeTTYNr(%d:%d) = %d, want %d", test.major, test.minor, got, test.want)
			}
		})
	}
}

func TestTTYOwners_NamesTheSessionLeader(t *testing.T) {
	t.Parallel()
	master, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open a pty: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = master.Close()
	})
	cmd := exec.Command("sleep", "30")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start a session leader on the pty: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	owners, err := ttyOwners(tty)
	if err != nil {
		t.Fatalf("ttyOwners: %v", err)
	}
	want := fmt.Sprintf("pid %d (sleep) session %d", cmd.Process.Pid, cmd.Process.Pid)
	if len(owners) != 1 || owners[0] != want {
		t.Errorf("owners = %q, want [%s]", owners, want)
	}
}
