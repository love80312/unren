package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/unren/unren-go/detector"
)

func TestDecompileAllRPYCWithFilesReturnsOnlyNewSuccessfulOutputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell as a fake Python executable")
	}

	gameRoot := t.TempDir()
	gameDir := filepath.Join(gameRoot, "game")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"success.rpyc", "skip.rpyc", "fail.rpyc", "no-output.rpyc", "partial.rpyc", "un.rpyc"} {
		if err := os.WriteFile(filepath.Join(gameDir, name), []byte("compiled"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(gameDir, "skip.rpy"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	pythonScript := filepath.Join(gameRoot, "fake-python.sh")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do target=\"$arg\"; done\n" +
		"case \"$(basename \"$target\")\" in fail.rpyc) printf 'partial output' > \"${target%.*}.rpy\"; exit 9 ;; no-output.rpyc) exit 0 ;; esac\n" +
		"out=\"${target%.*}.rpy\"\n" +
		"if [ \"$(basename \"$target\")\" = partial.rpyc ]; then printf 'partial output' > \"$out\"; exit 0; fi\n" +
		"printf 'decompiled' > \"$out\"\n" +
		"printf '> 1 file were successfully decompiled.\\n'\n"
	if err := os.WriteFile(pythonScript, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		GameInfo:  &detector.GameInfo{RootDir: gameRoot, GameDir: gameDir},
		PythonExe: pythonScript,
		IsPython3: true,
	}
	defer r.Cleanup()

	success, skipped, failed, succeededFiles, err := r.DecompileAllRPYCWithFiles()
	if err != nil {
		t.Fatalf("DecompileAllRPYCWithFiles returned an unexpected error: %v", err)
	}
	if success != 1 || skipped != 1 || failed != 3 {
		t.Fatalf("unexpected counts: success=%d skipped=%d failed=%d", success, skipped, failed)
	}
	if len(succeededFiles) != 1 || succeededFiles[0] != filepath.Join(gameDir, "success.rpyc") {
		t.Fatalf("cleanup candidates must contain only success.rpyc; got %#v", succeededFiles)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "success.rpy")); err != nil {
		t.Fatalf("successful decompilation output was not created: %v", err)
	}
	skipContent, err := os.ReadFile(filepath.Join(gameDir, "skip.rpy"))
	if err != nil {
		t.Fatalf("pre-existing RPY output was not preserved: %v", err)
	}
	if string(skipContent) != "existing" {
		t.Fatalf("pre-existing RPY output was modified: %q", skipContent)
	}
	if _, err := os.Lstat(filepath.Join(gameDir, "partial.rpy")); !os.IsNotExist(err) {
		t.Fatalf("partial output from a failed decompilation must be removed, got error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(gameDir, "fail.rpy")); !os.IsNotExist(err) {
		t.Fatalf("partial output from a non-zero-exit decompilation must be removed, got error: %v", err)
	}
	for _, name := range []string{"skip.rpyc", "fail.rpyc", "no-output.rpyc", "partial.rpyc", "un.rpyc"} {
		if _, err := os.Stat(filepath.Join(gameDir, name)); err != nil {
			t.Fatalf("source %s must remain untouched: %v", name, err)
		}
	}
}

func TestFailedPartialOutputIsRemovedAndCanBeRetried(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell as a fake Python executable")
	}

	gameRoot := t.TempDir()
	gameDir := filepath.Join(gameRoot, "game")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	rpycPath := filepath.Join(gameDir, "retry.rpyc")
	if err := os.WriteFile(rpycPath, []byte("compiled"), 0644); err != nil {
		t.Fatal(err)
	}

	pythonScript := filepath.Join(gameRoot, "fake-python.sh")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do target=\"$arg\"; done\n" +
		"out=\"${target%.*}.rpy\"\n" +
		"attempt=\"${target%.*}.attempted\"\n" +
		"if [ ! -e \"$attempt\" ]; then\n" +
		"  printf 'attempted' > \"$attempt\"\n" +
		"  printf 'partial output' > \"$out\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf 'complete output' > \"$out\"\n" +
		"printf '> 1 file were successfully decompiled.\\n'\n"
	if err := os.WriteFile(pythonScript, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		GameInfo:  &detector.GameInfo{RootDir: gameRoot, GameDir: gameDir},
		PythonExe: pythonScript,
		IsPython3: true,
	}
	defer r.Cleanup()

	// First run simulates a decompiler that writes a partial .rpy but reports
	// no successful decompilation. The partial output must be removed, while
	// the original .rpyc must remain available for a retry.
	success, skipped, failed, candidates, err := r.DecompileAllRPYCWithFiles()
	if err != nil {
		t.Fatalf("first decompilation run failed unexpectedly: %v", err)
	}
	if success != 0 || skipped != 0 || failed != 1 || len(candidates) != 0 {
		t.Fatalf("unexpected first-run result: success=%d skipped=%d failed=%d candidates=%#v", success, skipped, failed, candidates)
	}
	if _, err := os.Lstat(filepath.Join(gameDir, "retry.rpy")); !os.IsNotExist(err) {
		t.Fatalf("partial .rpy should be removed after failure, got error: %v", err)
	}
	if _, err := os.Stat(rpycPath); err != nil {
		t.Fatalf("original .rpyc must remain after failure: %v", err)
	}

	// Second run sees no stale .rpy and can execute the decompiler again.
	success, skipped, failed, candidates, err = r.DecompileAllRPYCWithFiles()
	if err != nil {
		t.Fatalf("retry failed unexpectedly: %v", err)
	}
	if success != 1 || skipped != 0 || failed != 0 || len(candidates) != 1 || candidates[0] != rpycPath {
		t.Fatalf("unexpected retry result: success=%d skipped=%d failed=%d candidates=%#v", success, skipped, failed, candidates)
	}
	content, err := os.ReadFile(filepath.Join(gameDir, "retry.rpy"))
	if err != nil {
		t.Fatalf("successful retry did not leave an RPY output: %v", err)
	}
	if string(content) != "complete output" {
		t.Fatalf("unexpected retry output: %q", content)
	}
	if _, err := os.Stat(rpycPath); err != nil {
		t.Fatalf("successful decompilation must not delete the RPYC source by itself: %v", err)
	}
}

func TestCleanupFailedRPYOutputRemovesSymlinkWithoutTouchingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "output.rpy")
	if err := os.WriteFile(target, []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	removed, err := cleanupFailedRPYOutput(link)
	if err != nil {
		t.Fatalf("cleanupFailedRPYOutput returned an unexpected error: %v", err)
	}
	if !removed {
		t.Fatal("expected the newly created symlink itself to be removed")
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("symlink should be removed, got error: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("symlink target must remain: %v", err)
	}
	if string(content) != "keep me" {
		t.Fatalf("symlink target was modified: %q", content)
	}
}

func TestCleanupFailedRPYOutputPreservesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.rpy")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}

	removed, err := cleanupFailedRPYOutput(path)
	if err == nil {
		t.Fatal("expected a non-regular output path to be preserved with an error")
	}
	if removed {
		t.Fatal("a directory must not be reported as removed")
	}
	info, statErr := os.Stat(path)
	if statErr != nil || !info.IsDir() {
		t.Fatalf("directory must remain untouched, stat error: %v", statErr)
	}
}

func TestDecompileAllRPYCWithFilesReturnsNoCleanupCandidatesOnSetupError(t *testing.T) {
	var r Runner
	_, _, _, succeededFiles, err := r.DecompileAllRPYCWithFiles()
	if err == nil {
		t.Fatal("expected setup error for an uninitialized Runner")
	}
	if len(succeededFiles) != 0 {
		t.Fatalf("setup failure must not produce cleanup candidates: %#v", succeededFiles)
	}
}

func TestPythonBitnessUsesRenPyPlatformNames(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"windows", "amd64", "x86_64"},
		{"windows", "386", "i686"},
		{"windows", "arm64", "x86_64"},
		{"linux", "arm64", "aarch64"},
		{"linux", "arm", "armv7l"},
		{"darwin", "arm64", "arm64"},
	}
	for _, test := range tests {
		t.Run(test.goos+"/"+test.goarch, func(t *testing.T) {
			if got := pythonBitness(test.goos, test.goarch); got != test.want {
				t.Fatalf("pythonBitness(%q, %q) = %q, want %q", test.goos, test.goarch, got, test.want)
			}
		})
	}
}

