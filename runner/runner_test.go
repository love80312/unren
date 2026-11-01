package runner

import (
	"strings"
	"testing"
)

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
