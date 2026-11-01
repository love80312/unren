package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unren/unren-go/detector"
)

func TestSetupTempDirDoesNotWriteIntoGameRoot(t *testing.T) {
	gameRoot := t.TempDir()

	// Files and directories with names historically used by Cleanup must survive.
	preserved := map[string]string{
		"_rpatool.py":                 "user rpatool",
		"_rpa.py":                     "user fallback",
		"unrpyc.py":                   "user decompiler",
		"decompiler/user_module.py":   "user package file",
		"__pycache__/user_module.pyc": "user bytecode",
	}
	for name, content := range preserved {
		path := filepath.Join(gameRoot, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	r := &Runner{
		GameInfo:  &detector.GameInfo{RootDir: gameRoot},
		IsPython3: true,
	}
	if err := r.SetupTempDir(); err != nil {
		t.Fatalf("SetupTempDir failed: %v", err)
	}
	createdTempDir := r.TempDir
	if createdTempDir == gameRoot {
		t.Fatal("temporary directory must not be the game root")
	}
	if _, err := os.Stat(filepath.Join(createdTempDir, "_rpatool.py")); err != nil {
		t.Fatalf("embedded rpatool was not extracted into the private temp directory: %v", err)
	}

	r.Cleanup()
	if r.TempDir != "" || r.ownsTempDir {
		t.Fatal("Cleanup should clear the owned temporary directory state")
	}
	if _, err := os.Stat(createdTempDir); !os.IsNotExist(err) {
		t.Fatalf("private temporary directory was not removed: %v", err)
	}

	for name, expected := range preserved {
		actual, err := os.ReadFile(filepath.Join(gameRoot, name))
		if err != nil {
			t.Fatalf("game data %q was removed: %v", name, err)
		}
		if string(actual) != expected {
			t.Fatalf("game data %q was modified: got %q, want %q", name, actual, expected)
		}
	}
}

func TestCleanupDoesNotRemoveUnownedDirectory(t *testing.T) {
	providedDir := t.TempDir()
	sentinel := filepath.Join(providedDir, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{TempDir: providedDir}
	r.Cleanup()

	actual, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("Cleanup removed an unowned directory: %v", err)
	}
	if string(actual) != "keep" {
		t.Fatalf("unexpected sentinel contents: %q", actual)
	}
}

func TestSetupTempDirRejectsCallerSuppliedDirectory(t *testing.T) {
	providedDir := t.TempDir()
	marker := filepath.Join(providedDir, "marker.txt")
	if err := os.WriteFile(marker, []byte("untouched"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		GameInfo: &detector.GameInfo{RootDir: providedDir},
		TempDir:  providedDir,
	}
	if err := r.SetupTempDir(); err == nil {
		t.Fatal("SetupTempDir should reject a directory it did not create")
	}
	if _, err := os.Stat(filepath.Join(providedDir, "_rpatool.py")); !os.IsNotExist(err) {
		t.Fatalf("SetupTempDir unexpectedly wrote into caller directory: %v", err)
	}
}

func TestLocalizeToolOutput(t *testing.T) {
	input := []byte("Decompiling \"script.rpyc\" to \"script.rpy\" ...\n" +
		"Could not extract file script.rpy from archive: missing entry\n" +
		"To overwrite existing files instead of skipping them, use the --clobber flag.\n")

	got := localizeToolOutput(input)
	for _, expected := range []string{
		"正在反編譯 \"script.rpyc\"",
		"無法從封包解出檔案 script.rpy：missing entry",
		"若要覆寫已存在的輸出檔案",
		"--clobber",
	} {
		if !strings.Contains(got, expected) {
			t.Errorf("localized output does not contain %q; got:\n%s", expected, got)
		}
	}
}

func TestLocalizeToolOutputKeepsPathsAndTechnicalTerms(t *testing.T) {
	input := []byte("Input path not found: C:\\Games\\RenPy\\game\\script.rpyc\n" +
		"Unable to find the right slot to load from the rpyc file. The file header structure has been changed. File header: 01020304\n")

	got := localizeToolOutput(input)
	for _, expected := range []string{
		"找不到輸入路徑：C:\\Games\\RenPy\\game\\script.rpyc",
		"RPYC",
		"01020304",
	} {
		if !strings.Contains(got, expected) {
			t.Errorf("localized output does not preserve/include %q; got:\n%s", expected, got)
		}
	}
}

func TestLocalizeRPAToolErrors(t *testing.T) {
	input := []byte("Could not open archive file game.rpa for reading: permission denied\n" +
		"Could not extract file script.rpy from archive: missing entry\n")

	got := localizeToolOutput(input)
	for _, expected := range []string{
		"無法開啟封包檔案 game.rpa 以供讀取：permission denied",
		"無法從封包解出檔案 script.rpy：missing entry",
	} {
		if !strings.Contains(got, expected) {
			t.Errorf("localized output does not contain %q; got:\n%s", expected, got)
		}
	}
}

func TestLocalizeDecompilerStatus(t *testing.T) {
	input := []byte("Found 1 file to process. Performing decompilation using 1 worker.\n" +
		"Processed 1 file.\n" +
		"> 1 file were successfully decompiled.\n")

	got := localizeToolOutput(input)
	for _, expected := range []string{
		"找到 1 個待處理檔案，使用 1 個工作程序進行反編譯。",
		"已處理 1 個檔案。",
		"成功反編譯 1 個檔案。",
	} {
		if !strings.Contains(got, expected) {
			t.Errorf("localized output does not contain %q; got:\n%s", expected, got)
		}
	}
}
