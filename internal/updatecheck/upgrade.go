// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

// This file implements `nvfleetint upgrade` by delegating to the
// release's own installer rather than replacing the binary directly. The
// installers verify the release checksum and, per platform, the Developer ID
// signature and notarization ticket or the Authenticode signature. Reproducing
// that here would mean maintaining a second, weaker copy of it.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// installerTimeout bounds fetching the installer script. The install itself
	// is not bounded here: the installer applies its own download timeouts, and
	// a slow link should not abort an upgrade midway.
	installerTimeout = 30 * time.Second

	// maxInstallerBytes caps the installer download.
	maxInstallerBytes = 1 << 20
)

// UpgradePlan describes the upgrade that `nvfleetint upgrade` would perform.
type UpgradePlan struct {
	// CurrentVersion is the running build's version.
	CurrentVersion string
	// Release is the release to install.
	Release Release
	// ExecutablePath is the binary that will be replaced, with symlinks resolved.
	ExecutablePath string
	// InstallDir is where the installer is told to write, i.e. the directory
	// holding the running binary.
	InstallDir string
	// InstallerURL is the installer published with Release.
	InstallerURL string
}

// ErrInstallerUnavailable reports that a release publishes no installer for the
// running platform, so it cannot be installed in place.
var ErrInstallerUnavailable = errors.New("no installer for this release")

// NewUpgradePlan describes how to replace the running binary with release. It
// hangs off Checker so the installer and the release lookup are addressed
// through one base URL.
func (c Checker) NewUpgradePlan(current string, release Release) (UpgradePlan, error) {
	executable, err := os.Executable()
	if err != nil {
		return UpgradePlan{}, fmt.Errorf("could not locate the running executable: %w", err)
	}
	// Resolve symlinks so an installation reached through one replaces the real
	// binary rather than overwriting the link with a file.
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}

	return UpgradePlan{
		CurrentVersion: current,
		Release:        release,
		ExecutablePath: executable,
		InstallDir:     filepath.Dir(executable),
		// Pin the installer to the resolved tag rather than the `latest`
		// permalink, so the script and the archive it downloads come from one
		// release even if a new one is published mid-upgrade.
		InstallerURL: fmt.Sprintf("%s/download/%s/%s", c.releasesURL(), release.Version, installerName()),
	}, nil
}

// CheckInstallerAvailable reports whether the target release actually publishes
// the installer this platform needs.
//
// It runs before the confirmation prompt, because not every published release
// can be installed in place: releases made before the installers were added to
// the release ship no install.sh or install.ps1 at all. Without this the user
// confirms an upgrade, and only then does the installer download 404 — a
// confirmation for something that was never going to work.
func (p UpgradePlan) CheckInstallerAvailable(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, p.InstallerURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "nvfleetint")

	client := http.Client{Timeout: installerTimeout}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("could not reach the installer for %s: %w", p.Release.Version, err)
	}
	defer func() { _ = response.Body.Close() }()

	switch {
	case response.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: nvfleetint %s publishes no %s, so it cannot be installed in place; "+
			"it predates the installer. Download it from %s and install it by hand",
			ErrInstallerUnavailable, p.Release.Version, installerName(), p.releasePageURL())
	case response.StatusCode < 200 || response.StatusCode > 299:
		return fmt.Errorf("could not reach the installer for %s: unexpected status %d",
			p.Release.Version, response.StatusCode)
	}
	return nil
}

// releasePageURL is where a user is sent to install a release by hand.
func (p UpgradePlan) releasePageURL() string {
	if p.Release.URL != "" {
		return p.Release.URL
	}
	return releasesPage + tagSegment + p.Release.Version
}

// Summary renders what the upgrade will do, for the confirmation prompt.
func (p UpgradePlan) Summary() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "nvfleetint %s -> %s\n", DisplayVersion(p.CurrentVersion), p.Release.Version)
	fmt.Fprintf(&builder, "Install dir: %s\n", p.InstallDir)
	fmt.Fprintf(&builder, "\nThis runs the official installer (%s), which\nverifies the release checksum and code signature.\n", p.InstallerURL)
	return builder.String()
}

