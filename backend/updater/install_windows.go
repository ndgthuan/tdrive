//go:build windows

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	mediaDirName = "media"
	toolsDirName = "tools"
)

type windowsInstaller struct {
	executable func() (string, error)
	swapPaths  func(current, fresh string) error
}

func newPlatformInstaller() Installer {
	return windowsInstaller{
		executable: os.Executable,
		swapPaths:  swapPaths,
	}
}

func (w windowsInstaller) Target() (Target, error) {
	exe, err := w.executable()
	if err != nil {
		return Target{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return Target{Path: exe, Kind: "exe"}, nil
}

// Install swaps the executable and the bundled media and downloader runtimes. Windows lets a
// running .exe be renamed but not deleted, which is exactly what swapPaths
// relies on; the runtime folders must not be in use, so callers close native
// players and downloader processes first.
func (w windowsInstaller) Install(payload string, target Target) error {
	swap := w.swapPaths
	if swap == nil {
		swap = swapPaths
	}

	dir := filepath.Dir(target.Path)
	staging := stagingPath(dir)
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("clear staging: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := extractZip(payload, staging); err != nil {
		return fmt.Errorf("unpack update: %w", err)
	}

	newExe, err := findSingleExecutable(staging)
	if err != nil {
		return err
	}

	currentMedia := filepath.Join(dir, mediaDirName)
	newMedia := filepath.Join(staging, mediaDirName)
	mediaSwapped, mediaCreated, err := swapOptionalRuntimeDir(swap, currentMedia, newMedia, "media runtime")
	if err != nil {
		return err
	}
	currentTools := filepath.Join(dir, toolsDirName)
	newTools := filepath.Join(staging, toolsDirName)
	toolsSwapped, toolsCreated, err := swapOptionalRuntimeDir(swap, currentTools, newTools, "downloader runtime")
	if err != nil {
		_ = rollbackRuntimeDir(currentMedia, newMedia, mediaSwapped, mediaCreated)
		return err
	}

	if err := swap(target.Path, newExe); err != nil {
		mediaRollbackErr := rollbackRuntimeDir(currentMedia, newMedia, mediaSwapped, mediaCreated)
		toolsRollbackErr := rollbackRuntimeDir(currentTools, newTools, toolsSwapped, toolsCreated)
		if mediaRollbackErr != nil || toolsRollbackErr != nil {
			return fmt.Errorf("%w (runtime rollback failed: media=%v tools=%v)", err, mediaRollbackErr, toolsRollbackErr)
		}
		return err
	}
	return nil
}

// swapOptionalRuntimeDir installs one payload directory when present. Older
// releases did not carry the downloader runtime, so a payload without tools
// deliberately leaves an existing tools directory untouched.
func swapOptionalRuntimeDir(swap func(current, fresh string) error, current, fresh, label string) (swapped, created bool, err error) {
	info, err := os.Stat(fresh)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("%s: %w", label, err)
	}
	if !info.IsDir() {
		return false, false, fmt.Errorf("%s: payload path is not a directory", label)
	}
	if _, err := os.Stat(current); err == nil {
		if err := swap(current, fresh); err != nil {
			return false, false, fmt.Errorf("%s: %w", label, err)
		}
		return true, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, false, fmt.Errorf("%s: inspect installed directory: %w", label, err)
	}
	if err := os.Rename(fresh, current); err != nil {
		return false, false, fmt.Errorf("%s: %w", label, err)
	}
	return false, true, nil
}

func rollbackRuntimeDir(current, fresh string, swapped, created bool) error {
	if swapped {
		if err := os.Rename(current, fresh); err != nil {
			return err
		}
		return os.Rename(current+previousSuffix, current)
	}
	if created {
		return os.RemoveAll(current)
	}
	return nil
}

// findSingleExecutable returns the one top-level .exe in dir.
func findSingleExecutable(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var exes []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".exe") {
			exes = append(exes, filepath.Join(dir, entry.Name()))
		}
	}
	if len(exes) != 1 {
		return "", errNoAppInPayload
	}
	return exes[0], nil
}

func (w windowsInstaller) Relaunch(target Target, waitPID int) error {
	cmd := exec.Command(target.Path, WaitForPIDFlag, strconv.Itoa(waitPID))
	cmd.Dir = filepath.Dir(target.Path)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new version: %w", err)
	}
	return cmd.Process.Release()
}

// Cleanup retries briefly: Windows can keep the old image mapped for a moment
// after the previous process exits.
func (w windowsInstaller) Cleanup(target Target) error {
	dir := filepath.Dir(target.Path)
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = removeAllIgnoringMissing(target.Path + previousSuffix)
		if mediaErr := removeAllIgnoringMissing(filepath.Join(dir, mediaDirName+previousSuffix)); err == nil {
			err = mediaErr
		}
		if toolsErr := removeAllIgnoringMissing(filepath.Join(dir, toolsDirName+previousSuffix)); err == nil {
			err = toolsErr
		}
		if stagingErr := removeStaging(dir); err == nil {
			err = stagingErr
		}
		if err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return err
}

func removeAllIgnoringMissing(path string) error {
	err := os.RemoveAll(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
