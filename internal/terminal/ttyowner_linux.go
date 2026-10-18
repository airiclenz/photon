package terminal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// procDir is where Linux lists its processes, one directory per pid.
const procDir = "/proc"

// maxListedOwners caps how many holders [ttyOwners] names: a launch error is one line, and past a
// handful the count says as much as the names.
const maxListedOwners = 8

// procStatFieldsAfterName is how many fields of a /proc/<pid>/stat line after the command name
// [parseProcStat] reads: state, ppid, pgrp, session, tty_nr.
const procStatFieldsAfterName = 5

// ttyOwners lists the processes whose controlling terminal tty is, as "pid N (comm) session S",
// read from the tty_nr field of every /proc/<pid>/stat. A process that exits mid-scan is skipped.
func ttyOwners(tty *os.File) ([]string, error) {
	info, err := tty.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", tty.Name(), err)
	}
	stat, isUnixStat := info.Sys().(*syscall.Stat_t)
	if !isUnixStat {
		return nil, errors.New("no device number for " + tty.Name())
	}
	// Rdev is a uint32 on some Linux architectures.
	want := encodeTTYNr(uint64(stat.Rdev))
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, fmt.Errorf("list the processes: %w", err)
	}
	var owners []string
	held := 0
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(procDir, entry.Name(), "stat"))
		if err != nil {
			continue
		}
		comm, session, ttyNr, isParsed := parseProcStat(string(raw))
		if !isParsed || ttyNr != want {
			continue
		}
		held++
		if len(owners) < maxListedOwners {
			owner := fmt.Sprintf("pid %s (%s) session %d", entry.Name(), comm, session)
			owners = append(owners, owner)
		}
	}
	if held > len(owners) {
		owners = append(owners, fmt.Sprintf("%d more", held-len(owners)))
	}
	return owners, nil
}

// parseProcStat reads the command name, session id and tty_nr out of one /proc/<pid>/stat line.
// The name is the parenthesised second field and may itself hold spaces and parentheses, so the
// fields after it are counted from its last closing parenthesis.
func parseProcStat(line string) (comm string, session, ttyNr uint64, isParsed bool) {
	open := strings.IndexByte(line, '(')
	closing := strings.LastIndexByte(line, ')')
	if open < 0 || closing < open {
		return "", 0, 0, false
	}
	// After the name: state ppid pgrp session tty_nr ...
	fields := strings.Fields(line[closing+1:])
	if len(fields) < procStatFieldsAfterName {
		return "", 0, 0, false
	}
	session, sessionErr := strconv.ParseUint(fields[3], 10, 64)
	// tty_nr is a signed int in the kernel's format; a process with no terminal reads 0, and a
	// real terminal's number is never negative.
	ttyField, ttyErr := strconv.ParseInt(fields[4], 10, 64)
	if sessionErr != nil || ttyErr != nil || ttyField < 0 {
		return "", 0, 0, false
	}
	return line[open+1 : closing], session, uint64(ttyField), true
}

// encodeTTYNr turns a device number as stat reports it into the kernel's tty_nr encoding: the
// minor's low byte in bits 0-7, the major in bits 8-19, the minor's remaining bits from bit 20 up.
func encodeTTYNr(rdev uint64) uint64 {
	major, minor := uint64(unix.Major(rdev)), uint64(unix.Minor(rdev))
	return minor&0xff | major<<8 | (minor&^0xff)<<12
}