// CheckWritable reports whether the installer will be able to replace the
// binary. It runs before anything is downloaded, so a system-wide installation
// that needs elevated privileges fails immediately and says so, rather than
// after a download and a confirmation.
func (p UpgradePlan) CheckWritable() error {
	file, err := os.CreateTemp(p.InstallDir, ".nvfleetint-upgrade-*")
	if err != nil {
		return fmt.Errorf("%s is not writable, so the upgrade would fail; "+
			"re-run with elevated privileges or upgrade manually with: %s",
			p.InstallDir, ManualUpgradeCommand())
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return nil
}

// Run downloads the release's installer and executes it, streaming the
// installer's own output to progress.
func (p UpgradePlan) Run(ctx context.Context, progress io.Writer) error {
	script, err := downloadInstaller(ctx, p.InstallerURL)
	if err != nil {
		return err
	}

	workDir, err := os.MkdirTemp("", "nvfleetint-upgrade-")
	if err != nil {
		return fmt.Errorf("could not create a temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	// Write the installer out and run it from disk rather than piping it into a
	// shell: a truncated download then fails to run at all, instead of executing
	// however much of it arrived.
	scriptPath := filepath.Join(workDir, installerName())
	if err := os.WriteFile(scriptPath, script, 0o700); err != nil {
		return fmt.Errorf("could not write the installer: %w", err)
	}

	moved, err := p.moveRunningBinaryAside()
	if err != nil {
		return err
	}

	command := p.installerCommand(ctx, scriptPath)
	command.Stdout = progress
	command.Stderr = progress
	if err := command.Run(); err != nil {
		if restoreErr := moved.restore(); restoreErr != nil {
			return fmt.Errorf("the installer failed: %w; additionally failed to restore the original executable: %v", err, restoreErr)
		}
		return fmt.Errorf("the installer failed: %w", err)
	}

	moved.discard()
	return nil
}

// movedBinary undoes or finalizes moving the running binary out of the way.
type movedBinary struct {
	restore func() error
	discard func()
}

// moveRunningBinaryAside makes room for the installer on Windows, which cannot
// overwrite a running executable but can rename one. Everywhere else replacing
// the file is safe, because the running process keeps the old inode open.
func (p UpgradePlan) moveRunningBinaryAside() (movedBinary, error) {
	noop := movedBinary{restore: func() error { return nil }, discard: func() {}}
	if runtime.GOOS != "windows" {
		return noop, nil
	}

	backup := p.ExecutablePath + ".old"
	// A previous upgrade cannot delete its own backup while still running, so
	// clear a leftover one now that nothing holds it open.
	_ = os.Remove(backup)
	if err := os.Rename(p.ExecutablePath, backup); err != nil {
		return noop, fmt.Errorf("could not move the running executable aside: %w", err)
	}

	return movedBinary{
		restore: func() error { return restoreWindowsBinary(backup, p.ExecutablePath) },
		// The backup is still mapped by this process, so removing it is expected
		// to fail; the next upgrade clears it.
		discard: func() { _ = os.Remove(backup) },
	}, nil
}

// restoreWindowsBinary puts the original executable back after a failed
// installer run. The failed installer may have already created a replacement,
// so remove that first before moving the backup back into place.
func restoreWindowsBinary(backup, executable string) error {
	if err := os.Remove(executable); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove the failed replacement executable: %w", err)
	}
	if err := os.Rename(backup, executable); err != nil {
		return fmt.Errorf("could not restore the original executable: %w", err)
	}
	return nil
}

// installerCommand builds the platform's installer invocation, pinned to the
// target release and to the directory the running binary occupies.
func (p UpgradePlan) installerCommand(ctx context.Context, scriptPath string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell.exe",
			"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath,
			"-Version", p.Release.Version,
			"-InstallDir", p.InstallDir,
			// The directory already holds the running binary, so it is on PATH.
			"-NoModifyPath")
	}
	return exec.CommandContext(ctx, "bash", scriptPath,
		"--version", p.Release.Version,
		"--install-dir", p.InstallDir)
}

// ManualUpgradeCommand returns the documented install command for this
// platform, for the paths where the CLI cannot upgrade itself.
func ManualUpgradeCommand() string {
	return manualUpgradeCommand(runtime.GOOS)
}

func manualUpgradeCommand(goos string) string {
	if goos == "windows" {
		return "irm " + releasesPage + "/latest/download/install.ps1 | iex"
	}
	return "curl -fsSL " + releasesPage + "/latest/download/install.sh | bash"
}

// installerName is the installer asset for the running platform.
func installerName() string {
	if runtime.GOOS == "windows" {
		return "install.ps1"
	}
	return "install.sh"
}

// downloadInstaller fetches the installer script.
//
// The script is trusted on HTTPS from github.com alone: checksums.txt covers
// the release archives, not the installers. That is the same trust the
// documented `curl … | bash` install places in it, and the script it fetches
// then verifies everything it downloads.
func downloadInstaller(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "nvfleetint")

	client := http.Client{Timeout: installerTimeout}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not download the installer: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not download the installer: unexpected status %d", response.StatusCode)
	}

	script, err := io.ReadAll(io.LimitReader(response.Body, maxInstallerBytes+1))
	if err != nil {
		return nil, fmt.Errorf("could not download the installer: %w", err)
	}
	if len(script) == 0 {
		return nil, errors.New("could not download the installer: it is empty")
	}
	if len(script) > maxInstallerBytes {
		return nil, fmt.Errorf("could not download the installer: it is larger than %d bytes", maxInstallerBytes)
	}
	return script, nil
}
