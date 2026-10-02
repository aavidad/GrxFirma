// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && (amd64 || arm64)

package pkcs11worker

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// RestrictDriverSyscalls installs a thread-synchronized, irreversible filter.
// Call only in the worker, after HardenProcess and before any native loading.
// AF_UNIX remains available for PC/SC: this is NOT a filesystem or Unix-path
// sandbox. Existing descriptors retain their authority; the launcher must
// restrict mounts, namespaces and inherited descriptors independently.
func RestrictDriverSyscalls() error {
	pid := os.Getpid()
	if pid < 0 || uint64(pid) > uint64(^uint32(0)) {
		return errors.New("identificador de proceso fuera de rango")
	}
	filter := driverSyscallFilter(uint32(pid)) // #nosec G115 -- pid was checked against uint32 above.
	if len(filter) == 0 || len(filter) > int(^uint16(0)) {
		return errors.New("filtro de llamadas fuera de rango")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return errors.New("no se pudo restringir privilegios del controlador")
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]} // #nosec G115 -- filter length was checked against uint16 above.
	result, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER,
		unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(program)
	runtime.KeepAlive(filter)
	if errno != 0 || result != 0 {
		return errors.New("no se pudo restringir todos los hilos del controlador")
	}
	return nil
}

func driverSyscallFilter(pid uint32) []unix.SockFilter {
	load := func(offset uint32) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: offset}
	}
	ret := func(value uint32) unix.SockFilter { return unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: value} }
	jeq := func(value uint32, yes, no uint8) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: value, Jt: yes, Jf: no}
	}
	deny := ret(unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM))
	allow := ret(unix.SECCOMP_RET_ALLOW)
	f := []unix.SockFilter{load(4), jeq(driverAuditArch, 1, 0), ret(unix.SECCOMP_RET_KILL_PROCESS), load(0)}
	// Reject the x32 ABI, including historical x32 numbers, before matching syscalls.
	if driverAuditArch == unix.AUDIT_ARCH_X86_64 {
		f = append(f, unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: 0x40000000, Jf: 1}, ret(unix.SECCOMP_RET_KILL_PROCESS))
		f = append(f, unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JGE | unix.BPF_K, K: 512, Jf: 2}, unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JGE | unix.BPF_K, K: 548, Jt: 1}, deny)
	}
	// clone3 flags are behind a pointer, which classic BPF cannot inspect.
	// ENOSYS lets libc pthread_create use the inspectable legacy clone ABI.
	f = append(f, jeq(unix.SYS_CLONE3, 0, 1), ret(unix.SECCOMP_RET_ERRNO|uint32(unix.ENOSYS)))
	denied := []uint32{unix.SYS_EXECVE, unix.SYS_EXECVEAT, unix.SYS_SETNS, unix.SYS_UNSHARE,
		unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_PIVOT_ROOT, unix.SYS_CHROOT,
		unix.SYS_FSOPEN, unix.SYS_FSCONFIG, unix.SYS_FSMOUNT, unix.SYS_MOVE_MOUNT, unix.SYS_OPEN_TREE, unix.SYS_MOUNT_SETATTR,
		unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV,
		unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN, unix.SYS_KEYCTL, unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY,
		unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER,
		unix.SYS_KEXEC_LOAD, unix.SYS_KEXEC_FILE_LOAD, unix.SYS_REBOOT,
		unix.SYS_KILL, unix.SYS_TKILL, unix.SYS_RT_SIGQUEUEINFO, unix.SYS_RT_TGSIGQUEUEINFO, unix.SYS_PIDFD_SEND_SIGNAL,
		unix.SYS_PIDFD_GETFD, unix.SYS_SETSID, unix.SYS_SETPGID}
	denied = append(denied, driverForkSyscalls...)
	for _, nr := range denied {
		f = append(f, jeq(nr, 0, 1), deny)
	}
	// Legacy clone must create a thread, with no upper argument bits or namespaces.
	f = append(f, jeq(unix.SYS_CLONE, 0, 9), load(20), jeq(0, 1, 0), deny, load(16),
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: unix.CLONE_THREAD, Jt: 1}, deny,
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: unix.CLONE_NEWCGROUP | unix.CLONE_NEWIPC | unix.CLONE_NEWNET | unix.CLONE_NEWNS | unix.CLONE_NEWPID | unix.CLONE_NEWUSER | unix.CLONE_NEWUTS, Jt: 1}, allow, deny)
	for _, nr := range []uint32{unix.SYS_SOCKET, unix.SYS_SOCKETPAIR} {
		f = append(f, load(0), jeq(nr, 0, 7), load(20), jeq(0, 1, 0), deny, load(16), jeq(unix.AF_UNIX, 0, 1), allow, deny)
	}
	// Go preemption uses tgkill(getpid(), tid, SIGURG). Never allow another TGID.
	f = append(f, load(0), jeq(unix.SYS_TGKILL, 0, 7), load(20), jeq(0, 1, 0), deny, load(16), jeq(pid, 0, 1), allow, deny, allow)
	return f
}
