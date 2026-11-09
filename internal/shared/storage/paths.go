package storage

import (
	"os"
	"path/filepath"
)

const (
	appDirName       = ".portico"
	legacyHotkeyDir  = "SuiNest"
	legacyHotkeyFile = "suidemo.db"
)

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
