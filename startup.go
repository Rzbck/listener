package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func containsHiddenLaunchArg(args []string) bool {
	for _, arg := range args {
		if arg == "--hidden" {
			return true
		}
	}
	return false
}

func startupTab(w fyne.Window) *container.TabItem {
	if !startupSupported() {
		return nil
	}

	configMu.RLock()
	launchAtLogin := config.LaunchAtLogin
	startHidden := config.StartHidden
	runElevated := config.RunElevated
	configMu.RUnlock()

	launchCheck := widget.NewCheck("Launch at login", nil)
	launchCheck.SetChecked(launchAtLogin)

	hiddenCheck := widget.NewCheck("Start hidden in system tray", nil)
	hiddenCheck.SetChecked(startHidden)

	elevatedCheck := widget.NewCheck("Run with highest privileges", nil)
	elevatedCheck.SetChecked(runElevated)

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	refreshStatus := func() {
		registered := startupTaskExists()

		configMu.RLock()
		configured := config.LaunchAtLogin
		configMu.RUnlock()

		switch {
		case registered && configured:
			status.SetText("Windows startup task is registered.")
		case registered && !configured:
			status.SetText("An existing Bitfocus Listener startup task was found. Apply these settings to replace it.")
		case !registered && configured:
			status.SetText("Startup is enabled in Listener settings, but the Windows task is missing.")
		default:
			status.SetText("Windows startup is disabled.")
		}
	}

	applyButton := widget.NewButton("Apply startup settings", func() {
		launch := launchCheck.Checked
		hidden := hiddenCheck.Checked
		elevated := elevatedCheck.Checked

		if err := configureStartup(launch, hidden, elevated); err != nil {
			dialog.ShowError(err, w)
			refreshStatus()
			return
		}

		configMu.Lock()
		config.LaunchAtLogin = launch
		config.StartHidden = hidden
		config.RunElevated = elevated
		err := saveConfigLocked(&config)
		configMu.Unlock()

		if err != nil {
			dialog.ShowError(err, w)
			return
		}

		auditLog(
			"gui",
			"STARTUP_CHANGED",
			fmt.Sprintf(
				"launch_at_login=%t start_hidden=%t run_elevated=%t",
				launch,
				hidden,
				elevated,
			),
		)

		refreshStatus()
	})
	applyButton.Importance = widget.HighImportance

	info := widget.NewLabel("Uses Windows Task Scheduler directly. No PowerShell, VBS, or helper script is created.")
	info.Wrapping = fyne.TextWrapWord

	elevationWarning := widget.NewLabel("Highest privileges also elevate every enabled remote action, including shell commands.")
	elevationWarning.Wrapping = fyne.TextWrapWord

	content := container.NewPadded(container.NewVBox(
		widget.NewLabel("Windows startup"),
		widget.NewSeparator(),
		launchCheck,
		hiddenCheck,
		elevatedCheck,
		elevationWarning,
		widget.NewSeparator(),
		info,
		applyButton,
		status,
	))

	refreshStatus()
	return container.NewTabItem("Startup", content)
}
