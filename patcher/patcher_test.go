package patcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableConsoleBacksUpExistingFileAndIsIdempotent(t *testing.T) {
	gameDir := t.TempDir()
	path := filepath.Join(gameDir, "unren-dev.rpy")
	original := []byte("# user-owned file\nlabel start:\n    return\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	p := New(gameDir, nil)
	if err := p.EnableConsole(); err != nil {
		t.Fatalf("EnableConsole failed: %v", err)
	}

	backup, err := os.ReadFile(path + ".org")
	if err != nil {
		t.Fatalf("original backup was not created: %v", err)
	}
	if string(backup) != string(original) {
		t.Fatalf("backup changed original contents: got %q, want %q", backup, original)
	}

	generated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "config.console = True") {
		t.Fatalf("expected generated console patch, got: %s", generated)
	}

	// Running the same patch again must be harmless and keep the first backup.
	if err := p.EnableConsole(); err != nil {
		t.Fatalf("second EnableConsole call should be idempotent: %v", err)
	}
	backupAfter, err := os.ReadFile(path + ".org")
	if err != nil {
		t.Fatal(err)
	}
	if string(backupAfter) != string(original) {
		t.Fatal("the original backup was modified on the second run")
	}
}

func TestEnableConsoleDoesNotOverwriteUserModifiedPatch(t *testing.T) {
	gameDir := t.TempDir()
	path := filepath.Join(gameDir, "unren-dev.rpy")
	original := []byte("original user data\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	p := New(gameDir, nil)
	if err := p.EnableConsole(); err != nil {
		t.Fatal(err)
	}

	userEdit := []byte("# user modified generated patch\nconfig.console = False\n")
	if err := os.WriteFile(path, userEdit, 0644); err != nil {
		t.Fatal(err)
	}
	if err := p.EnableConsole(); err == nil {
		t.Fatal("expected an error rather than overwriting a user-modified patch")
	}

	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(userEdit) {
		t.Fatalf("user edit was overwritten: got %q", actual)
	}
	backup, err := os.ReadFile(path + ".org")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatalf("original backup changed: got %q", backup)
	}
}

func TestWriteFileRefusesSymlinkTarget(t *testing.T) {
	gameDir := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.rpy")
	if err := os.WriteFile(outside, []byte("do not touch"), 0644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(gameDir, "unren-dev.rpy")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation is unavailable on this platform: %v", err)
	}

	p := New(gameDir, nil)
	if err := p.EnableConsole(); err == nil {
		t.Fatal("expected a symlink target to be rejected")
	}
	actual, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != "do not touch" {
		t.Fatalf("symlink target was modified: %q", actual)
	}
}

func TestWriteFilePreservesExistingBackup(t *testing.T) {
	gameDir := t.TempDir()
	path := filepath.Join(gameDir, "unren-dev.rpy")
	backup := path + ".org"
	original := []byte("original data")
	current := []byte("different existing data")
	if err := os.WriteFile(path, current, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, original, 0644); err != nil {
		t.Fatal(err)
	}

	p := New(gameDir, nil)
	if err := p.EnableConsole(); err == nil {
		t.Fatal("expected existing backup conflict to stop replacement")
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(current) {
		t.Fatalf("current file was overwritten: %q", actual)
	}
	actualBackup, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(actualBackup) != string(original) {
		t.Fatalf("existing backup was overwritten: %q", actualBackup)
	}
}

func TestWriteFileRecoversWhenBackupMatchesUnchangedOriginal(t *testing.T) {
	gameDir := t.TempDir()
	path := filepath.Join(gameDir, "unren-dev.rpy")
	original := []byte("original file left behind by an interrupted patch run")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".org", original, 0644); err != nil {
		t.Fatal(err)
	}

	p := New(gameDir, nil)
	if err := p.EnableConsole(); err != nil {
		t.Fatalf("expected safe recovery when current file still matches .org: %v", err)
	}
	backup, err := os.ReadFile(path + ".org")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatal("recovery changed the pre-existing backup")
	}
}

func TestWriteFileRejectsSymlinkBackup(t *testing.T) {
	gameDir := t.TempDir()
	outsideDir := t.TempDir()
	path := filepath.Join(gameDir, "unren-dev.rpy")
	backup := path + ".org"
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideDir, "backup.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, backup); err != nil {
		t.Skipf("symlink creation is unavailable on this platform: %v", err)
	}

	p := New(gameDir, nil)
	if err := p.EnableConsole(); err == nil {
		t.Fatal("expected symlink backup path to be rejected")
	}
	actual, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != "outside" {
		t.Fatalf("backup symlink target was modified: %q", actual)
	}
}
