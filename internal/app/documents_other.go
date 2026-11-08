//go:build !windows

package app

import (
	"os"
	"path/filepath"
)

func documentsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Documents"), nil
}
