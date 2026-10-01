// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/testsupport/pkcs11sandbox"

	"golang.org/x/sys/unix"
)

func sandboxPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sandboxWrite(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("synthetic sandbox data"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSandboxPlanAndDescriptorOwnership(t *testing.T) {
	pkcs11sandbox.RequireBinary(t)
	client := fixtureClientFiles(t, "env")
	roFile := sandboxWrite(t, sandboxPrivateDir(t), "config")
	roDir, rwDir := sandboxPrivateDir(t), sandboxPrivateDir(t)
	launch, err := NewSandboxCommand(context.Background(), client.Executable, client.ModulePath, SandboxResources{ReadOnlyFiles: []string{roFile}, ReadOnlyDirs: []string{roDir}, ReadWriteDirs: []string{rwDir}})
	if err != nil {
		t.Fatal(err)
	}
	defer launch.Close()
	if launch.Command.Process != nil || launch.Command.Path != "/usr/bin/bwrap" || launch.Command.Dir != "/" {
		t.Fatal("constructor executed or changed launcher")
	}
	if strings.Join(launch.Command.Env, "|") != "PATH=/usr/bin:/bin|LANG=C.UTF-8" {
		t.Fatal("environment inherited")
	}
	args := launch.Command.Args
	for _, required := range []string{"--unshare-user", "--unshare-ipc", "--unshare-pid", "--unshare-net", "--unshare-uts", "--disable-userns", "--die-with-parent", "--new-session"} {
		if !sandboxContains(args, required) {
			t.Fatalf("missing %s", required)
		}
	}
	for _, forbidden := range []string{"--bind", "--ro-bind", "--dev-bind", "--share-net", "--unshare-user-try", "--as-pid-1", "--sync-fd", "--lock-file"} {
		if sandboxContains(args, forbidden) {
			t.Fatalf("unexpected %s", forbidden)
		}
	}
	used := map[int]int{}
	for i, arg := range args {
		if arg != "--ro-bind-fd" && arg != "--bind-fd" {
			continue
		}
		fd, err := strconv.Atoi(args[i+1])
		if err != nil {
			t.Fatal(err)
		}
		used[fd]++
		if arg == "--bind-fd" && args[i+2] != rwDir {
			t.Fatal("unplanned writable mount")
		}
	}
	var fds []int
	for i, file := range launch.Command.ExtraFiles {
		if used[3+i] != 1 {
			t.Fatal("descriptor not consumed exactly once")
		}
		fds = append(fds, int(file.Fd()))
	}
	if len(fds) != len(used) {
		t.Fatal("unaccounted bind descriptor")
	}
	if err := launch.Close(); err != nil {
		t.Fatal(err)
	}
	if err := launch.Close(); err != nil {
		t.Fatal(err)
	}
	for _, fd := range fds {
		var stat unix.Stat_t
		if !errors.Is(unix.Fstat(fd, &stat), unix.EBADF) {
			t.Fatal("pinned descriptor leaked")
		}
	}
}

func sandboxContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func TestSandboxHostileResourcesFailClosed(t *testing.T) {
	pkcs11sandbox.RequireBinary(t)
	client := fixtureClientFiles(t, "env")
	dir := sandboxPrivateDir(t)
	file := sandboxWrite(t, dir, "file")
	symlink := filepath.Join(dir, "symlink")
	if err := os.Symlink(file, symlink); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(dir, "symlink-dir")
	if err := os.Symlink(filepath.Dir(client.ModulePath), symlinkDir); err != nil {
		t.Fatal(err)
	}
	unsafeDir := sandboxPrivateDir(t)
	if err := os.Chmod(unsafeDir, 0770); err != nil {
		t.Fatal(err)
	}
	unsafeFile := sandboxWrite(t, unsafeDir, "file")
	publicDir := sandboxPrivateDir(t)
	if err := os.Chmod(publicDir, 0755); err != nil {
		t.Fatal(err)
	}
	hardlink := filepath.Join(dir, "hardlink")
	if err := os.Link(file, hardlink); err != nil {
		t.Fatal(err)
	}
	home, err := sandboxLocalHome()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]SandboxResources{
		"relative":          {ReadOnlyFiles: []string{"relative"}},
		"nul":               {ReadOnlyFiles: []string{file + "\x00suffix"}},
		"newline":           {ReadOnlyFiles: []string{file + "\n"}},
		"oversize path":     {ReadOnlyFiles: []string{"/" + strings.Repeat("a", 4096)}},
		"noncanonical":      {ReadOnlyFiles: []string{dir + "/../file"}},
		"symlink":           {ReadOnlyFiles: []string{symlink}},
		"symlink ancestor":  {ReadOnlyFiles: []string{filepath.Join(symlinkDir, filepath.Base(client.ModulePath))}},
		"writable ancestor": {ReadOnlyFiles: []string{unsafeFile}},
		"public directory":  {ReadOnlyDirs: []string{publicDir}},
		"hardlink":          {ReadOnlyFiles: []string{hardlink}},
		"home":              {ReadWriteDirs: []string{home}},
		"system":            {ReadOnlyDirs: []string{"/usr"}},
		"system var lib":    {ReadWriteDirs: []string{"/var/lib"}},
		"root":              {ReadWriteDirs: []string{"/"}},
		"proc":              {ReadOnlyFiles: []string{"/proc/self/status"}},
		"run":               {ReadOnlyDirs: []string{"/run/user"}},
		"overlap":           {ReadOnlyDirs: []string{dir}, ReadOnlyFiles: []string{file}},
		"module parent":     {ReadWriteDirs: []string{filepath.Dir(client.ModulePath)}},
		"duplicates":        {ReadOnlyDirs: []string{dir, dir}},
		"too many":          {ReadOnlyFiles: make([]string, 17)},
	}
	for name, resources := range cases {
		t.Run(name, func(t *testing.T) {
			launch, err := NewSandboxCommand(context.Background(), client.Executable, client.ModulePath, resources)
			if launch != nil {
				_ = launch.Close()
				t.Fatal("unsafe launch returned")
			}
			if !errors.Is(err, ErrSandboxPolicy) {
				t.Fatalf("error %v", err)
			}
			if err.Error() != ErrSandboxPolicy.Error() {
				t.Fatal("resource disclosed")
			}
		})
	}
}

func TestSandboxSpecialAndOversizeFiles(t *testing.T) {
	pkcs11sandbox.RequireBinary(t)
	client := fixtureClientFiles(t, "env")
	dir := sandboxPrivateDir(t)
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(dir, "large")
	file, err := os.OpenFile(large, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(sandboxMaxFile + 1); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	for _, path := range []string{fifo, large} {
		launch, err := NewSandboxCommand(context.Background(), client.Executable, client.ModulePath, SandboxResources{ReadOnlyFiles: []string{path}})
		if launch != nil {
			_ = launch.Close()
			t.Fatal("special file accepted")
		}
		if !errors.Is(err, ErrSandboxPolicy) {
			t.Fatal(err)
		}
	}
}

func TestSandboxCancellationUnavailableAndLateCleanup(t *testing.T) {
	client := fixtureClientFiles(t, "env")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewSandboxCommand(ctx, client.Executable, client.ModulePath, SandboxResources{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := NewSandboxCommand(nil, client.Executable, client.ModulePath, SandboxResources{}); !errors.Is(err, ErrSandboxPolicy) {
		t.Fatal(err)
	}
	if _, err := newSandboxCommand(context.Background(), client.Executable, client.ModulePath, SandboxResources{}, "/nonexistent-sandbox-launcher"); !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatal(err)
	}
	if _, err := newSandboxCommand(context.Background(), client.Executable, client.ModulePath, SandboxResources{}, client.Executable); !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatal("user-owned launcher accepted")
	}
	t.Run("late cleanup requires installed launcher", func(t *testing.T) {
		pkcs11sandbox.RequireBinary(t)
		before, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 10; i++ {
			if _, err := NewSandboxCommand(context.Background(), client.Executable, client.ModulePath, SandboxResources{ReadOnlyFiles: []string{"/no-such-sandbox-resource"}}); !errors.Is(err, ErrSandboxPolicy) {
				t.Fatal(err)
			}
		}
		after, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != len(before) {
			t.Fatalf("late failure leaked descriptors: before=%d after=%d", len(before), len(after))
		}
	})
}

type sandboxProbe struct {
	Hidden, ReadOnly, Writable, Unix, TCP string
	Module                                string
	HostPID                               int
	Pinned                                []string
}

// This test executable is the synthetic payload, never a PKCS#11 module. Input
// is a private pipe containing only temporary QA paths and loopback addresses.
func TestSandboxSyntheticProbe(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-1] != "sandbox-synthetic-probe" {
		return
	}
	var probe sandboxProbe
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 16384)).Decode(&probe); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 || os.Getenv("HOME") != "" || os.Getenv("GRXFIRMA_SANDBOX_SECRET") != "" {
		t.Fatal("privilege/environment boundary failed")
	}
	if err := HardenProcess(); err != nil {
		t.Fatal("existing worker hardening incompatible with launcher")
	}
	if _, err := os.ReadFile(probe.Hidden); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("host private data visible")
	}
	if _, err := os.ReadFile(probe.ReadOnly); err != nil {
		t.Fatal("read-only grant unavailable")
	}
	if err := os.WriteFile(probe.ReadOnly, []byte("unexpected write"), 0600); err == nil {
		t.Fatal("read-only grant writable")
	}
	if err := os.WriteFile(filepath.Join(probe.Writable, "created"), []byte("synthetic writable grant"), 0600); err != nil {
		t.Fatal("private write grant unavailable")
	}
	if conn, err := net.DialTimeout("unix", probe.Unix, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("host Unix socket visible")
	}
	if conn, err := net.DialTimeout("tcp", probe.TCP, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("host TCP listener reachable")
	}
	// The network checks above exercise namespaces, without relying on seccomp.
	// Now meet the isolated module validator's production precondition as well.
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		if err := RestrictDriverSyscalls(); err != nil {
			t.Fatal(err)
		}
	} else {
		// Namespace/FD tests still run on other architectures. Only the
		// unsupported positive syscall-filter portion is explicitly omitted.
		t.Run("positive syscall restriction", func(t *testing.T) {
			t.Skip("native PKCS11 isolation not supported on this architecture; namespace checks still run")
		})
	}
	broadFile, err := filepath.EvalSymlinks("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateIsolatedModulePath(broadFile); err == nil {
		t.Fatal("file visible through broad /usr mount accepted as pinned module")
	}
	if _, err := ValidateIsolatedModulePath(probe.Module); err != nil {
		t.Fatalf("individually pinned root-host file rejected: %v", err)
	}
	// The new procfs sees the bwrap reaper and this child, not the host parent.
	if _, err := os.Stat("/proc/" + strconv.Itoa(probe.HostPID)); err == nil {
		t.Fatal("host PID visible")
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		target, err := os.Readlink("/proc/self/fd/" + entry.Name())
		if err != nil {
			continue
		}
		for _, forbidden := range probe.Pinned {
			if target == forbidden {
				t.Fatalf("mount-source descriptor inherited: %s", entry.Name())
			}
		}
	}
	// PID 1 must also not retain a descriptor granting access to the old root.
	entries, err = os.ReadDir("/proc/1/fd")
	if err == nil {
		for _, entry := range entries {
			target, _ := os.Readlink("/proc/1/fd/" + entry.Name())
			if target == "/" || strings.Contains(target, "oldroot") {
				t.Fatal("reaper retains host root")
			}
		}
	}
}

