//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	startupTaskName        = "Bitfocus Listener"
	seeMaskNoCloseProcess  = 0x00000040
	shellExecuteWaitMillis = 15000
	waitObject0            = 0x00000000
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     syscall.Handle
}

var (
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procShellExecuteEx      = shell32.NewProc("ShellExecuteExW")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procGetExitCodeProcess  = kernel32.NewProc("GetExitCodeProcess")
)

func hiddenLaunchRequested() bool {
	return containsHiddenLaunchArg(os.Args[1:])
}

func startupSupported() bool {
	return true
}

func schtasksPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		return "schtasks.exe"
	}
	return filepath.Join(root, "System32", "schtasks.exe")
}

func startupTaskExists() bool {
	cmd := exec.Command(schtasksPath(), "/Query", "/TN", startupTaskName)
	hideCmdWindow(cmd)
	return cmd.Run() == nil
}

func configureStartup(enabled, hidden, elevated bool) error {
	var args []string

	if enabled {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate Listener executable: %w", err)
		}

		taskRun := `"` + exePath + `"`
		if hidden {
			taskRun += " --hidden"
		}

		runLevel := "LIMITED"
		if elevated {
			runLevel = "HIGHEST"
		}

		args = []string{
			"/Create",
			"/TN", startupTaskName,
			"/SC", "ONLOGON",
			"/IT",
			"/RL", runLevel,
			"/TR", taskRun,
			"/F",
		}
	} else {
		if !startupTaskExists() {
			return nil
		}

		args = []string{
			"/Delete",
			"/TN", startupTaskName,
			"/F",
		}
	}

	// schtasks receives only application-controlled values here: the current
	// executable path, a fixed task name, and fixed scheduler options. No remote
	// Listener command input reaches this process.
	if enabled && elevated {
		if err := runSchtasksElevated(args); err != nil {
			return err
		}
	} else if err := runSchtasksDirect(args); err != nil {
		// Replacing or deleting an existing elevated task also requires elevation.
		if elevatedErr := runSchtasksElevated(args); elevatedErr != nil {
			return fmt.Errorf("startup task failed: %v; elevated retry failed: %w", err, elevatedErr)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if startupTaskExists() == enabled {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	if enabled {
		return fmt.Errorf("Windows startup task was not created")
	}
	return fmt.Errorf("Windows startup task was not removed")
}

func runSchtasksDirect(args []string) error {
	cmd := exec.Command(schtasksPath(), args...)
	hideCmdWindow(cmd)

	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("schtasks: %s", message)
}

func runSchtasksElevated(args []string) error {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}

	executable, err := syscall.UTF16PtrFromString(schtasksPath())
	if err != nil {
		return err
	}

	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = syscall.EscapeArg(arg)
	}

	parameters, err := syscall.UTF16PtrFromString(strings.Join(escaped, " "))
	if err != nil {
		return err
	}

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verb,
		lpFile:       executable,
		lpParameters: parameters,
		nShow:        0, // SW_HIDE: keep schtasks.exe from flashing a console window.
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	result, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("Windows elevation failed or was cancelled: %w", callErr)
		}
		return fmt.Errorf("Windows elevation failed or was cancelled")
	}
	if info.hProcess == 0 {
		return fmt.Errorf("Windows elevation started without a process handle")
	}
	defer syscall.CloseHandle(info.hProcess)

	waitResult, _, waitErr := procWaitForSingleObject.Call(
		uintptr(info.hProcess),
		shellExecuteWaitMillis,
	)
	if waitResult != waitObject0 {
		if waitErr != syscall.Errno(0) {
			return fmt.Errorf("wait for elevated startup task: %w", waitErr)
		}
		return fmt.Errorf("timed out waiting for elevated startup task")
	}

	var exitCode uint32
	ok, _, exitErr := procGetExitCodeProcess.Call(
		uintptr(info.hProcess),
		uintptr(unsafe.Pointer(&exitCode)),
	)
	if ok == 0 {
		if exitErr != syscall.Errno(0) {
			return fmt.Errorf("read elevated startup task exit code: %w", exitErr)
		}
		return fmt.Errorf("read elevated startup task exit code")
	}
	if exitCode != 0 {
		return fmt.Errorf("schtasks exited with code %d", exitCode)
	}

	return nil
}
