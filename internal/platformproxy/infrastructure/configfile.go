package infrastructure

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
)

func readConfigFile(path string, missingContent []byte) (data []byte, existed bool, err error) {
	data, err = os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return append([]byte(nil), missingContent...), false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func writeConfigFileIfChanged(path string, before, after []byte) error {
	if bytes.Equal(before, after) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWriteFile(path, after, 0o600)
}

func removeConfigFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
