//go:build windows

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// errNoInterrupt is what terminate returns: Windows cannot ask a process tree to stop, so the
// caller kills it.
var errNoInterrupt = errors.New("windows cannot interrupt a service")

// jobAccounting is the Win32 JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, which x/sys/windows lacks.
type jobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// group is a Job object holding the service's shell and every process it starts, so the whole
// tree is killed at once. Windows has no process groups to signal; a job is its equivalent.
type group struct {
	job windows.Handle
}

// prepareGroup does nothing on Windows: the job is made once the process exists.
func prepareGroup(*exec.Cmd) {}

// joinGroup puts process in a new job. A child the shell starts before this returns stays
// outside the job; the shell starts none that early in practice.
func joinGroup(process *os.Process) (*group, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create a job: %w", err)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open the process: %w", err), windows.CloseHandle(job))
	}
	assignErr := windows.AssignProcessToJobObject(job, handle)
	closeErr := windows.CloseHandle(handle)
	if assignErr != nil || closeErr != nil {
		if assignErr != nil {
			assignErr = fmt.Errorf("put the process in a job: %w", assignErr)
		}
		if closeErr != nil {
			closeErr = fmt.Errorf("close the process: %w", closeErr)
		}
		return nil, errors.Join(assignErr, closeErr, windows.CloseHandle(job))
	}
	return &group{job: job}, nil
}

// terminate reports errNoInterrupt, so the caller kills the job straight away.
func (g *group) terminate() error {
	return errNoInterrupt
}

// kill ends every process in the job. A released job has none left to end.
func (g *group) kill() error {
	if g.job == 0 {
		return nil
	}
	return windows.TerminateJobObject(g.job, 1)
}

// alive reports whether any process of the job is still running; one the job cannot be asked
// about counts as running, and a released job has none.
func (g *group) alive() bool {
	if g.job == 0 {
		return false
	}
	var info jobAccounting
	err := windows.QueryInformationJobObject(g.job, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
	return err != nil || info.ActiveProcesses > 0
}

// release closes the job's handle, once: a set stopped again finds its jobs already released.
func (g *group) release() error {
	if g.job == 0 {
		return nil
	}
	job := g.job
	g.job = 0
	return windows.CloseHandle(job)
}
