// Package runner provides functionality to execute embedded Python scripts
// using the game's bundled Python interpreter, similar to the original UnRen batch.
package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unren/unren-go/detector"
	"github.com/unren/unren-go/files"
)

// Runner manages extraction and execution of embedded Python scripts.
type Runner struct {
	GameInfo  *detector.GameInfo
	TempDir   string
	PythonExe string
	PythonLib string
	IsPython3 bool

	// ownsTempDir is true only when this Runner created TempDir itself.
	// Cleanup must never remove a caller-supplied or game-owned directory.
	ownsTempDir bool
}

// NewRunner creates a new runner for the detected game.
func NewRunner(gameInfo *detector.GameInfo) (*Runner, error) {
	if gameInfo == nil {
		return nil, fmt.Errorf("尚未設定遊戲資訊")
	}

	r := &Runner{
		GameInfo: gameInfo,
	}

	// Find Python executable
	pythonExe, pythonLib, isPy3, err := r.findPython()
	if err != nil {
		return nil, err
	}
	r.PythonExe = pythonExe
	r.PythonLib = pythonLib
	r.IsPython3 = isPy3

	// Ensure Python executable is executable on Unix-like systems
	if runtime.GOOS != "windows" && r.PythonExe != "" {
		if info, err := os.Stat(r.PythonExe); err == nil {
			if info.Mode()&0111 == 0 {
				if err := os.Chmod(r.PythonExe, info.Mode()|0111); err != nil {
					return nil, fmt.Errorf("無法設定 Python 執行權限：%w", err)
				}
			}
		}
	}

	return r, nil
}

// findPython locates the Python interpreter bundled with the game.
// Returns: pythonExe path, pythonLib path, isPython3, error
func (r *Runner) findPython() (string, string, bool, error) {
	if r.GameInfo.RootDir == "" {
		return "", "", false, fmt.Errorf("尚未設定遊戲根目錄")
	}

	if !r.GameInfo.HasLib || r.GameInfo.LibDir == "" {
		return "", "", false, fmt.Errorf("找不到 lib 目錄")
	}
	libDir := r.GameInfo.LibDir

	// Determine OS prefix and bitness
	var osPrefix string
	switch runtime.GOOS {
	case "windows":
		osPrefix = "windows"
	case "linux":
		osPrefix = "linux"
	case "darwin":
		osPrefix = "mac"
	default:
		osPrefix = "linux"
	}

	bitness := pythonBitness(runtime.GOOS, runtime.GOARCH)

	// Only select a Mac App Bundle interpreter when this binary is running
	// on macOS. A Windows/Linux build cannot execute the bundled Mac binary.
	if runtime.GOOS == "darwin" {
		// Contents/Resources/lib -> Contents/MacOS/python.
		macExePath := filepath.Join(filepath.Dir(filepath.Dir(libDir)), "MacOS", "python")
		if _, err := os.Stat(macExePath); err == nil {
			return macExePath, libDir, true, nil
		}
	}

	// Try Python 3 paths first (Ren'Py 8)
	py3Patterns := []string{
		filepath.Join(libDir, fmt.Sprintf("py3-%s-%s", osPrefix, bitness)),
		filepath.Join(libDir, fmt.Sprintf("python3-%s-%s", osPrefix, bitness)),
	}

	for _, pyDir := range py3Patterns {
		pythonExe := r.getPythonExe(pyDir)
		if pythonExe != "" {
			pythonLib := r.findPythonLib(pyDir)
			return pythonExe, pythonLib, true, nil
		}
	}

	// Try Python 2 paths (Ren'Py 7)
	py2Patterns := []string{
		filepath.Join(libDir, fmt.Sprintf("py2-%s-%s", osPrefix, bitness)),
		filepath.Join(libDir, fmt.Sprintf("python2-%s-%s", osPrefix, bitness)),
		filepath.Join(libDir, fmt.Sprintf("%s-%s", osPrefix, bitness)), // Legacy
	}

	for _, pyDir := range py2Patterns {
		pythonExe := r.getPythonExe(pyDir)
		if pythonExe != "" {
			pythonLib := r.findPythonLib(pyDir)
			return pythonExe, pythonLib, false, nil
		}
	}

	// Fallback: search recursively
	pythonExe, pythonLib, isPy3 := r.searchForPython(libDir)
	if pythonExe != "" {
		return pythonExe, pythonLib, isPy3, nil
	}

	return "", "", false, fmt.Errorf("在以下目錄中找不到 Python 執行檔：%s", libDir)
}

