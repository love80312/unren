// UnRen-Go: Cross-platform Ren'Py game utility
// Ported from the original UnRen Windows batch script
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unren/unren-go/detector"
	"github.com/unren/unren-go/patcher"
	"github.com/unren/unren-go/runner"
	"github.com/unren/unren-go/utils"
)

const version = "0.0.6"

func main() {
	// 解析命令列參數
	var (
		showVersion bool
		gameDir     string

		// 動作旗標
		extract   bool
		decompile bool
		console   bool
		quicksave bool
		skip      bool
		rollback  bool
		all       bool
		clean     bool
	)

	// 自訂使用說明
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "UnRen-Go v%s\n", version)
		fmt.Fprintf(os.Stderr, "用法：%s [選項] [遊戲目錄]\n\n", filepath.Base(os.Args[0]))

		fmt.Fprintln(os.Stderr, "一般選項：")
		fmt.Fprintln(os.Stderr, "  -h, --help       顯示此說明訊息")
		fmt.Fprintln(os.Stderr, "  -v, --version    顯示版本資訊")

		fmt.Fprintln(os.Stderr, "\n動作：")
		fmt.Fprintln(os.Stderr, "  -e, --extract    解包 RPA 壓縮檔")
		fmt.Fprintln(os.Stderr, "  -d, --decompile  反編譯 RPYC 檔案")
		fmt.Fprintln(os.Stderr, "  -a, --all        執行所有動作（解包、反編譯、套用所有修補）")

		fmt.Fprintln(os.Stderr, "\n修補項目：")
		fmt.Fprintln(os.Stderr, "  --console        啟用開發者主控台（SHIFT+O）與選單（SHIFT+D）")
		fmt.Fprintln(os.Stderr, "  --quicksave      啟用快速存檔（F5）與快速讀檔（F9）")
		fmt.Fprintln(os.Stderr, "  --skip           強制啟用略過未讀內容")
		fmt.Fprintln(os.Stderr, "  --rollback       強制啟用回滾功能（滾輪）")

		fmt.Fprintln(os.Stderr, "\n進階：")
		fmt.Fprintln(os.Stderr, "  --clean          成功解包／反編譯後刪除原始檔案（.rpa/.rpyc）")

		fmt.Fprintln(os.Stderr, "\n範例：")
		fmt.Fprintf(os.Stderr, "  %s -e -d /path/to/game\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "  %s --all .\n", filepath.Base(os.Args[0]))
	}

	flag.BoolVar(&showVersion, "version", false, "顯示版本")
	flag.BoolVar(&showVersion, "v", false, "顯示版本（簡短）")

	flag.BoolVar(&extract, "extract", false, "解包 RPA")
	flag.BoolVar(&extract, "e", false, "解包 RPA（簡短）")

	flag.BoolVar(&decompile, "decompile", false, "反編譯 RPYC")
	flag.BoolVar(&decompile, "d", false, "反編譯 RPYC（簡短）")

	flag.BoolVar(&all, "all", false, "執行所有動作")
	flag.BoolVar(&all, "a", false, "執行所有動作（簡短）")

	flag.BoolVar(&console, "console", false, "啟用主控台")
	flag.BoolVar(&quicksave, "quicksave", false, "啟用快速存檔")
	flag.BoolVar(&skip, "skip", false, "啟用略過")
	flag.BoolVar(&rollback, "rollback", false, "啟用回滾")

	flag.BoolVar(&clean, "clean", false, "成功後刪除原始檔案")
	flag.BoolVar(&clean, "c", false, "成功後刪除原始檔案（簡短）")

	flag.Parse()

	if showVersion {
		fmt.Printf("UnRen-Go v%s\n", version)
		return
	}

	// 處理遊戲目錄的位置參數
	if flag.NArg() > 0 {
		gameDir = strings.Trim(flag.Arg(0), "\"'")
	} else {
		gameDir = "."
	}

	// 判斷是否為自動化模式
	automationMode := extract || decompile || console || quicksave || skip || rollback || all

	// 偵測遊戲迴圈
	var game *detector.GameInfo

	for {
		var err error
		game, err = detector.DetectGame(gameDir)
		if err == nil {
			break
		}

		// 驗證失敗
		if automationMode {
			fmt.Println()
			fmt.Printf("   ! 錯誤：找不到遊戲檔案，無法繼續。\n")
			fmt.Printf("     路徑：%s\n", gameDir)
			fmt.Println()
			return
		}

		utils.ClearTerminal()
		fmt.Println()
		fmt.Printf("   ! 錯誤：在以下路徑找不到遊戲檔案：%s\n", gameDir)
		fmt.Println()

		fmt.Println("  復原選項：")
		fmt.Println("    1) 瀏覽遊戲目錄")
		fmt.Println("    2) 顯示說明")
		fmt.Println("    3) 離開")
		fmt.Println()

		option := readInput("請輸入 1-3（或拖放資料夾至此）：")
		fmt.Println()

		// 優先嘗試將輸入視為路徑（支援拖放）
		cleanedPath := strings.Trim(option, "\"'")
		if cleanedPath != "" && utils.DirExists(cleanedPath) {
			gameDir = cleanedPath
			continue
		}

		switch option {
		case "1":
			newDir := browseDirectory(gameDir)
			if newDir != "" {
				gameDir = newDir
				continue // 以新路徑重新偵測
			}
			return // 使用者取消瀏覽
		case "2":
			flag.Usage()
			waitForKey()
			return
		case "3":
			return
		default:
			fmt.Printf("   ! 無效選項：%s\n", option)
			fmt.Println("     請輸入 1、2、3 或有效路徑。")
			continue
		}
	}

	printGameInfo(game)

	if automationMode {
		if all {
			handleAllOptions(game, clean)
			return
		}

		if extract {
			handleExtractRPA(game, clean)
			fmt.Println()
		}
		if decompile {
			handleDecompileRPYC(game, clean)
			fmt.Println()
		}
		if console {
			handleEnableConsole(game)
		}
		if quicksave {
			handleEnableQuickSave(game)
		}
		if skip {
			handleEnableSkip(game)
		}
		if rollback {
			handleEnableRollback(game)
		}
		return
	}

	// 互動模式（傳統）
	// 主選單迴圈
	for {
		utils.ClearTerminal()
		printBanner()
		printGameInfo(game)
		printMenu()
		option := readInput("請輸入 1-8（或其他任意鍵離開）：")
		fmt.Println()
		printSeparator()
		fmt.Println()

		switch option {
		case "1":
			handleExtractRPA(game, false) // 互動模式下為安全起見停用清理，除非另外新增選項
		case "2":
			handleDecompileRPYC(game, false)
		case "3":
			handleEnableConsole(game)
		case "4":
			handleEnableQuickSave(game)
		case "5":
			handleEnableSkip(game)
		case "6":
			handleEnableRollback(game)
		case "7":
			handleOptionsGroup1(game)
		case "8":
			handleAllOptions(game, false)
		default:
			return
		}

		fmt.Println()
		printSeparator()
		fmt.Println()
		fmt.Println("  完成！")
		fmt.Println()

		again := readInput("輸入「1」返回選單，或任意鍵離開：")
		if again != "1" {
			return
		}
		fmt.Println()
	}
}

