// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11sandbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var binaryOnce, namespaceOnce sync.Once
var binaryState, namespaceState availability

// RequireBinary is sufficient for constructor-only tests. It checks external
// binary/options, not whether product construction or path validation succeeds.
func RequireBinary(t testing.TB) {
	t.Helper()
	binaryOnce.Do(func() { binaryState = probeBinary() })
	report(t, binaryState, required())
}

// Require runs a separate harmless external namespace probe, once per test
// executable. Only this prerequisite check can skip; application errors after
// it succeeds must remain ordinary test failures.
func Require(t testing.TB) {
	t.Helper()
	RequireBinary(t)
	namespaceOnce.Do(func() { namespaceState = probeNamespaces() })
	report(t, namespaceState, required())
}

func probeBinary() availability {
	info, err := os.Lstat("/usr/bin/bwrap")
	if errors.Is(err, os.ErrNotExist) {
		return availability{reason: "bubblewrap_missing"}
	}
	if err != nil {
		return availability{reason: "bubblewrap_inspection_failed", broken: true}
	}
	if !binaryPermissionsAllowed(info) {
		return availability{reason: "bubblewrap_unsafe_permissions", broken: true}
	}
	if os.Geteuid() == 0 || os.Getuid() != os.Geteuid() || os.Getgid() != os.Getegid() {
		return availability{reason: "nonprivileged_user_required"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := externalCommand(ctx, "--help")
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return availability{reason: "bubblewrap_help_failed", broken: true}
	}
	return optionsAvailable(output.String())
}

func optionsAvailable(help string) availability {
	options := map[string]bool{}
	for _, word := range strings.Fields(help) {
		options[word] = true
	}
	for _, option := range strings.Fields(requiredOptions) {
		if !options[option] {
			return availability{reason: "bubblewrap_required_options_missing"}
		}
	}
	return availability{}
}

const requiredOptions = "--ro-bind-fd --bind-fd --unshare-user --unshare-ipc --unshare-pid --unshare-net --unshare-uts --disable-userns --die-with-parent --new-session --cap-drop --clearenv --setenv --chdir --proc --dev --perms --size --tmpfs"

func binaryPermissionsAllowed(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && stat.Uid == 0 && info.Mode().Perm()&0022 == 0 && info.Mode().Perm()&0111 != 0 && info.Mode()&(os.ModeSetuid|os.ModeSetgid) == 0
}

func externalCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/usr/bin/bwrap", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	cmd.Dir = "/"
	return cmd
}

// This command is deliberately independent of NewSandboxCommand: no driver,
// request, token, PIN, system configuration or personal file is used. The
// administrator's true utility just exits after bwrap performs the mounts.
func probeNamespaces() availability {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := []string{"--unshare-user", "--unshare-ipc", "--unshare-pid", "--unshare-net", "--unshare-uts", "--disable-userns", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--setenv", "PATH", "/usr/bin:/bin", "--setenv", "LANG", "C.UTF-8", "--chdir", "/"}
	var files []*os.File
	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()
	for _, path := range []string{"/usr", "/lib", "/lib64", "/usr/bin/true"} {
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) && (path == "/lib" || path == "/lib64") {
			continue
		}
		if err != nil {
			return availability{reason: "probe_system_files_unavailable"}
		}
		destination := path
		if path == "/usr/bin/true" {
			destination = "/sandbox-probe/true"
		}
		args = append(args, "--ro-bind-fd", strconv.Itoa(3+len(files)), destination)
		files = append(files, file)
	}
	args = append(args, "--proc", "/proc", "--dev", "/dev", "--perms", "0700", "--size", "16777216", "--tmpfs", "/tmp", "--", "/sandbox-probe/true")
	cmd := externalCommand(ctx, args...)
	cmd.ExtraFiles = files
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return availability{reason: "namespace_probe_unavailable"}
	}
	// No compiler, HOME resolution, module metadata or runtime test is folded
	// into this result. Each is tested separately and retains its own failure.
	return availability{}
}

// Do not retain unbounded external diagnostics or forward host-specific text.
type boundedOutput struct{ bytes.Buffer }

func (output *boundedOutput) Write(data []byte) (int, error) {
	n := len(data)
	if remaining := 65536 - output.Len(); remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = output.Buffer.Write(data)
	}
	return n, nil
}
