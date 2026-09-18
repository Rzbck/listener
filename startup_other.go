//go:build !windows

package main

import "errors"

func startupSupported() bool {
	return false
}

func startupTaskExists() bool {
	return false
}

func configureStartup(enabled, hidden, elevated bool) error {
	return errors.New("startup integration is only available on Windows")
}
