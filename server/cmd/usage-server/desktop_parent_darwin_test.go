//go:build darwin

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDesktopParentLifetime(t *testing.T) {
	// This fixture only runs the parent watcher. It constructs no providers,
	// starts no API server, and cannot invoke a usage reset.
	if mode := os.Getenv("HEADROOM_PARENT_FIXTURE"); mode != "" {
		marker := os.Getenv("HEADROOM_PARENT_MARKER")
		if mode == "child" {
			ctx, cancel := desktopParentContext(context.Background())
			defer cancel()
			if err := os.WriteFile(marker+".ready", []byte("ready"), 0600); err != nil {
				t.Fatal(err)
			}
			select {
			case <-ctx.Done():
				if err := os.WriteFile(marker+".stopped", []byte("stopped"), 0600); err != nil {
					t.Fatal(err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("orphaned desktop watcher remained active")
			}
			return
		}
		child := exec.Command(os.Args[0], "-test.run=^TestDesktopParentLifetime$")
		child.Env = append(os.Environ(), "HEADROOM_PARENT_FIXTURE=child")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		defer child.Process.Kill()
		time.Sleep(20 * time.Second)
		return
	}
	marker := filepath.Join(t.TempDir(), "lifetime")
	owner := exec.Command(os.Args[0], "-test.run=^TestDesktopParentLifetime$")
	owner.Env = append(os.Environ(), "HEADROOM_PARENT_FIXTURE=owner", "HEADROOM_PARENT_MARKER="+marker)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Process.Kill()
	waitFile := func(name string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(name); err == nil {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("parent lifecycle marker was not written: %s", filepath.Base(name))
	}
	waitFile(marker + ".ready")
	if _, err := os.Stat(marker + ".stopped"); !os.IsNotExist(err) {
		t.Fatal("desktop parent watcher stopped while owner was alive")
	}
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = owner.Wait()
	waitFile(marker + ".stopped")
}
