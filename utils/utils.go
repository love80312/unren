package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ClearTerminal clears the terminal screen in a cross-platform way.
func ClearTerminal() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
}

// Colors for terminal output.
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
)

// supportsColor checks if the terminal supports ANSI colors.
func supportsColor() bool {
	if runtime.GOOS == "windows" {
		return os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != ""
	}
	return os.Getenv("NO_COLOR") == ""
}

// ColorPrint prints colored text if supported.
func ColorPrint(color, text string) {
	if supportsColor() {
		fmt.Print(color + text + ColorReset)
		return
	}
	fmt.Print(text)
}

// ColorPrintln prints colored text with newline if supported.
func ColorPrintln(color, text string) {
	if supportsColor() {
		fmt.Println(color + text + ColorReset)
		return
	}
	fmt.Println(text)
}

// PrintError prints an error message in red.
func PrintError(format string, args ...interface{}) {
	ColorPrintln(ColorRed, "ERROR: "+fmt.Sprintf(format, args...))
}

// PrintSuccess prints a success message in green.
func PrintSuccess(format string, args ...interface{}) {
	ColorPrintln(ColorGreen, fmt.Sprintf(format, args...))
}

// PrintWarning prints a warning message in yellow.
func PrintWarning(format string, args ...interface{}) {
	ColorPrintln(ColorYellow, "WARNING: "+fmt.Sprintf(format, args...))
}

// PrintInfo prints an info message in cyan.
func PrintInfo(format string, args ...interface{}) {
	ColorPrintln(ColorCyan, fmt.Sprintf(format, args...))
}

// NormalizePath normalizes a path for the current OS.
func NormalizePath(path string) string {
	return filepath.Clean(path)
}

// EnsureDir creates a directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// FileExists reports whether path resolves to an existing non-directory file.
// Stat can return a nil FileInfo for errors other than not-exist; always check
// the error before dereferencing the returned FileInfo.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info == nil {
		return false
	}
	return !info.IsDir()
}

// DirExists reports whether path resolves to an existing directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info == nil {
		return false
	}
	return info.IsDir()
}

// FindFilesWithExtension finds files with the given extension recursively.
func FindFilesWithExtension(dir, ext string) ([]string, error) {
	var files []string
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return fmt.Errorf("無法取得路徑資訊：%s", path)
		}
		if !info.IsDir() && strings.ToLower(filepath.Ext(path)) == ext {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// FormatBytes formats bytes into a human-readable representation.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
