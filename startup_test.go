package main

import "testing"

func TestContainsHiddenLaunchArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "default launch", args: nil, want: false},
		{name: "hidden launch", args: []string{"--hidden"}, want: true},
		{name: "other argument", args: []string{"--example"}, want: false},
		{name: "hidden among arguments", args: []string{"--example", "--hidden"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsHiddenLaunchArg(tt.args); got != tt.want {
				t.Fatalf("containsHiddenLaunchArg() = %v, want %v", got, tt.want)
			}
		})
	}
}
