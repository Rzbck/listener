package main

import (
	"os"
	"testing"
)

func TestHiddenLaunchRequested(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "default launch", args: []string{"listener"}, want: false},
		{name: "hidden launch", args: []string{"listener", "--hidden"}, want: true},
		{name: "other argument", args: []string{"listener", "--example"}, want: false},
		{name: "hidden among arguments", args: []string{"listener", "--example", "--hidden"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			if got := hiddenLaunchRequested(); got != tt.want {
				t.Fatalf("hiddenLaunchRequested() = %v, want %v", got, tt.want)
			}
		})
	}
}
