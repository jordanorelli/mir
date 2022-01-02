package main

import (
	"testing"
)

func TestIncrMinor(t *testing.T) {
	var tests = []struct {
		in  string
		out string
	}{
		{"v0.0.0", "v0.1.0"},
	}
	t.Logf("%v", tests)
}