// pythonBitness maps the host OS/architecture to Ren'Py's platform suffixes.
// Ren'Py uses Linux aarch64/armv7l, macOS arm64, and Windows x86_64/i686.
func pythonBitness(goos, goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "386":
		return "i686"
	}

	switch goos {
	case "linux":
		switch goarch {
		case "arm64":
			return "aarch64"
		case "arm":
			return "armv7l"
		}
	case "darwin":
		if goarch == "arm64" {
			return "arm64"
		}
	case "windows":
		if goarch == "arm64" {
			// Ren'Py's Windows runtime is x86_64; Windows on ARM can run it
			// through its x64 emulation layer.
			return "x86_64"
		}
	}

	return goarch
}

// getPythonExe returns the python executable path if it exists.
func (r *Runner) getPythonExe(pyDir string) string {
	var exeName string
	if runtime.GOOS == "windows" {
		exeName = "python.exe"
	} else {
		exeName = "python"
	}

	pythonPath := filepath.Join(pyDir, exeName)
	if _, err := os.Stat(pythonPath); err == nil {
		return pythonPath
	}
	return ""
}

// searchForPython recursively searches for python executable.
func (r *Runner) searchForPython(libDir string) (string, string, bool) {
	pythonNames := []string{"python", "python.exe", "python3", "python3.exe"}

	var found string
	var foundDir string
	var isPy3 bool

	filepath.Walk(libDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		for _, name := range pythonNames {
			if strings.EqualFold(info.Name(), name) {
				found = path
				foundDir = filepath.Dir(path)
				isPy3 = strings.Contains(strings.ToLower(path), "py3") ||
					strings.Contains(strings.ToLower(path), "python3")
				return filepath.SkipAll
			}
		}
		return nil
	})

	if found != "" {
		pythonLib := r.findPythonLib(foundDir)
		return found, pythonLib, isPy3
	}
	return "", "", false
}

