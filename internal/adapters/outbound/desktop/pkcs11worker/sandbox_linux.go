// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

var (
	ErrSandboxUnavailable = errors.New("aislamiento del auxiliar no disponible")
	ErrSandboxPolicy      = errors.New("recursos de aislamiento del auxiliar no validos")
)

// SandboxResources is a local, trusted policy, never a worker request, URI or
// environment variable. Additional directories are explicit private subtrees;
// granting one grants its entire contents, including subsequent owner changes.
type SandboxResources struct {
	ReadOnlyFiles []string
	ReadOnlyDirs  []string
	ReadWriteDirs []string
}

// SandboxLaunch owns the descriptors pinned during construction. Keep it alive
// until Command.Start (or Run) returns, then Close it, including on errors.
// Command is a one-shot local command; callers must not change its policy args,
// environment or ExtraFiles. Stdin/stdout are the existing private protocol.
type SandboxLaunch struct {
	Command    *exec.Cmd
	Executable string
	Module     string
	mu         sync.Mutex
	files      []*os.File
}

// Close releases only the parent's pinned descriptors, never a running worker.
// It is idempotent. It must not race Command.Start, which borrows those FDs.
func (launch *SandboxLaunch) Close() error {
	if launch == nil {
		return nil
	}
	launch.mu.Lock()
	defer launch.mu.Unlock()
	var failed bool
	for _, file := range launch.files {
		if err := file.Close(); err != nil {
			failed = true
		}
	}
	launch.files = nil
	if failed {
		return ErrSandboxUnavailable
	}
	return nil
}

const sandboxMaxFile = 128 * 1024 * 1024

// NewSandboxCommand constructs, but does not execute, a mandatory namespace
// launcher. Unsupported bwrap options or disabled user namespaces fail at Start/
// Wait; there is deliberately no unconfined fallback. This is not a complete
// sandbox: the worker must also apply its all-thread syscall restrictions before
// loading any module, and the mounted PC/SC daemon remains a trusted service.
func NewSandboxCommand(ctx context.Context, executable, module string, resources SandboxResources) (*SandboxLaunch, error) {
	return newSandboxCommand(ctx, executable, module, resources, "/usr/bin/bwrap")
}

