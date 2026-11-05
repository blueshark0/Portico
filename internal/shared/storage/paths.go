package storage

import (
	"os"
	"path/filepath"
)

const (
	appDirName       = ".portico"
	legacyAppDirName = ".code-switch"
	legacyHotkeyDir  = "SuiNest"
	legacyHotkeyFile = "suidemo.db"
)

// MigrateLegacyAppDataDir preserves existing profiles, logs and proxy state
// across the Code Switch -> Portico rename. It runs before logging creates any
// file in the new directory; an existing Portico directory is never merged or
// overwritten.
func MigrateLegacyAppDataDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	currentDir := filepath.Join(home, appDirName)
	if _, err := os.Stat(currentDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	legacyDir := filepath.Join(home, legacyAppDirName)
	if _, err := os.Stat(legacyDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(legacyDir, currentDir)
}

func AppDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, appDirName), nil
}

func AppDataPath(name string) (string, error) {
	dir, err := AppDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func LegacyHotkeyDBPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, legacyHotkeyDir, legacyHotkeyFile), nil
}
