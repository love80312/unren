package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileExistsAndDirExists(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(file, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing")

	if !FileExists(file) {
		t.Fatal("FileExists should return true for a regular file")
	}
	if FileExists(root) {
		t.Fatal("FileExists should return false for a directory")
	}
	if FileExists(missing) {
		t.Fatal("FileExists should return false for a missing path")
	}
	if !DirExists(root) {
		t.Fatal("DirExists should return true for a directory")
	}
	if DirExists(file) {
		t.Fatal("DirExists should return false for a regular file")
	}
	if DirExists(missing) {
		t.Fatal("DirExists should return false for a missing path")
	}
}
