package version

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Release struct {
	TagName string `json:"tagName"`
	Name    string `json:"name"`
}

func binaryName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	if osName == "windows" {
		return fmt.Sprintf("lazydiff-%s-%s.exe", osName, arch)
	}
	return fmt.Sprintf("lazydiff-%s-%s", osName, arch)
}

func CheckForUpdate() (bool, string, error) {
	if Current == "dev" {
		return false, "", nil
	}
	out, err := exec.Command("gh", "release", "view",
		"--repo", "alex-irvine/lazydiff",
		"--json", "tagName,name",
	).Output()
	if err != nil {
		return false, "", fmt.Errorf("gh release view: %w", err)
	}
	var release Release
	if err := json.Unmarshal(out, &release); err != nil {
		return false, "", fmt.Errorf("parse release: %w", err)
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Current, "v")
	if latest != current {
		return true, release.TagName, nil
	}
	return false, release.TagName, nil
}

func PerformUpdate() error {
	if Current == "dev" {
		return fmt.Errorf("cannot update dev build")
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}
	tmpFile := exePath + ".new"
	cmd := exec.Command("gh", "release", "download",
		"--repo", "alex-irvine/lazydiff",
		"--pattern", binaryName(),
		"--output", tmpFile,
		"--clobber",
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("gh release download: %w", err)
	}
	if err := os.Chmod(tmpFile, 0755); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("chmod new binary: %w", err)
	}
	oldFile := exePath + ".old"
	os.Remove(oldFile)
	if err := os.Rename(exePath, oldFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(tmpFile, exePath); err != nil {
		os.Rename(oldFile, exePath)
		os.Remove(tmpFile)
		return fmt.Errorf("install new binary: %w", err)
	}
	os.Remove(oldFile)
	return nil
}