func TestNewRunnerRejectsNilGameInfo(t *testing.T) {
	if _, err := NewRunner(nil); err == nil {
		t.Fatal("NewRunner should reject nil game information")
	}
}

func TestSetupUnrpycRejectsCallerSuppliedTempDir(t *testing.T) {
	gameRoot := t.TempDir()
	providedDir := t.TempDir()
	marker := filepath.Join(providedDir, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		GameInfo:  &detector.GameInfo{RootDir: gameRoot},
		TempDir:   providedDir,
		IsPython3: true,
	}
	if err := r.SetupUnrpyc(); err == nil {
		t.Fatal("SetupUnrpyc should reject a caller-supplied temporary directory")
	}
	if _, err := os.Stat(filepath.Join(providedDir, "decompiler")); !os.IsNotExist(err) {
		t.Fatalf("SetupUnrpyc wrote into an unowned directory: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("caller-owned directory contents were changed: %v", err)
	}
}

func TestExtractRPARejectsCallerSuppliedTempDir(t *testing.T) {
	gameRoot := t.TempDir()
	providedDir := t.TempDir()
	marker := filepath.Join(providedDir, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		GameInfo:  &detector.GameInfo{RootDir: gameRoot},
		TempDir:   providedDir,
		PythonExe: "unused",
		IsPython3: true,
	}
	if err := r.ExtractRPA(filepath.Join(gameRoot, "game.rpa")); err == nil {
		t.Fatal("ExtractRPA should reject a caller-supplied temporary directory")
	}
	if _, err := os.Stat(filepath.Join(providedDir, "_rpatool.py")); !os.IsNotExist(err) {
		t.Fatalf("ExtractRPA wrote into an unowned directory: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("caller-owned directory contents were changed: %v", err)
	}
}
