//go:build windows

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// errNoInterrupt is what terminate returns: Windows cannot ask a process tree to stop, so the
// caller kills it.
var errNoInterrupt = errors.New("windows cannot interrupt a service")

// errNoThread is what resume returns when the suspended process has no thread to resume.
var errNoThread = errors.New("the process has no thread")

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

// prepareGroup makes cmd start suspended: the job is made once the process exists, and a shell
// left running could start a child before then, which would stay outside the job.
func prepareGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
}

// joinGroup puts the suspended process in a new job, then resumes it, so every process it
// starts is in the job too.
func joinGroup(process *os.Process) (*group, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create a job: %w", err)
	}
	const access = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE
	handle, err := windows.OpenProcess(access, false, uint32(process.Pid))
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
	if err := resume(uint32(process.Pid)); err != nil {
		return nil, errors.Join(fmt.Errorf("resume the process: %w", err), windows.CloseHandle(job))
	}
	return &group{job: job}, nil
}

// resume resumes every thread of the process pid; a process started suspended has one, and
// os.StartProcess does not keep its handle.
func resume(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("list the threads: %w", err)
	}
	var errs []error
	resumed := 0
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	next := windows.Thread32First
	for err = next(snapshot, &entry); err == nil; err = next(snapshot, &entry) {
		next = windows.Thread32Next
		if entry.OwnerProcessID != pid {
			continue
		}
		if resumeErr := resumeThread(entry.ThreadID); resumeErr != nil {
			errs = append(errs, resumeErr)
			continue
		}
		resumed++
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		errs = append(errs, fmt.Errorf("list the threads: %w", err))
	}
	if resumed == 0 && len(errs) == 0 {
		errs = append(errs, errNoThread)
	}
	if closeErr := windows.CloseHandle(snapshot); closeErr != nil {
		errs = append(errs, fmt.Errorf("close the thread list: %w", closeErr))
	}
	return errors.Join(errs...)
}

// resumeThread resumes the thread id.
func resumeThread(id uint32) error {
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, id)
	if err != nil {
		return fmt.Errorf("open thread %d: %w", id, err)
	}
	_, resumeErr := windows.ResumeThread(thread)
	if resumeErr != nil {
		resumeErr = fmt.Errorf("resume thread %d: %w", id, resumeErr)
	}
	return errors.Join(resumeErr, windows.CloseHandle(thread))
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
