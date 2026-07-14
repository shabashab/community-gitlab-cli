package benchmark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMakeWorkspaceAccessiblePreservesExecutableBitsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "script.sh")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}

	if err := makeWorkspaceAccessible(root); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o766 {
		t.Fatalf("executable mode = %o, want 766", got)
	}
	outsideInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := outsideInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("symlink target mode changed to %o", got)
	}
}