func newSandboxCommand(ctx context.Context, executable, module string, resources SandboxResources, launcherPath string) (_ *SandboxLaunch, resultErr error) {
	if ctx == nil {
		return nil, ErrSandboxPolicy
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if os.Geteuid() == 0 || os.Getuid() != os.Geteuid() || os.Getgid() != os.Getegid() {
		return nil, ErrSandboxUnavailable
	}
	if len(resources.ReadOnlyFiles)+len(resources.ReadOnlyDirs)+len(resources.ReadWriteDirs) > 16 {
		return nil, ErrSandboxPolicy
	}
	launch := &SandboxLaunch{}
	defer func() {
		if resultErr != nil {
			_ = launch.Close()
		}
	}()
	launcher, err := sandboxResolvedOpenOwned(launcherPath, true)
	if err != nil {
		return nil, ErrSandboxUnavailable
	}
	// bwrap itself is an administrator-installed executable, not user policy.
	launcherStat, err := sandboxStat(launcher)
	_ = launcher.Close()
	if err != nil || !sandboxRegular(launcherStat) || launcherStat.Uid != 0 || launcherStat.Mode&06000 != 0 || launcherStat.Mode&0111 == 0 {
		return nil, ErrSandboxUnavailable
	}
	args := []string{"--unshare-user", "--unshare-ipc", "--unshare-pid", "--unshare-net", "--unshare-uts", "--disable-userns", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--setenv", "PATH", "/usr/bin:/bin", "--setenv", "LANG", "C.UTF-8", "--chdir", "/"}
	add := func(file *os.File, destination string, writable bool) {
		// bwrap 0.11.1 compares dev+ino after resolving/mounting the FD, then
		// consumes it. A plain --ro-bind /proc/self/fd/N lacks that check and
		// leaks the original FD into the payload. Never substitute it here.
		// https://github.com/containers/bubblewrap/blob/v0.11.1/bubblewrap.c
		flag := "--ro-bind-fd"
		if writable {
			flag = "--bind-fd"
		}
		args = append(args, flag, strconv.Itoa(3+len(launch.files)), destination)
		launch.files = append(launch.files, file)
	}
	// Preserve the distribution's loader layout without exposing /etc, /var,
	// /run or home trees. /usr is intentionally broad, read-only system data.
	for _, path := range []string{"/usr", "/lib", "/lib64"} {
		file, openErr := sandboxResolvedOpenOwned(path, true)
		if errors.Is(openErr, os.ErrNotExist) && path != "/usr" {
			continue
		}
		if openErr != nil {
			return nil, ErrSandboxUnavailable
		}
		stat, statErr := sandboxStat(file)
		resolved := file.Name()
		if statErr != nil || stat.Uid != 0 || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0022 != 0 ||
			(resolved != path && resolved != "/usr"+path) {
			_ = file.Close()
			return nil, ErrSandboxUnavailable
		}
		add(file, path, false)
	}
	args = append(args, "--proc", "/proc", "--dev", "/dev", "--perms", "0700", "--size", "16777216", "--tmpfs", "/tmp")
	for i, path := range []string{executable, module} {
		file, openErr := sandboxResolvedOpen(path)
		if openErr != nil {
			return nil, ErrSandboxPolicy
		}
		stat, statErr := sandboxStat(file)
		if statErr != nil || !sandboxRegular(stat) || stat.Size == 0 || stat.Mode&06000 != 0 || (i == 0 && stat.Mode&0111 == 0) || sandboxReservedDestination(file.Name()) {
			_ = file.Close()
			return nil, ErrSandboxPolicy
		}
		if i == 0 {
			launch.Executable = file.Name()
		} else {
			launch.Module = file.Name()
		}
		if i == 1 && launch.Executable == launch.Module {
			_ = file.Close()
			return nil, ErrSandboxPolicy
		}
		add(file, file.Name(), false)
	}
	for _, path := range []string{"/etc/ld.so.cache", "/etc/localtime", "/etc/opensc.conf", "/etc/opensc/opensc.conf"} {
		file, openErr := sandboxResolvedOpenOwned(path, true)
		if errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if openErr != nil {
			return nil, ErrSandboxPolicy
		}
		stat, statErr := sandboxStat(file)
		if statErr != nil || !sandboxRegular(stat) || stat.Uid != 0 || (!sandboxBeneath(file.Name(), "/etc") && !sandboxBeneath(file.Name(), "/usr")) {
			_ = file.Close()
			return nil, ErrSandboxPolicy
		}
		add(file, path, false)
	}
	// A pathname socket still works across network namespaces. Expose just the
	// pinned root-owned socket inode, not pcscd's directory. Daemon replacement
	// may invalidate this one operation; never reconnect/retry a PIN implicitly.
	const pcsc = "/run/pcscd/pcscd.comm"
	if file, openErr := sandboxOpenPathOwned(pcsc, true); openErr == nil {
		stat, statErr := sandboxStat(file)
		if statErr != nil || stat.Uid != 0 || stat.Mode&unix.S_IFMT != unix.S_IFSOCK {
			_ = file.Close()
			return nil, ErrSandboxPolicy
		}
		add(file, pcsc, false)
	} else if !errors.Is(openErr, os.ErrNotExist) {
		return nil, ErrSandboxPolicy
	}
	paths := []string{launch.Executable, launch.Module}
	groups := []struct {
		paths               []string
		directory, writable bool
	}{{resources.ReadOnlyFiles, false, false}, {resources.ReadOnlyDirs, true, false}, {resources.ReadWriteDirs, true, true}}
	home := ""
	if len(resources.ReadOnlyDirs)+len(resources.ReadWriteDirs) > 0 {
		home, err = sandboxLocalHome()
		if err != nil {
			return nil, ErrSandboxPolicy
		}
	}
	for _, group := range groups {
		for _, path := range group.paths {
			if !sandboxValidPath(path) || sandboxReservedResource(path) || (group.directory && (path == home || sandboxBeneath(home, path) || filepath.Dir(path) == "/home")) {
				return nil, ErrSandboxPolicy
			}
			for _, previous := range paths {
				if path == previous || sandboxBeneath(path, previous) || sandboxBeneath(previous, path) {
					return nil, ErrSandboxPolicy
				}
			}
			file, openErr := sandboxOpenPath(path)
			if openErr != nil {
				return nil, ErrSandboxPolicy
			}
			stat, statErr := sandboxStat(file)
			valid := statErr == nil && sandboxRegular(stat)
			if group.directory {
				valid = statErr == nil && stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Uid == uint32(os.Geteuid()) && stat.Mode&07777 == 0700 // #nosec G115 -- Linux euid is a kernel uid_t (uint32).
			}
			if !valid {
				_ = file.Close()
				return nil, ErrSandboxPolicy
			}
			paths = append(paths, path)
			add(file, path, group.writable)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	args = append(args, "--", launch.Executable)
	launch.Command = exec.CommandContext(ctx, launcherPath, args...)
	launch.Command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	launch.Command.Dir = "/"
	launch.Command.ExtraFiles = append([]*os.File(nil), launch.files...)
	return launch, nil
}

func sandboxValidPath(path string) bool {
	if path == "" || len(path) > 4096 || !utf8.ValidString(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for _, c := range path {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

func sandboxBeneath(path, parent string) bool { return strings.HasPrefix(path, parent+"/") }

func sandboxReservedDestination(path string) bool {
	for _, root := range []string{"/proc", "/dev", "/sys", "/run"} {
		if path == root || sandboxBeneath(path, root) {
			return true
		}
	}
	return path == "/"
}

func sandboxReservedResource(path string) bool {
	if sandboxReservedDestination(path) {
		return true
	}
	for _, root := range []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/boot"} {
		if path == root || sandboxBeneath(path, root) {
			return true
		}
	}
	for _, root := range []string{"/etc", "/var", "/var/lib", "/var/cache", "/var/log", "/var/tmp", "/tmp", "/home", "/root", "/mnt", "/media", "/srv", "/opt"} {
		if path == root {
			return true
		}
	}
	// Never replace fixed configuration mounts through the resource contract.
	for _, fixed := range []string{"/etc/ld.so.cache", "/etc/localtime", "/etc/opensc.conf", "/etc/opensc/opensc.conf"} {
		if path == fixed || sandboxBeneath(fixed, path) {
			return true
		}
	}
	return false
}

func sandboxStat(file *os.File) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstat(int(file.Fd()), &stat) // #nosec G115 -- os.File.Fd is a kernel int descriptor on Linux.
	return stat, err
}

func sandboxRegular(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Size >= 0 && stat.Size <= sandboxMaxFile && stat.Nlink == 1 && stat.Mode&0022 == 0
}

// Only system defaults and executable/module paths may use controlled symlinks.
// The resolved path is walked again by descriptor; no untrusted symlink is
// followed by openat, and bwrap binds exactly the subsequently pinned inode.
// Pinning does not prevent a trusted host owner editing that inode's contents.
func sandboxResolvedOpen(path string) (*os.File, error) {
	return sandboxResolvedOpenOwned(path, false)
}

func sandboxResolvedOpenOwned(path string, rootOnly bool) (*os.File, error) {
	if !sandboxValidPath(path) {
		return nil, ErrSandboxPolicy
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	return sandboxOpenPathOwned(resolved, rootOnly)
}

// The leaf is O_PATH so opening a FIFO, device or socket never performs I/O.
// Validate metadata before handing any descriptor to the launcher.
func sandboxOpenPath(path string) (*os.File, error) {
	return sandboxOpenPathOwned(path, false)
}

func sandboxOpenPathOwned(path string, rootOnly bool) (*os.File, error) {
	if !sandboxValidPath(path) || path == "/" {
		return nil, ErrSandboxPolicy
	}
	fd, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil || !sandboxAncestor(stat) || (rootOnly && (stat.Uid != 0 || stat.Mode&0022 != 0)) {
			_ = unix.Close(fd)
			return nil, ErrSandboxPolicy
		}
		flags := unix.O_PATH | unix.O_NOFOLLOW | unix.O_CLOEXEC
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, err := unix.Openat(fd, part, flags, 0)
		_ = unix.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT == unix.S_IFLNK || (stat.Uid != 0 && (rootOnly || stat.Uid != uint32(os.Geteuid()))) { // #nosec G115 -- Linux euid is a kernel uid_t (uint32).
		_ = unix.Close(fd)
		return nil, ErrSandboxPolicy
	}
	return os.NewFile(uintptr(fd), path), nil // #nosec G115 -- unix.Openat returned a non-negative descriptor.
}

func sandboxAncestor(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && (stat.Uid == 0 || stat.Uid == uint32(os.Geteuid())) && (stat.Mode&0022 == 0 || (stat.Uid == 0 && stat.Mode&unix.S_ISVTX != 0)) // #nosec G115 -- Linux euid is a kernel uid_t (uint32).
}

// Do not take HOME from the caller's environment or load NSS plugins into the
// main process. Private directory grants require a bounded local passwd entry;
// users supplied exclusively by NSS need a future explicit local home policy.
func sandboxLocalHome() (string, error) {
	path, err := sandboxOpenPathOwned("/etc/passwd", true)
	if err != nil {
		return "", ErrSandboxPolicy
	}
	defer path.Close()
	stat, err := sandboxStat(path)
	if err != nil || !sandboxRegular(stat) || stat.Uid != 0 || stat.Size > 4*1024*1024 {
		return "", ErrSandboxPolicy
	}
	file, err := os.Open("/proc/self/fd/" + strconv.Itoa(int(path.Fd()))) // #nosec G115 -- os.File.Fd is a kernel int descriptor on Linux.
	if err != nil {
		return "", ErrSandboxPolicy
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return "", ErrSandboxPolicy
	}
	id := strconv.Itoa(os.Geteuid())
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) == 7 && fields[2] == id && sandboxValidPath(fields[5]) && fields[5] != "/" {
			return fields[5], nil
		}
	}
	return "", ErrSandboxPolicy
}
