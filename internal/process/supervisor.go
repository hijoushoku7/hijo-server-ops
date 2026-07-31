package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const supervisorArgument = "__hso_supervise"

func SupervisorCommand(args []string) (string, bool) {
	if len(args) != 3 || args[1] != supervisorArgument {
		return "", false
	}
	return args[2], true
}

func RunSupervisor(command string) int {
	control := os.NewFile(3, "hso-supervisor-control")
	if control == nil {
		fmt.Fprintln(os.Stderr, "hso supervisor: no control pipe")
		return 1
	}
	defer control.Close()
	syscall.CloseOnExec(int(control.Fd()))

	if err := becomeSubreaper(); err != nil {
		writeSupervisorError(control, err)
		return 1
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	server := exec.Command(command)
	server.Stdin = os.Stdin
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	server.Env = os.Environ()
	server.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := server.Start(); err != nil {
		writeSupervisorError(control, fmt.Errorf("start the start script: %w", err))
		return 1
	}
	serverPID := server.Process.Pid
	fmt.Fprintf(control, "PID %d\n", serverPID)
	_ = control.Close()

	waited := make(chan error, 1)
	go func() {
		waited <- server.Wait()
	}()

	var (
		serverErr  error
		serverDone bool
		exitSignal syscall.Signal
		killTimer  <-chan time.Time
	)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if serverDone {
			reapAdoptedChildren()
			alive, err := managedProcessesAlive("/proc", os.Getpid(), serverPID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "hso supervisor: check process: %v\n", err)
				return 1
			}
			if !alive {
				if exitSignal != 0 {
					return 128 + int(exitSignal)
				}
				return commandExitCode(serverErr)
			}
		}

		select {
		case serverErr = <-waited:
			serverDone = true
		case received := <-signals:
			value, ok := received.(syscall.Signal)
			if !ok {
				continue
			}
			if exitSignal == 0 {
				exitSignal = value
				terminateManagedProcesses("/proc", os.Getpid(), serverPID, syscall.SIGTERM)
				killTimer = time.After(time.Second)
			}
		case <-killTimer:
			terminateManagedProcesses("/proc", os.Getpid(), serverPID, syscall.SIGKILL)
			killTimer = nil
		case <-ticker.C:
		}
	}
}

func becomeSubreaper() error {
	const prSetChildSubreaper = 36
	_, _, errno := syscall.Syscall6(
		syscall.SYS_PRCTL,
		prSetChildSubreaper,
		1,
		0,
		0,
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("enable subreaper: %w", errno)
	}
	return nil
}

func managedProcessesAlive(procRoot string, supervisorPID, serverGroup int) (bool, error) {
	processes, err := readProcesses(procRoot)
	if err != nil {
		return false, err
	}
	managed := managedProcessIDs(processes, supervisorPID, serverGroup)
	delete(managed, supervisorPID)
	return len(managed) > 0, nil
}

func terminateManagedProcesses(
	procRoot string,
	supervisorPID int,
	serverGroup int,
	signal syscall.Signal,
) {
	processes, err := readProcesses(procRoot)
	if err != nil {
		return
	}
	managed := managedProcessIDs(processes, supervisorPID, serverGroup)

	groups := map[int]bool{serverGroup: true}
	for pid := range managed {
		if pid == supervisorPID {
			continue
		}
		entry := processes[pid]
		if entry.pgrp > 0 && entry.pgrp != processes[supervisorPID].pgrp {
			groups[entry.pgrp] = true
			continue
		}
		_ = syscall.Kill(pid, signal)
	}
	for group := range groups {
		_ = syscall.Kill(-group, signal)
	}
}

func managedProcessIDs(
	processes map[int]procEntry,
	supervisorPID int,
	serverGroup int,
) map[int]bool {
	managed := map[int]bool{supervisorPID: true}
	for changed := true; changed; {
		changed = false
		for pid, entry := range processes {
			if managed[pid] || (!managed[entry.ppid] && entry.pgrp != serverGroup) {
				continue
			}
			managed[pid] = true
			changed = true
		}
	}
	return managed
}

func reapAdoptedChildren() {
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if pid <= 0 || (err != nil && !errors.Is(err, syscall.ECHILD)) {
			return
		}
	}
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exitErr.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "hso supervisor: wait for the start script: %v\n", err)
	return 1
}

func writeSupervisorError(control *os.File, err error) {
	message := strings.ReplaceAll(err.Error(), "\n", " ")
	fmt.Fprintf(control, "ERR %s\n", message)
	fmt.Fprintf(os.Stderr, "hso supervisor: %s\n", message)
}
