package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func FindProjectRoot(depth int) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working dir: %w", err)
	}
	for range depth + 1 {
		info, err := os.Stat(filepath.Join(wd, "go.mod"))
		if err == nil && !info.IsDir() {
			return wd, nil
		}
		wd = filepath.Join(wd, "..")
	}
	return "", errors.New("project root not found")
}
