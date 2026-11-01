package detector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGameRejectsMissingGameDirectory(t *testing.T) {
	missingGameDir := filepath.Join(t.TempDir(), "game")

	_, err := DetectGame(missingGameDir)
	if err == nil {
		t.Fatal("DetectGame should reject a non-existent path named game")
	}

	var notFound *GameNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected GameNotFoundError, got %T: %v", err, err)
	}
}

func TestDetectGameAcceptsExistingGameDirectory(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "game")
	if err := os.Mkdir(gameDir, 0755); err != nil {
		t.Fatal(err)
	}

	info, err := DetectGame(gameDir)
	if err != nil {
		t.Fatalf("DetectGame returned an unexpected error: %v", err)
	}
	if info.RootDir != root {
		t.Fatalf("unexpected root directory: got %q, want %q", info.RootDir, root)
	}
	if info.GameDir != gameDir {
		t.Fatalf("unexpected game directory: got %q, want %q", info.GameDir, gameDir)
	}
}