func printBanner() {
	fmt.Println()
	fmt.Println("     __  __      ____               __          __")
	fmt.Println("    / / / /___  / __ \\___  ____    / /_  ____ _/ /_")
	fmt.Println("   / / / / __ \\/ /_/ / _ \\/ __ \\  / __ \\/ __ `/ __/")
	fmt.Println("  / /_/ / / / / _, _/  __/ / / / / /_/ / /_/ / /_")
	fmt.Println("  \\____/_/ /_/_/ |_|\\___/_/ /_(_)_.___/\\__,_/\\__/ v" + version)
	fmt.Println("   Ported from Sam's script @ f95zone.to/members/sam.7899/")
	fmt.Println()
	printSeparator()
	fmt.Println()
}

func printSeparator() {
	fmt.Println("  ----------------------------------------------------")
}

func printGameInfo(game *detector.GameInfo) {
	fmt.Printf("  遊戲名稱：         %s\n", game.Name)
	if game.RenPyVersion > 0 {
		fmt.Printf("  Ren'Py 版本：      %d.x (", game.RenPyVersion)
		if game.RenPyVersion >= 8 {
			fmt.Print("Python 3")
		} else {
			fmt.Print("Python 2")
		}
		fmt.Println(")")
	}
	fmt.Printf("  RPA 壓縮檔：       %d 個\n", len(game.RPAFiles))
	fmt.Printf("  RPYC 檔案：        %d 個\n", len(game.RPYCFiles))
	fmt.Println()
	printSeparator()
	fmt.Println()
}

