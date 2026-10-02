// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// HardenProcess must be called only by the auxiliary executable, never by a
// caller's process or a library constructor. It restricts exec privilege gain
// on every thread and disables ordinary core/ptrace dumps. It does not confine
// filesystem/network access, stop a privileged debugger or sandbox a driver.
// Any error requires terminating the auxiliary before loading native code.
func HardenProcess() error {
	if os.Geteuid() == 0 || os.Getuid() != os.Geteuid() || os.Getgid() != os.Getegid() {
		return errors.New("auxiliar PKCS#11 no admite privilegios heredados")
	}
	var capabilities [2]unix.CapUserData
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	if err := unix.Capget(&header, &capabilities[0]); err != nil ||
		capabilities[0].Effective != 0 || capabilities[1].Effective != 0 ||
		capabilities[0].Permitted != 0 || capabilities[1].Permitted != 0 {
		return errors.New("auxiliar PKCS#11 no admite capacidades privilegiadas")
	}
	// PR_SET_NO_NEW_PRIVS is thread-local. Keep both calls on the same thread
	// and synchronize the bit using a one-instruction ALLOW seccomp filter.
	// seccomp_sync_threads propagates task_no_new_privs to every existing thread:
	// https://github.com/torvalds/linux/blob/master/kernel/seccomp.c
	// Future threads inherit it. This adds NO syscall access restrictions.
	// Go's AllThreadsSyscall cannot provide this guarantee in a CGo worker.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return errors.New("no se pudo limitar privilegios del auxiliar")
	}
	filter := unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW}
	program := unix.SockFprog{Len: 1, Filter: &filter}
	result, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER,
		unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(program)
	runtime.KeepAlive(filter)
	// TSYNC can report the incompatible thread's positive TID, with errno=0.
	if errno != 0 || result != 0 {
		return errors.New("no se pudieron limitar todos los hilos del auxiliar")
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return errors.New("no se pudo proteger memoria del auxiliar")
	}
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{}); err != nil {
		return errors.New("no se pudo desactivar volcado del auxiliar")
	}
	return nil
}

// ValidateModulePath accepts an explicitly configured regular module owned by
// root or the effective user. Every resolved ancestor must also be controlled;
// root-owned sticky directories such as /tmp may hold a private test directory.
// Native modules remain an administrator trust decision, not untrusted plugins.
// This is a filesystem snapshot: it does not stop a trusted owner replacing
// its own module between validation and load. Always load the returned path.
func ValidateModulePath(path string) (string, error) {
	invalid := errors.New("ruta administrada del modulo PKCS#11 no valida")
	if !filepath.IsAbs(path) || len(path) > 4096 {
		return "", invalid
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", invalid
	}
	current := resolved
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return "", invalid
		}
		if !moduleEntryAllowed(info, uint32(os.Geteuid()), current == resolved) { // #nosec G115 -- Linux euid is a kernel uid_t (uint32).
			return "", invalid
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return resolved, nil
}

func moduleEntryAllowed(info os.FileInfo, user uint32, leaf bool) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (stat.Uid != 0 && stat.Uid != user) {
		return false
	}
	if leaf {
		return info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= 128*1024*1024 && info.Mode().Perm()&0022 == 0
	}
	return info.IsDir() && (info.Mode().Perm()&0022 == 0 || (stat.Uid == 0 && info.Mode()&os.ModeSticky != 0))
}

// PrivateProtocolOutput preserves the anonymous output pipe and suppresses
// third-party printf/logging on stdout/stderr. No log file captures a PIN.
func PrivateProtocolOutput() (*os.File, error) {
	// A file, terminal, socket or named FIFO is not the parent's anonymous pipe.
	var stat unix.Stat_t
	target, readErr := os.Readlink("/proc/self/fd/1")
	flags, flagErr := unix.FcntlInt(uintptr(unix.Stdout), unix.F_GETFL, 0) // #nosec G115 -- Stdout is fixed descriptor 1.
	if unix.Fstat(unix.Stdout, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFIFO ||
		readErr != nil || !strings.HasPrefix(target, "pipe:[") || flagErr != nil || flags&unix.O_ACCMODE != unix.O_WRONLY {
		return nil, errors.New("el auxiliar requiere un pipe anonimo de salida")
	}
	fd, err := unix.FcntlInt(uintptr(unix.Stdout), unix.F_DUPFD_CLOEXEC, 3) // #nosec G115 -- Stdout is fixed descriptor 1.
	if err != nil {
		return nil, errors.New("no se pudo preparar canal privado")
	}
	output := os.NewFile(uintptr(fd), "pkcs11-private-output") // #nosec G115 -- F_DUPFD_CLOEXEC returns a non-negative descriptor on success.
	null, err := unix.Open("/dev/null", unix.O_WRONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = output.Close()
		return nil, errors.New("no se pudo proteger salida del auxiliar")
	}
	// If stderr was closed, Open may allocate descriptor 2. Keep that /dev/null
	// descriptor open after Dup2; closing it would leave driver stderr reusable.
	if null != unix.Stdout && null != unix.Stderr {
		defer unix.Close(null)
	}
	for _, target := range []int{unix.Stdout, unix.Stderr} {
		var redirectErr error
		if null == target {
			// x/sys Dup2 uses dup3 on Linux, which rejects oldfd == newfd.
			_, redirectErr = unix.FcntlInt(uintptr(null), unix.F_SETFD, 0) // #nosec G115 -- unix.Open returned a non-negative descriptor.
		} else {
			redirectErr = unix.Dup2(null, target)
		}
		if redirectErr != nil {
			_ = output.Close()
			return nil, errors.New("no se pudo proteger salida del auxiliar")
		}
	}
	return output, nil
}