func TestSandboxRealSyntheticIsolation(t *testing.T) {
	t.Run("native worker architecture", func(t *testing.T) { pkcs11sandbox.RequireDriverArchitecture(t) })
	client := fixtureClient(t, "env")
	hiddenDir, roDir, rwDir := sandboxPrivateDir(t), sandboxPrivateDir(t), sandboxPrivateDir(t)
	hidden := sandboxWrite(t, hiddenDir, "host-only")
	ro := sandboxWrite(t, roDir, "read-only")
	unixListener, err := net.Listen("unix", filepath.Join(hiddenDir, "qa.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer unixListener.Close()
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpListener.Close()
	// Positive host controls: failures inside must come from the boundary.
	for _, listener := range []net.Listener{unixListener, tcpListener} {
		conn, err := net.DialTimeout(listener.Addr().Network(), listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// /usr/bin/find is administrator-owned host data. It is mounted only to
	// exercise the worker validator, never executed or passed to a native API.
	launch, err := NewSandboxCommand(ctx, client.Executable, "/usr/bin/find", SandboxResources{ReadOnlyFiles: []string{ro}, ReadWriteDirs: []string{rwDir}})
	if err != nil {
		t.Fatal(err)
	}
	defer launch.Close()
	probe := sandboxProbe{Hidden: hidden, ReadOnly: ro, Writable: rwDir, Unix: unixListener.Addr().String(), TCP: tcpListener.Addr().String(), HostPID: os.Getpid(), Module: launch.Module}
	for _, file := range launch.Command.ExtraFiles {
		probe.Pinned = append(probe.Pinned, file.Name())
	}
	payload, err := json.Marshal(probe)
	if err != nil {
		t.Fatal(err)
	}
	launch.Command.Args = append(launch.Command.Args, "-test.run=^TestSandboxSyntheticProbe$", "sandbox-synthetic-probe")
	launch.Command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	launch.Command.Stdout = &stdout
	launch.Command.Stderr = &stderr
	if err := launch.Command.Run(); err != nil {
		t.Fatalf("synthetic sandbox: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "PASS") {
		t.Fatalf("probe did not run: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(rwDir, "created")); err != nil {
		t.Fatal("RW positive control missing")
	}
	if value, err := os.ReadFile(ro); err != nil || string(value) != "synthetic sandbox data" {
		t.Fatal("host read-only file changed")
	}
}