func printMenu() {
	fmt.Println("  可用選項：")
	fmt.Println("    1) 解包 RPA 檔案    （位於【game】資料夾）")
	fmt.Println("    2) 反編譯 rpyc 檔案（位於【game】資料夾）")
	fmt.Println()
	fmt.Println("    3) 啟用主控台與開發者選單")
	fmt.Println("    4) 啟用快速　存檔與快速讀檔")
	fmt.Println("    5) 強制啟用　略過未讀內容")
	fmt.Println("    6) 強制啟用　回滾功能（滾輪）")
	fmt.Println()
	fmt.Println("    7) 選項 3-6")
	fmt.Println("    8) 選項 1-6")
	fmt.Println()
}

func handleExtractRPA(game *detector.GameInfo, clean bool) {
	if !game.HasRPAFiles() {
		fmt.Println("  沒有找到可解包的 .rpa 檔案。")
		return
	}

	r, err := runner.NewRunner(game)
	if err != nil {
		fmt.Printf("    ! 找不到 Python 直譯器：%v\n", err)
		fmt.Println("      解包 RPA 需要遊戲內建的 Python 直譯器。")
		return
	}
	defer r.Cleanup()

	fmt.Println("  正從 RPA 壓縮檔　提取中...")
	fmt.Println()

	totalErrors := 0

	for _, rpaPath := range game.RPAFiles {
		relPath, _ := filepath.Rel(game.GameDir, rpaPath)
		info, _ := os.Stat(rpaPath)
		fmt.Printf("    + 正在解包 『%s』 - %s\n", relPath, utils.FormatBytes(info.Size()))

		if err := r.ExtractRPA(rpaPath); err != nil {
			fmt.Printf("    ! 解包失敗 %s：%v\n", relPath, err)
			totalErrors++
		}
	}

	fmt.Println()
	if totalErrors > 0 {
		fmt.Printf("  提取完成，共 %d 個錯誤。\n", totalErrors)
	} else {
		fmt.Println("  提取完成。")

		if clean {
			fmt.Println()
			fmt.Println("  清理 RPA 檔案中...")
			cleaned := 0
			for _, rpaPath := range game.RPAFiles {
				if err := os.Remove(rpaPath); err == nil {
					cleaned++
				} else {
					fmt.Printf("    ! 刪除失敗 %s：%v\n", filepath.Base(rpaPath), err)
				}
			}
			fmt.Printf("    已刪除 %d 個 .rpa 檔案。\n", cleaned)
		}
	}

	// 重新整理 RPYC 檔案清單，以便偵測到新提取的檔案
	if found, err := utils.FindFilesWithExtension(game.GameDir, ".rpyc"); err == nil {
		game.RPYCFiles = found
	}
}

func handleDecompileRPYC(game *detector.GameInfo, clean bool) {
	if !game.HasRPYCFiles() {
		fmt.Println("  沒有找到可反編譯的 .rpyc 檔案。")
		return
	}

	fmt.Println("  設定反編譯器中...")

	r, err := runner.NewRunner(game)
	if err != nil {
		fmt.Printf("    ! 找不到 Python 直譯器：%v\n", err)
		fmt.Println("    ! 反編譯 RPYC 需要遊戲內建的 Python 直譯器。")
		return
	}
	defer r.Cleanup()

	pyVersion := "Python 2"
	if r.IsPython3 {
		pyVersion = "Python 3"
	}
	fmt.Printf("    使用 %s\n", pyVersion)
	fmt.Println()

	fmt.Println("  搜尋 rpyc 檔案中...")
	fmt.Println()

	success, skipped, failed, succeededFiles, err := r.DecompileAllRPYCWithFiles()
	if err != nil {
		fmt.Printf("    ! 反編譯錯誤：%v\n", err)
	}

	fmt.Println()
	fmt.Print("  摘要：")
	parts := []string{}
	if success > 0 {
		parts = append(parts, fmt.Sprintf("成功反編譯 %d 個檔案", success))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d 個檔案反編譯失敗", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d 個檔案已存在，略過", skipped))
	}
	if len(parts) == 0 {
		fmt.Println("未處理任何檔案")
	} else {
		for i, p := range parts {
			if i > 0 {
				fmt.Print("，")
			}
			fmt.Print(p)
		}
		fmt.Println()
	}

	if clean && err == nil && failed == 0 {
		fmt.Println()
		fmt.Println("  清理 RPYC 檔案中...")
		cleaned := 0
		// Only delete sources that this run actually decompiled successfully.
		// Keep skipped files, un.rpyc, and every failed/unprocessed source.
		for _, rpycPath := range succeededFiles {
			if err := os.Remove(rpycPath); err == nil {
				cleaned++
			} else {
				fmt.Printf("    ! 刪除失敗 %s：%v\n", filepath.Base(rpycPath), err)
			}
		}
		fmt.Printf("    已刪除 %d 個 .rpyc 檔案。\n", cleaned)
	} else if clean {
		fmt.Println()
		if failed > 0 {
			fmt.Printf("  ! 因有 %d 個檔案反編譯失敗，略過清理步驟。\n", failed)
		} else {
			fmt.Println("  ! 因反編譯流程發生錯誤，略過清理步驟。")
		}
	}
}

