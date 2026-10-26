package main

import "testing"

func TestWebKitCompositingModeDefault(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		current string
		want    string
	}{
		{name: "Linux defaults to software compositing", goos: "linux", want: "1"},
		{name: "Linux honors explicit override", goos: "linux", current: "0", want: "0"},
		{name: "non-Linux remains unchanged", goos: "windows", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webKitCompositingModeDefault(tt.goos, tt.current); got != tt.want {
				t.Fatalf("webKitCompositingModeDefault(%q, %q) = %q, want %q", tt.goos, tt.current, got, tt.want)
			}
		})
	}
}
