// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && (amd64 || arm64)

package pkcs11worker

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/testsupport/pkcs11sandbox"

	"golang.org/x/sys/unix"
)

// Interpreter covers every instruction emitted by the policy and rejects all others.
func evaluateDriverFilter(t *testing.T, nr, arch uint32, arg uint64) uint32 {
	t.Helper()
	var data [64]byte
	binary.LittleEndian.PutUint32(data[0:], nr)
	binary.LittleEndian.PutUint32(data[4:], arch)
	binary.LittleEndian.PutUint64(data[16:], arg)
	var a uint32
	f := driverSyscallFilter(1234)
	for pc := 0; pc < len(f); pc++ {
		i := f[pc]
		switch i.Code {
		case unix.BPF_LD | unix.BPF_W | unix.BPF_ABS:
			a = binary.LittleEndian.Uint32(data[i.K:])
		case unix.BPF_RET | unix.BPF_K:
			return i.K
		case unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, unix.BPF_JMP | unix.BPF_JGE | unix.BPF_K, unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K:
			match := a == i.K
			if i.Code == unix.BPF_JMP|unix.BPF_JGE|unix.BPF_K {
				match = a >= i.K
			}
			if i.Code == unix.BPF_JMP|unix.BPF_JSET|unix.BPF_K {
				match = a&i.K != 0
			}
			if match {
				pc += int(i.Jt)
			} else {
				pc += int(i.Jf)
			}
		default:
			t.Fatalf("unknown instruction: %+v", i)
		}
	}
	t.Fatal("filter fell through")
	return 0
}

func TestDriverSeccompPolicyArgumentsAndArchitecture(t *testing.T) {
	deny := uint32(unix.SECCOMP_RET_ERRNO | unix.EPERM)
	allow := uint32(unix.SECCOMP_RET_ALLOW)
	for _, nr := range []uint32{unix.SYS_SOCKET, unix.SYS_SOCKETPAIR} {
		for _, arg := range []uint64{unix.AF_INET, unix.AF_INET6, unix.AF_NETLINK, uint64(unix.AF_UNIX) | 1<<32} {
			if got := evaluateDriverFilter(t, nr, driverAuditArch, arg); got != deny {
				t.Fatalf("socket %d arg%x=%x", nr, arg, got)
			}
		}
		if got := evaluateDriverFilter(t, nr, driverAuditArch, unix.AF_UNIX); got != allow {
			t.Fatal(got)
		}
	}
	for _, arg := range []uint64{0, uint64(unix.SIGCHLD), unix.CLONE_THREAD | unix.CLONE_NEWUSER, uint64(unix.CLONE_THREAD) | 1<<32} {
		if got := evaluateDriverFilter(t, unix.SYS_CLONE, driverAuditArch, arg); got != deny {
			t.Fatalf("clone%x=%x", arg, got)
		}
	}
	if evaluateDriverFilter(t, unix.SYS_CLONE, driverAuditArch, unix.CLONE_THREAD|unix.CLONE_VM|unix.CLONE_SIGHAND) != allow {
		t.Fatal("thread denied")
	}
	if evaluateDriverFilter(t, unix.SYS_CLONE3, driverAuditArch, 0) != uint32(unix.SECCOMP_RET_ERRNO|unix.ENOSYS) {
		t.Fatal("clone3 fallback")
	}
	for _, arg := range []uint64{1235, uint64(1234) | 1<<32, 0} {
		if evaluateDriverFilter(t, unix.SYS_TGKILL, driverAuditArch, arg) != deny {
			t.Fatal("external signal allowed")
		}
	}
	if evaluateDriverFilter(t, unix.SYS_TGKILL, driverAuditArch, 1234) != allow {
		t.Fatal("preemption denied")
	}
	for _, nr := range []uint32{unix.SYS_READ, unix.SYS_WRITE, unix.SYS_MMAP, unix.SYS_MLOCK, unix.SYS_FUTEX, unix.SYS_CLOCK_GETTIME} {
		if evaluateDriverFilter(t, nr, driverAuditArch, 0) != allow {
			t.Fatalf("runtime syscall %d denied", nr)
		}
	}
	for _, nr := range []uint32{unix.SYS_EXECVE, unix.SYS_EXECVEAT, unix.SYS_UNSHARE, unix.SYS_SETNS, unix.SYS_MOUNT, unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER, unix.SYS_KILL, unix.SYS_TKILL, unix.SYS_BPF, unix.SYS_KEYCTL, unix.SYS_PERF_EVENT_OPEN, unix.SYS_REBOOT} {
		if evaluateDriverFilter(t, nr, driverAuditArch, 0) != deny {
			t.Fatalf("dangerous syscall %d allowed", nr)
		}
	}
	if evaluateDriverFilter(t, unix.SYS_READ, 0, 0) != unix.SECCOMP_RET_KILL_PROCESS {
		t.Fatal("foreign architecture")
	}
	if driverAuditArch == unix.AUDIT_ARCH_X86_64 {
		if evaluateDriverFilter(t, 0x40000000|unix.SYS_READ, driverAuditArch, 0) != unix.SECCOMP_RET_KILL_PROCESS {
			t.Fatal("x32 bypass")
		}
		for nr := uint32(512); nr < 548; nr++ {
			if evaluateDriverFilter(t, nr, driverAuditArch, 0) != deny {
				t.Fatal("old x32 bypass")
			}
		}
	}
}