func handleEnableConsole(game *detector.GameInfo) {
	fmt.Println("  建立開發者／主控台設定檔...")

	p := patcher.New(game.GameDir, nil)
	if err := p.EnableConsole(); err != nil {
		fmt.Printf("    ! 失敗：%v\n", err)
		return
	}

	fmt.Println("    + 主控台：SHIFT+O")
	fmt.Println("    + 開發者選單：SHIFT+D")
}

func handleEnableQuickSave(game *detector.GameInfo) {
	fmt.Println("  建立快速存檔／讀檔設定檔...")

	p := patcher.New(game.GameDir, nil)
	if err := p.EnableQuickSave(); err != nil {
		fmt.Printf("    ! 失敗：%v\n", err)
		return
	}

	fmt.Println("    預設快速鍵：")
	fmt.Println("    + 快速存檔：F5")
	fmt.Println("    + 快速讀檔：F9")
}

func handleEnableSkip(game *detector.GameInfo) {
	fmt.Println("  建立略過設定檔...")

	p := patcher.New(game.GameDir, nil)
	if err := p.EnableSkip(); err != nil {
		fmt.Printf("    ! 失敗：%v\n", err)
		return
	}

	fmt.Println("    + 現在可使用 TAB 與 CTRL 鍵略過所有文字")
}

func handleEnableRollback(game *detector.GameInfo) {
	fmt.Println("  建立回滾設定檔...")

	p := patcher.New(game.GameDir, nil)
	if err := p.EnableRollback(); err != nil {
		fmt.Printf("    ! 失敗：%v\n", err)
		return
	}

	fmt.Println("    + 現在可使用滾輪進行回滾")
}

func handleOptionsGroup1(game *detector.GameInfo) {
	handleEnableConsole(game)
	fmt.Println()
	handleEnableQuickSave(game)
	fmt.Println()
	handleEnableSkip(game)
	fmt.Println()
	handleEnableRollback(game)
}

func handleAllOptions(game *detector.GameInfo, clean bool) {
	handleExtractRPA(game, clean)
	fmt.Println()
	handleDecompileRPYC(game, clean)
	fmt.Println()
	handleOptionsGroup1(game)
}

func readInput(prompt string) string {
	fmt.Print("  " + prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func waitForKey() {
	fmt.Print("            按 Enter 鍵離開...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// browseDirectory 提供互動式目錄選擇
func browseDirectory(startPath string) string {
	currentPath, err := filepath.Abs(startPath)
	if err != nil {
		currentPath, _ = os.Getwd()
	}

	for {
		fmt.Println("  ----------------------------------------------------")
		fmt.Printf("  瀏覽中：%s\n", currentPath)
		fmt.Println("  ----------------------------------------------------")

		dirs, err := listSubdirectories(currentPath)
		if err != nil {
			fmt.Printf("  列出目錄時發生錯誤：%v\n", err)
			waitForKey()
			return ""
		}

		fmt.Println("  [..]  上一層")
		fmt.Println("  [.]   選擇目前目錄")
		fmt.Println()

		for i, dir := range dirs {
			fmt.Printf("  [%d]   %s\n", i+1, dir)
		}

		fmt.Println()
		fmt.Println("  輸入數字瀏覽，'u' 上一層，'s' 選擇，'q' 離開")
		input := readInput("請選擇：")

		switch strings.ToLower(input) {
		case "u", "..":
			currentPath = filepath.Dir(currentPath)
		case "s", ".":
			return currentPath
		case "q":
			return ""
		default:
			var selection int
			_, err := fmt.Sscanf(input, "%d", &selection)
			if err == nil {
				if selection > 0 && selection <= len(dirs) {
					currentPath = filepath.Join(currentPath, dirs[selection-1])
					continue
				}
			}
			fmt.Println("  無效的選擇。")
		}
	}
}

// listSubdirectories 回傳指定路徑下所有子目錄名稱
func listSubdirectories(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	return dirs, nil
}
