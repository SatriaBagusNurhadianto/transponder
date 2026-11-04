package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{name: "version flag", args: []string{"--version"}, wantCode: 0, wantOut: "transponder dev"},
		{name: "unknown flag", args: []string{"--nope"}, wantCode: 2, wantOut: ""},
		{name: "no gateway yet", args: nil, wantCode: 1, wantOut: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Fatalf("stdout = %q, want it to contain %q", stdout.String(), tt.wantOut)
			}
		})
	}
}
