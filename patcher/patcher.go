// Package patcher creates .rpy modification files for Ren'Py games.
// It uses embedded templates for consistency and maintainability.
package patcher

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unren/unren-go/files"
)

// Config holds configuration for the patcher.
type Config struct {
	// QuickSaveKey is the Ren'Py key constant for quick save (default: K_F5)
	QuickSaveKey string
	// QuickLoadKey is the Ren'Py key constant for quick load (default: K_F9)
	QuickLoadKey string
}

// DefaultConfig returns the default patcher configuration.
func DefaultConfig() *Config {
	return &Config{
		QuickSaveKey: "K_F5",
		QuickLoadKey: "K_F9",
	}
}

// Patcher creates .rpy modification files for a Ren'Py game.
type Patcher struct {
	gameDir string
	config  *Config
}

// New creates a new Patcher for the given game directory.
func New(gameDir string, config *Config) *Patcher {
	if config == nil {
		config = DefaultConfig()
	}
	return &Patcher{
		gameDir: gameDir,
		config:  config,
	}
}

// EnableConsole creates unren-dev.rpy to enable developer console and menu.
// Console: SHIFT+O | Dev Menu: SHIFT+D
func (p *Patcher) EnableConsole() error {
	content, err := files.GetRPYContent(files.RPYFiles.Dev)
	if err != nil {
		return fmt.Errorf("無法載入開發者主控台範本：%w", err)
	}
	return p.writeFile("unren-dev.rpy", content)
}

// EnableQuickSave creates unren-quick.rpy to enable quick save/load hotkeys.
func (p *Patcher) EnableQuickSave() error {
	data := files.QuickSaveConfig{
		QuickSaveKey: p.config.QuickSaveKey,
		QuickLoadKey: p.config.QuickLoadKey,
	}

	content, err := files.GetRPYTemplated(files.RPYFiles.Quick, data)
	if err != nil {
		return fmt.Errorf("無法處理快速存檔範本：%w", err)
	}

	return p.writeFile("unren-quick.rpy", content)
}

// EnableSkip creates unren-skip.rpy to enable skipping unseen content.
func (p *Patcher) EnableSkip() error {
	content, err := files.GetRPYContent(files.RPYFiles.Skip)
	if err != nil {
		return fmt.Errorf("無法載入略過功能範本：%w", err)
	}
	return p.writeFile("unren-skip.rpy", content)
}

// EnableRollback creates unren-rollback.rpy to enable infinite rollback.
func (p *Patcher) EnableRollback() error {
	content, err := files.GetRPYContent(files.RPYFiles.Rollback)
	if err != nil {
		return fmt.Errorf("無法載入回滾功能範本：%w", err)
	}
	return p.writeFile("unren-rollback.rpy", content)
}

// EnableAll enables all patching features (console, quick save, skip, rollback).
func (p *Patcher) EnableAll() error {
	if err := p.EnableConsole(); err != nil {
		return fmt.Errorf("開發者主控台：%w", err)
	}
	if err := p.EnableQuickSave(); err != nil {
		return fmt.Errorf("快速存檔：%w", err)
	}
	if err := p.EnableSkip(); err != nil {
		return fmt.Errorf("略過功能：%w", err)
	}
	if err := p.EnableRollback(); err != nil {
		return fmt.Errorf("回滾功能：%w", err)
	}
	return nil
}

// PatchFiles returns the list of all patch file names.
func PatchFiles() []string {
	return []string{
		"unren-dev.rpy",
		"unren-quick.rpy",
		"unren-skip.rpy",
		"unren-rollback.rpy",
	}
}

// RemoveAll removes UnRen patch files. It intentionally does not delete .org
// backups; those are user data and can be restored manually.
func (p *Patcher) RemoveAll() error {
	for _, f := range PatchFiles() {
		path := filepath.Join(p.gameDir, f)
		if err := p.removeIfExists(path); err != nil {
			return err
		}
	}
	return nil
}

// removeIfExists removes a file if it exists, ignoring "not found" errors.
func (p *Patcher) removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("無法刪除 %s：%w", filepath.Base(path), err)
	}
	return nil
}

// writeFile preserves an existing file as <name>.org before replacing it.
// If that backup already exists, only an identical UnRen output is accepted;
// modified or ambiguous files are left untouched for manual review.
func (p *Patcher) writeFile(filename string, content []byte) error {
	if p.gameDir == "" {
		return fmt.Errorf("尚未設定遊戲目錄")
	}
	if err := os.MkdirAll(p.gameDir, 0755); err != nil {
		return fmt.Errorf("無法建立遊戲目錄：%w", err)
	}

	path := filepath.Join(p.gameDir, filename)
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("拒絕覆寫符號連結：%s", filename)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("目標不是一般檔案，拒絕覆寫：%s", filename)
		}

		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("無法讀取既有檔案 %s：%w", filename, readErr)
		}
		if bytes.Equal(existing, content) {
			// Running the same option repeatedly is harmless and does not
			// replace or alter the original backup.
			return nil
		}

		backupPath := path + ".org"
		backupInfo, backupErr := os.Lstat(backupPath)
		if backupErr == nil {
			if backupInfo.Mode()&os.ModeSymlink != 0 || !backupInfo.Mode().IsRegular() {
				return fmt.Errorf("備份路徑不是一般檔案，拒絕使用：%s", backupPath)
			}
			backupContents, readErr := os.ReadFile(backupPath)
			if readErr != nil {
				return fmt.Errorf("無法讀取備份檔 %s：%w", backupPath, readErr)
			}
			if !bytes.Equal(existing, backupContents) {
				return fmt.Errorf("備份檔已存在，且目前檔案與備份內容不同；為避免覆蓋使用者修改，已保留原檔：%s", backupPath)
			}
			// Recovery case: an earlier run created the backup but stopped
			// before replacing the original. The original and backup still match.
		} else if !os.IsNotExist(backupErr) {
			return fmt.Errorf("無法檢查備份檔 %s：%w", backupPath, backupErr)
		} else {
			if err := writeBackupExclusive(backupPath, existing, info.Mode().Perm()); err != nil {
				return fmt.Errorf("無法備份原始檔案 %s：%w", filename, err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("無法檢查目標檔案 %s：%w", filename, err)
	}

	if err := writeFileAtomic(path, content, 0644); err != nil {
		return fmt.Errorf("無法寫入 %s：%w", filename, err)
	}
	return nil
}

func writeBackupExclusive(path string, content []byte, mode os.FileMode) (retErr error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			_ = os.Remove(path)
		}
	}()

	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if mode != 0 {
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
	}
	return nil
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) (retErr error) {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".unren-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return nil
}
