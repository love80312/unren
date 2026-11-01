package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRenPyVersionFromPycacheDirectory(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "renpy", "__pycache__")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	got := detectRenPyVersion(&GameInfo{RootDir: root})
	if got != 8 {
		t.Fatalf("expected Ren'Py 8 from __pycache__ directory, got %d", got)
	}
}

func TestDetectRenPyVersionDoesNotTreatPycacheFileAsDirectory(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "renpy", "__pycache__")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	got := detectRenPyVersion(&GameInfo{RootDir: root})
	if got != 0 {
		t.Fatalf("expected unknown version from a regular __pycache__ file, got %d", got)
	}
}