func TestDriverSeccompRealSubprocess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	before, _ := unix.PrctlRetInt(unix.PR_GET_SECCOMP, 0, 0, 0, 0)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDriverSeccompChild$")
	cmd.Env = append(os.Environ(), "GRXFIRMA_TEST_DRIVER_SECCOMP=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child %v: %s", err, out)
	}
	after, _ := unix.PrctlRetInt(unix.PR_GET_SECCOMP, 0, 0, 0, 0)
	if before != after {
		t.Fatal("parent policy changed")
	}
}

func TestDriverSeccompChild(t *testing.T) {
	if os.Getenv("GRXFIRMA_TEST_DRIVER_SECCOMP") != "1" {
		return
	}
	// Threads already alive at TSYNC must obey the same policy as new threads.
	ready := make(chan struct{}, 4)
	release := make(chan struct{})
	results := make(chan error, 8)
	var group sync.WaitGroup
	probe := func(wait bool) {
		defer group.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if wait {
			ready <- struct{}{}
			<-release
		}
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
		if fd >= 0 {
			unix.Close(fd)
		}
		results <- err
	}
	for i := 0; i < 4; i++ {
		group.Add(1)
		go probe(true)
	}
	for i := 0; i < 4; i++ {
		<-ready
	}
	if err := RestrictDriverSyscalls(); err != nil {
		t.Fatal(err)
	}
	close(release)
	for i := 0; i < 4; i++ {
		group.Add(1)
		go probe(false)
	}
	group.Wait()
	for i := 0; i < 8; i++ {
		if err := <-results; err != unix.EPERM {
			t.Fatalf("thread socket: %v", err)
		}
	}
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	if _, err := unix.Write(fds[0], []byte{0, 255, 1}); err != nil {
		t.Fatal(err)
	}
	var output [3]byte
	if n, err := unix.Read(fds[1], output[:]); err != nil || n != 3 || output != [3]byte{0, 255, 1} {
		t.Fatal("unix channel")
	}
	for _, nr := range []uintptr{unix.SYS_EXECVE, unix.SYS_EXECVEAT, unix.SYS_UNSHARE, unix.SYS_PTRACE, unix.SYS_KILL} {
		_, _, errno := unix.RawSyscall(nr, 0, 0, 0)
		if errno != unix.EPERM {
			t.Fatalf("syscall%d: %v", nr, errno)
		}
	}
	_, _, errno := unix.RawSyscall(unix.SYS_CLONE3, 0, 0, 0)
	if errno != unix.ENOSYS {
		t.Fatal(errno)
	}
	_, _, errno = unix.RawSyscall(unix.SYS_CLONE, uintptr(unix.SIGCHLD), 0, 0)
	if errno != unix.EPERM {
		t.Fatal(errno)
	}
	if err := unix.Tgkill(os.Getpid(), unix.Gettid(), 0); err != nil {
		t.Fatal(err)
	}
	if err := unix.Tgkill(os.Getppid(), unix.Gettid(), 0); err != unix.EPERM {
		t.Fatal(err)
	}
	// Exercise scheduler, timers, GC and memory mappings after restrictions.
	runtime.GC()
	time.Sleep(time.Millisecond)
}

func TestDriverSeccompPthreadFallback(t *testing.T) {
	compiler := pkcs11sandbox.RequireCompiler(t)
	var instructions strings.Builder
	for _, i := range driverSyscallFilter(1234) {
		if i.K == 1234 && i.Code == unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K {
			fmt.Fprintf(&instructions, "{%d,%d,%d,(unsigned)getpid()},\n", i.Code, i.Jt, i.Jf)
		} else {
			fmt.Fprintf(&instructions, "{%d,%d,%d,%du},\n", i.Code, i.Jt, i.Jf, i.K)
		}
	}
	source := `// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
#define _GNU_SOURCE
#include <errno.h>
#include <pthread.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <sys/socket.h>
#include <unistd.h>
static pthread_barrier_t ready, release;
static void *probe(void *arg) {
  if (arg) { pthread_barrier_wait(&ready); pthread_barrier_wait(&release); }
  int fd = socket(AF_INET, SOCK_STREAM, 0);
  if (fd >= 0) { close(fd); return (void*)1; }
  return errno == EPERM ? 0 : (void*)2;
}
int main(void) {
  pthread_t before[4], after[4];
  pthread_barrier_init(&ready, 0, 5); pthread_barrier_init(&release, 0, 5);
  for(int i=0;i<4;i++) if(pthread_create(&before[i],0,probe,(void*)1)) return 10;
  pthread_barrier_wait(&ready);
  struct sock_filter filter[] = {` + instructions.String() + `};
  struct sock_fprog program = {sizeof(filter)/sizeof(filter[0]), filter};
  if(prctl(PR_SET_NO_NEW_PRIVS,1,0,0,0)) return 11;
  if(syscall(SYS_seccomp,SECCOMP_SET_MODE_FILTER,SECCOMP_FILTER_FLAG_TSYNC,&program)) return 12;
  pthread_barrier_wait(&release);
  for(int i=0;i<4;i++) if(pthread_create(&after[i],0,probe,0)) return 13;
  for(int i=0;i<4;i++) {
    void *result=0;
    if(pthread_join(before[i],&result)||result) return 14;
    if(pthread_join(after[i],&result)||result) return 15;
  }
  return 0;
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "pthread-probe.c")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(dir, "pthread-probe")
	if out, err := exec.CommandContext(ctx, compiler, "-pthread", "-Wall", "-Wextra", "-o", binary, path).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v %s", err, out)
	}
	if out, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("pthread probe: %v %s", err, out)
	}
}
