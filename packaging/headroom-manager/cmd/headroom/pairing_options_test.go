package main

import "testing"

func TestRecoveryOptionsAreExplicitAndUnambiguous(t *testing.T) {
	for _, args := range [][]string{nil, {"--this-install-only"}, {"--this-install-only", "--version", "2.1.0"}, {"--reconcile"}} {
		if _, err := parseUpdateOptions(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"--this-install-only", "--version", ""}, {"--version=", "--reconcile"}, {"--version", "2.1.0"}, {"--this-install-only", "--reconcile"}, {"--version", "2.1.0", "--reconcile"}, {"unexpected"}, {"--this-install-only", "--version", "not-a-version"}, {"--unknown"}} {
		if _, err := parseUpdateOptions(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