// findPythonLib finds the Python library directory containing encodings/.
func (r *Runner) findPythonLib(pythonHome string) string {
	// Look for pythonXX or python3.X directories with encodings
	entries, err := os.ReadDir(pythonHome)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(strings.ToLower(entry.Name()), "python") {
			libPath := filepath.Join(pythonHome, entry.Name())
			if _, err := os.Stat(filepath.Join(libPath, "encodings")); err == nil {
				return libPath
			}
		}
	}

	// Also check lib/pythonX.Y paths
	var foundLib string
	filepath.Walk(pythonHome, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if strings.HasPrefix(strings.ToLower(info.Name()), "python") {
			if _, err := os.Stat(filepath.Join(path, "encodings")); err == nil {
				foundLib = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	return foundLib
}

// SetupTempDir creates a temporary directory and extracts scripts.
// The batch script uses the game root directory (maindir) for these scripts.
func (r *Runner) SetupTempDir() error {
	if r.TempDir != "" {
		if r.ownsTempDir {
			return nil
		}
		return fmt.Errorf("拒絕寫入未由 UnRen 建立的暫存目錄：%s", r.TempDir)
	}
	if r.GameInfo == nil || r.GameInfo.RootDir == "" {
		return fmt.Errorf("尚未設定遊戲根目錄，無法建立暫存目錄")
	}

	// Keep helper scripts and Python cache files out of the game directory.
	// MkdirTemp creates a private directory (0700 on Unix-like systems).
	tempDir, err := os.MkdirTemp("", "unren-")
	if err != nil {
		return fmt.Errorf("無法建立 UnRen 暫存目錄：%w", err)
	}
	r.TempDir = tempDir
	r.ownsTempDir = true

	// Extract rpatool and the fallback extractor into our private directory.
	rpatool, err := files.GetRPATool(r.IsPython3)
	if err != nil {
		r.Cleanup()
		return fmt.Errorf("無法讀取 rpatool：%w", err)
	}
	if err := os.WriteFile(filepath.Join(r.TempDir, "_rpatool.py"), rpatool, 0600); err != nil {
		r.Cleanup()
		return fmt.Errorf("無法寫入 rpatool：%w", err)
	}

	rpaFallback, err := files.GetRPAFallback()
	if err != nil {
		r.Cleanup()
		return fmt.Errorf("無法讀取 rpa.py：%w", err)
	}
	if err := os.WriteFile(filepath.Join(r.TempDir, "_rpa.py"), rpaFallback, 0600); err != nil {
		r.Cleanup()
		return fmt.Errorf("無法寫入 rpa.py：%w", err)
	}

	return nil
}

// SetupUnrpyc extracts the unrpyc decompiler.
func (r *Runner) SetupUnrpyc() error {
	// Always validate ownership, even when TempDir was set by the caller.
	// SetupTempDir is idempotent for this Runner's own directory and rejects
	// caller-supplied paths to prevent writing into arbitrary directories.
	if err := r.SetupTempDir(); err != nil {
		return err
	}

	return files.ExtractUnrpyc(r.TempDir, r.IsPython3)
}

// Cleanup removes the temporary files created beside the game executable.
func (r *Runner) Cleanup() {
	// TempDir used to point at the game root. Removing fixed names and entire
	// subdirectories there could delete pre-existing game files. Only remove
	// the isolated directory created by this Runner instance.
	if r.TempDir == "" || !r.ownsTempDir {
		return
	}

	_ = os.RemoveAll(r.TempDir)
	r.TempDir = ""
	r.ownsTempDir = false
}

// getPythonEnv returns the environment variables for Python execution.
func (r *Runner) getPythonEnv() []string {
	pythonHome := filepath.Dir(r.PythonExe)

	env := os.Environ()
	env = append(env, fmt.Sprintf("PYTHONHOME=%s", pythonHome))

	// Build PYTHONPATH
	// Batch script sets: PYTHONPATH=%pythondir%;%pythonlibdir%;%maindir%;%decompilerdir%\
	paths := []string{pythonHome}
	if r.PythonLib != "" {
		paths = append(paths, r.PythonLib)
	}
	// Add game root (maindir) and decompiler dir
	if r.TempDir != "" {
		paths = append(paths, r.TempDir)
		paths = append(paths, filepath.Join(r.TempDir, "decompiler"))
	} else {
		// Fallback if TempDir not set (shouldn't happen if Setup called)
		paths = append(paths, r.GameInfo.RootDir)
		paths = append(paths, filepath.Join(r.GameInfo.RootDir, "decompiler"))
	}

	sep := string(os.PathListSeparator)
	env = append(env, fmt.Sprintf("PYTHONPATH=%s", strings.Join(paths, sep)))

	return env
}

// localizeToolOutput translates known user-facing messages emitted by the embedded
// rpatool and unrpyc scripts. It deliberately leaves paths, filenames, tracebacks,
// command-line flags, and technical identifiers unchanged.
func localizeToolOutput(output []byte) string {
	s := string(output)
	replacer := strings.NewReplacer(
		"warning: pickle5 module could not be loaded and Python version is < 3.8,", "警告：無法載入 pickle5 模組，且 Python 版本低於 3.8，",
		"newer Ren'Py games may fail to unpack!", "較新的 Ren'Py 遊戲可能無法解包！",
		"if this occurs, fix it by installing pickle5:", "若發生此情況，請安裝 pickle5：",
		"if this occurs, please upgrade to a newer Python (>= 3.5).", "若發生此情況，請升級至較新的 Python（>= 3.5）。",
		"the given file is not a valid Ren'Py archive, or an unsupported version", "指定檔案不是有效的 Ren'Py 封包，或使用不支援的版本",
		"the requested file ", "要求的檔案 ",
		" does not exist in the given Ren'Py archive", " 不存在於指定的 Ren'Py 封包中",
		" does not exist in this archive", " 不存在於此封包中",
		"no target file found for saving archive", "找不到要儲存的封包目標檔案",
		"saving is only supported for version 2 and 3 archives", "僅支援儲存為 RPA 2.0 或 3.0 格式",
		"Could not open archive file ", "無法開啟封包檔案 ",
		" for reading: ", " 以供讀取：",
		"Could not add file ", "無法加入檔案 ",
		" to archive: ", " 至封包：",
		"Could not save archive file:", "無法儲存封包檔案：",
		"Could not delete file ", "無法從封包刪除檔案 ",
		"Could not extract file ", "無法從封包解出檔案 ",
		" from archive: ", "：",
		"No operation given :( ", "未指定要執行的操作。 ",
		"No operation given :(\n", "未指定要執行的操作。\n",
		"Use ", "請使用 ",
		" --help for usage details.", " --help 查看使用方式。",
		"The multiprocessing module is not available or broken!", "無法使用多程序模組！",
		"Proceeding with single threaded decompilation. This will be slow.", "改以單執行緒進行反編譯，處理速度會較慢。",
		"Warning: Encountered an unexpected slot structure. It is possible the \n    file header structure has been changed.", "警告：偵測到非預期的資料區塊結構，檔案標頭結構可能已遭修改。",
		"Unable to find the right slot to load from the rpyc file. The file header structure has been changed. File header:", "無法從 RPYC 檔案找到可載入的資料區塊。檔案標頭結構可能已遭修改。檔案標頭：",
		"Did not find a zlib compressed blob where it was expected. Either the header has been modified or the file structure has been changed. File header:", "在預期位置找不到 zlib 壓縮資料。檔案標頭或檔案結構可能已遭修改。檔案標頭：",
		"Warning: analysis found signs that this .rpyc file was generated by ren'py ", "警告：分析結果顯示，此 .rpyc 檔案可能由 Ren'Py ",
		" or below, while this unrpyc version targets ren'py \n    version 8. Decompilation will still be attempted, but errors or incorrect \n    decompilation might occur. ", " 或更舊版本產生，而目前的 unrpyc 版本主要針對 Ren'Py 8。仍會嘗試反編譯，但可能發生錯誤或產生不正確的結果。 ",
		"Skipping ", "略過 ",
		" already exists.", " 已存在。",
		"Decompiling ", "正在反編譯 ",
		"Extracting translations from ", "正在擷取翻譯資料：",
		"Error while extracting translations from ", "擷取翻譯資料時發生錯誤：",
		"Error while decompiling ", "反編譯時發生錯誤：",
		"Input path not found: ", "找不到輸入路徑：",
		"Found no script files to decompile.", "找不到可反編譯的腳本檔案。",
		"Found 1 file to process. Performing decompilation using 1 worker.", "找到 1 個待處理檔案，使用 1 個工作程序進行反編譯。",
		"Processed 1 file.", "已處理 1 個檔案。",
		"> 1 file were successfully decompiled.", "成功反編譯 1 個檔案。",
		"> 1 file did not have the correct header, these were ignored.", "1 個檔案的標頭格式不正確，已略過。",
		"> 1 file failed to decompile due to errors.", "1 個檔案反編譯失敗。",
		"> 1 file were skipped as the output file already existed.", "1 個檔案因輸出檔案已存在而略過。",
		"> 1 file failed translation extraction.", "1 個檔案擷取翻譯資料失敗。",
		"To overwrite existing files instead of skipping them, use the --clobber flag.", "若要覆寫已存在的輸出檔案，而非略過，請使用 --clobber 參數。",
		"To attempt to bypass modifications to the file header, use the --try-harder flag.", "若要嘗試處理遭修改的檔案標頭，請使用 --try-harder 參數。",
		"Errors were encountered during decompilation. Check the log for more information.", "反編譯過程發生錯誤，請查看記錄以取得詳細資訊。",
		"When making a bug report, please include this entire log.", "回報問題時，請附上完整的記錄內容。",
		"Bad format in custom sl displayable registration:", "自訂 SL displayable 註冊格式錯誤：",
		"Bad child node count in custom sl displayable registration:", "自訂 SL displayable 的子節點數量錯誤：",
		"must be executed with Python 3.9 or later.", "必須使用 Python 3.9 或更新版本執行。",
		"must be executed with Python 2.7.", "必須使用 Python 2.7 執行。",
	)
	return replacer.Replace(s)
}

// runWithLocalizedOutput executes a child process and writes its captured output
// after translating known interface messages. Exit status and command behavior
// remain unchanged.
func runWithLocalizedOutput(cmd *exec.Cmd) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if stdout.Len() > 0 {
		fmt.Print(localizeToolOutput(stdout.Bytes()))
	}
	if stderr.Len() > 0 {
		fmt.Fprint(os.Stderr, localizeToolOutput(stderr.Bytes()))
	}

	output := make([]byte, 0, stdout.Len()+stderr.Len())
	output = append(output, stdout.Bytes()...)
	output = append(output, stderr.Bytes()...)
	return output, err
}

// ExtractRPA extracts an RPA archive using the embedded rpatool.
func (r *Runner) ExtractRPA(rpaPath string) error {
	// Validate ownership even when TempDir was pre-populated by a caller.
	if err := r.SetupTempDir(); err != nil {
		return err
	}

	rpatoolPath := filepath.Join(r.TempDir, "_rpatool.py")
	rpaFallbackPath := filepath.Join(r.TempDir, "_rpa.py")
	outputDir := filepath.Dir(rpaPath)
	env := r.getPythonEnv()

	// First try rpatool.py with extract flag
	cmd := exec.Command(r.PythonExe, "-O", rpatoolPath, "-x", rpaPath, "-o", outputDir)
	cmd.Dir = outputDir
	cmd.Env = env

	if _, err := runWithLocalizedOutput(cmd); err != nil {
		// Fallback to rpa.py (simpler extractor)
		fmt.Println("    改用備援解包工具重試...")
		cmd = exec.Command(r.PythonExe, rpaFallbackPath, rpaPath)
		cmd.Dir = outputDir
		cmd.Env = env

		if _, err := runWithLocalizedOutput(cmd); err != nil {
			return fmt.Errorf("RPA 解包失敗：%w", err)
		}
	}

	return nil
}

// DecompileRPYC decompiles an RPYC file to RPY.
func (r *Runner) DecompileRPYC(rpycPath string) error {
	if err := r.SetupUnrpyc(); err != nil {
		return fmt.Errorf("無法設定 unrpyc：%w", err)
	}

	unrpycPath := filepath.Join(r.TempDir, "unrpyc.py")
	env := r.getPythonEnv()

	args := []string{"-O", unrpycPath}
	if !r.IsPython3 {
		args = append(args, "--init-offset")
	}
	args = append(args, rpycPath)

	cmd := exec.Command(r.PythonExe, args...)
	cmd.Dir = filepath.Dir(rpycPath)
	cmd.Env = env

	_, err := runWithLocalizedOutput(cmd)
	return err
}

// DecompileAllRPYC decompiles all RPYC files in the game directory.
// Returns: success count, skipped count, failed count, error.
// This compatibility wrapper preserves the original method signature.
func (r *Runner) DecompileAllRPYC() (int, int, int, error) {
	success, skipped, failed, _, err := r.DecompileAllRPYCWithFiles()
	return success, skipped, failed, err
}

// DecompileAllRPYCWithFiles decompiles all RPYC files and also returns the
// source RPYC paths whose corresponding RPY outputs were newly created and
// verified as regular files. Callers may remove only these returned paths.
func (r *Runner) DecompileAllRPYCWithFiles() (int, int, int, []string, error) {
	if err := r.SetupUnrpyc(); err != nil {
		return 0, 0, 0, nil, fmt.Errorf("無法設定 unrpyc：%w", err)
	}

	unrpycPath := filepath.Join(r.TempDir, "unrpyc.py")
	env := r.getPythonEnv()
	gameDir := r.GameInfo.GameDir

	success := 0
	skipped := 0
	failed := 0
	var succeededFiles []string
	lastDir := ""

	walkErr := filepath.Walk(gameDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("無法存取路徑 %s：%w", path, walkErr)
		}
		if info == nil {
			return fmt.Errorf("無法取得路徑資訊：%s", path)
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".rpyc") {
			return nil
		}

		// Skip un.rpyc (special Ren'Py file).
		baseName := filepath.Base(path)
		nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		if strings.ToLower(nameWithoutExt) == "un" {
			return nil
		}

		// Show directory change (like batch script).
		currentDir := filepath.Dir(path)
		if currentDir != lastDir {
			lastDir = currentDir
			relDir, _ := filepath.Rel(r.GameInfo.RootDir, currentDir)
			if relDir == "" {
				relDir = "."
			}
			fmt.Printf("  目前目錄：'%s'\n", relDir)
		}

		// Never treat a pre-existing output (including a symlink) as a newly
		// decompiled file. It must not be eligible for --clean removal.
		rpyPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".rpy"
		if _, statErr := os.Lstat(rpyPath); statErr == nil {
			fmt.Printf("    \"%s.rpy\" 已存在，略過\n", nameWithoutExt)
			skipped++
			return nil
		} else if !os.IsNotExist(statErr) {
			return fmt.Errorf("無法檢查輸出檔案 %s：%w", rpyPath, statErr)
		}

		// Show decompiling message with file size (like batch script).
		fmt.Printf("    + 正在反編譯 \"%s\" - %d bytes\n", baseName, info.Size())

		args := []string{"-O", unrpycPath}
		if !r.IsPython3 {
			args = append(args, "--init-offset")
		}
		args = append(args, path)

		cmd := exec.Command(r.PythonExe, args...)
		cmd.Dir = filepath.Dir(path)
		cmd.Env = env

		if output, cmdErr := cmd.CombinedOutput(); cmdErr == nil {
			// unrpyc currently reports per-file worker failures in its summary
			// but still exits with status 0. Require the explicit one-file success
			// marker as well as a newly created regular output before --clean may
			// consider the RPYC source safe to remove.
			if !bytes.Contains(output, []byte("> 1 file were successfully decompiled.")) {
				fmt.Printf("    - 反編譯器未確認成功完成 %s，保留原始 RPYC 檔案。\n", baseName)
				if len(output) > 0 {
					fmt.Printf("    子程序輸出：\n%s\n", localizeToolOutput(output))
				}
				failed++
				return nil
			}

			outInfo, statErr := os.Lstat(rpyPath)
			if statErr == nil && outInfo.Mode().IsRegular() {
				success++
				succeededFiles = append(succeededFiles, path)
			} else {
				fmt.Printf("    - 無法建立有效的 RPY 檔案：找不到一般檔案 %s.rpy。\n", nameWithoutExt)
				if len(output) > 0 {
					fmt.Printf("    子程序輸出：\n%s\n", localizeToolOutput(output))
				}
				failed++
			}
		} else {
			fmt.Printf("    - 反編譯 \"%s\" 失敗。錯誤：%v\n", baseName, cmdErr)
			if len(output) > 0 {
				fmt.Printf("    子程序輸出：\n%s\n", localizeToolOutput(output))
			}
			failed++
		}

		return nil
	})

	return success, skipped, failed, succeededFiles, walkErr
}
