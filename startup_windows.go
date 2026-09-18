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

const startupTaskName = "Bitfocus Listener"

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

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

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if startupTaskExists() == enabled {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
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

	result, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(executable)),
		uintptr(unsafe.Pointer(parameters)),
		0,
		0, // SW_HIDE: keep schtasks.exe from flashing a console window.
	)
	if result <= 32 {
		return fmt.Errorf("Windows elevation failed or was cancelled (code %d)", result)
	}

	return nil
}
