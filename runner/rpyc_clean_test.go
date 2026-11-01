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

	for _, name := range []string{"success.rpyc", "skip.rpyc", "fail.rpyc", "no-output.rpyc", "un.rpyc"} {
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
		"case \"$(basename \"$target\")\" in fail.rpyc) exit 9 ;; no-output.rpyc) exit 0 ;; esac\n" +
		"out=\"${target%.*}.rpy\"\n" +
		"printf 'decompiled' > \"$out\"\n"
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
	if success != 1 || skipped != 1 || failed != 2 {
		t.Fatalf("unexpected counts: success=%d skipped=%d failed=%d", success, skipped, failed)
	}
	if len(succeededFiles) != 1 || succeededFiles[0] != filepath.Join(gameDir, "success.rpyc") {
		t.Fatalf("cleanup candidates must contain only success.rpyc; got %#v", succeededFiles)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "success.rpy")); err != nil {
		t.Fatalf("successful decompilation output was not created: %v", err)
	}
	for _, name := range []string{"skip.rpyc", "fail.rpyc", "no-output.rpyc", "un.rpyc"} {
		if _, err := os.Stat(filepath.Join(gameDir, name)); err != nil {
			t.Fatalf("source %s must remain untouched: %v", name, err)
		}
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
