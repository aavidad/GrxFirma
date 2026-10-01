// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && cgo && (amd64 || arm64)

package main

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/testsupport/pkcs11sandbox"
)

func TestWorkerNativeConstructorIsolation(t *testing.T) {
	if _, present := os.LookupEnv("GRXFIRMA_PKCS11_QA_WORKER"); present && os.Getenv("GRXFIRMA_REQUIRE_PKCS11_SANDBOX") != "1" {
		t.Fatal("QA worker override requires GRXFIRMA_REQUIRE_PKCS11_SANDBOX=1")
	}
	pkcs11sandbox.Require(t)
	cc := pkcs11sandbox.RequireCompiler(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "excluded")
	resultDir := filepath.Join(dir, "results")
	for _, p := range []string{outside, resultDir} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	input := filepath.Join(outside, "input")
	marker := filepath.Join(outside, "marker")
	result := filepath.Join(resultDir, "constructor.json")
	original := []byte("QA synthetic content, never personal")
	if err := os.WriteFile(input, original, 0600); err != nil {
		t.Fatal(err)
	}
	tcp, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	unixPath := filepath.Join(outside, "socket")
	pathListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: unixPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer pathListener.Close()
	abstractName := "af2-isolation-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	abstractListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: "@" + abstractName, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer abstractListener.Close()
	module := filepath.Join(dir, "isolation.so")
	args := []string{"-shared", "-fPIC", "-Wall", "-Wextra", "-o", module, "testdata/isolation_module.c"}
	for key, value := range map[string]string{"QA_INPUT": input, "QA_MARKER": marker, "QA_RESULT": result, "QA_UNIX_PATH": unixPath, "QA_ABSTRACT_NAME": abstractName, "QA_MISSING_EXEC": filepath.Join(outside, "no-executable")} {
		args = append(args, "-D"+key+"="+strconv.Quote(value))
	}
	args = append(args, fmt.Sprintf("-DQA_TCP_PORT=%d", tcp.Addr().(*net.TCPAddr).Port), fmt.Sprintf("-DQA_UDP_PORT=%d", udp.LocalAddr().(*net.UDPAddr).Port))
	if out, err := exec.CommandContext(ctx, cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("module build: %v %s", err, out)
	}
	if err := os.Chmod(module, 0500); err != nil {
		t.Fatal(err)
	}
	controlSource := filepath.Join(dir, "control.c")
	if err := os.WriteFile(controlSource, []byte("#include <dlfcn.h>\nint main(int n,char**v){if(n!=2)return 1;void*p=dlopen(v[1],RTLD_NOW);if(!p)return 2;dlclose(p);return 0;}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(dir, "control")
	if out, err := exec.CommandContext(ctx, cc, "-o", control, controlSource, "-ldl").CombinedOutput(); err != nil {
		t.Fatalf("control build: %v %s", err, out)
	}
	if out, err := exec.CommandContext(ctx, control, module).CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("control: %v %s", err, out)
	}
	readResult := func() map[string]int {
		t.Helper()
		b, err := os.ReadFile(result)
		if err != nil {
			t.Fatal("constructor result missing: ", err)
		}
		var m map[string]int
		if err = json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if len(m) != 9 {
			t.Fatal("incomplete constructor evidence")
		}
		return m
	}
	baseline := readResult()
	for _, key := range []string{"read", "write", "tcp", "udp", "unixPath", "unixAbstract", "fork", "execChild"} {
		if baseline[key] != 0 {
			t.Fatalf("control %s=%d", key, baseline[key])
		}
	}
	if baseline["exec"] != -int(unix.ENOENT) {
		t.Fatal("control exec not reached")
	}
	for _, listener := range []*net.UnixListener{pathListener, abstractListener} {
		listener.SetDeadline(time.Now().Add(time.Second))
		c, err := listener.AcceptUnix()
		if err != nil {
			t.Fatal("control connection missing: ", err)
		}
		c.Close()
	}
	tcp.SetDeadline(time.Now().Add(time.Second))
	c, err := tcp.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	udp.SetReadDeadline(time.Now().Add(time.Second))
	var packet [8]byte
	if n, _, err := udp.ReadFromUDP(packet[:]); err != nil || string(packet[:n]) != "QA" {
		t.Fatal("control datagram missing")
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "QA" {
		t.Fatal("control marker missing")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(result, filepath.Join(resultDir, "control.json")); err != nil {
		t.Fatal(err)
	}
	worker := filepath.Join(dir, "worker")
	if packaged, present := os.LookupEnv("GRXFIRMA_PKCS11_QA_WORKER"); present {
		digest, err := copyIsolationPackagedWorker(packaged, worker)
		if err != nil {
			t.Fatalf("packaged QA worker rejected (no build fallback): %v", err)
		}
		t.Logf("exact packaged worker copied identically; SHA256=%s", digest)
	} else {
		if out, err := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", worker, ".").CombinedOutput(); err != nil {
			t.Fatalf("worker build: %v %s", err, out)
		}
	}
	if err := os.Chmod(worker, 0500); err != nil {
		t.Fatal(err)
	}
	client := pkcs11worker.Client{Executable: worker, ModulePath: module, Timeout: 10 * time.Second, Resources: pkcs11worker.SandboxResources{ReadWriteDirs: []string{resultDir}}}
	for _, path := range []string{worker, module} {
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("artifact %s SHA256=%x", filepath.Base(path), sha256.Sum256(bytes))
	}
	response, workerErr := client.Execute(ctx, pkcs11worker.Request{Operation: "list"})
	t.Logf("worker response code=%s error=%v", response.Code, workerErr)
	actual := readResult() // An error without proof that the constructor ran is NEVER a pass.
	var operationError *pkcs11worker.OperationError
	if !errors.As(workerErr, &operationError) || operationError.Code != "driver_unavailable" {
		t.Fatalf("unexpected protocol completion: %v", workerErr)
	}
	for _, key := range []string{"read", "write", "tcp", "udp", "unixPath", "unixAbstract", "exec", "fork"} {
		if actual[key] >= 0 {
			t.Fatalf("sandbox allowed %s: %+v", key, actual)
		}
	}
	for _, key := range []string{"tcp", "udp", "exec", "fork"} {
		if actual[key] != -int(unix.EPERM) {
			t.Fatalf("missing syscall filter for %s: %d", key, actual[key])
		}
	}
	if actual["execChild"] != -1 {
		t.Fatal("child executed")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("excluded marker created")
	}
	if b, err := os.ReadFile(input); err != nil || sha256.Sum256(b) != sha256.Sum256(original) {
		t.Fatal("excluded file changed")
	}
	for _, listener := range []*net.UnixListener{pathListener, abstractListener} {
		listener.SetDeadline(time.Now().Add(30 * time.Millisecond))
		c, err := listener.AcceptUnix()
		if err == nil {
			c.Close()
			t.Fatal("worker connected to excluded UNIX listener")
		}
	}
	tcp.SetDeadline(time.Now().Add(30 * time.Millisecond))
	if c, err := tcp.AcceptTCP(); err == nil {
		c.Close()
		t.Fatal("worker TCP connection")
	}
	udp.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	if _, _, err := udp.ReadFromUDP(packet[:]); err == nil {
		t.Fatal("worker UDP datagram")
	}
	t.Logf("constructor controls=%v confined=%v input SHA256=%x", baseline, actual, sha256.Sum256(original))

	// A fresh build of the same fixture blocks only after emitting its proof.
	if out, err := exec.CommandContext(ctx, cc, append(args, "-DQA_HANG=1")...).CombinedOutput(); err != nil {
		t.Fatalf("hang module: %v %s", err, out)
	}
	if err := os.Chmod(module, 0500); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			if err := os.Remove(result); err != nil {
				t.Fatal(err)
			}
			operation, stop := context.WithCancel(ctx)
			defer stop()
			copyClient := client
			if mode == "deadline" {
				copyClient.Timeout = 1500 * time.Millisecond
			}
			finished := make(chan error, 1)
			go func() {
				_, err := copyClient.Execute(operation, pkcs11worker.Request{Operation: "list"})
				finished <- err
			}()
			until := time.Now().Add(time.Second)
			for {
				if _, err := os.Stat(result); err == nil {
					break
				}
				if time.Now().After(until) {
					t.Fatal("hanging constructor did not run")
				}
				time.Sleep(5 * time.Millisecond)
			}
			owned := isolationProcesses(t, dir)
			workerSeen := false
			parentGroup := unix.Getpgrp()
			for _, p := range owned {
				if p.group == parentGroup {
					t.Fatalf("QA child shares host process group: %+v", p)
				}
				if p.executable == worker {
					workerSeen = true
				}
			}
			if !workerSeen {
				t.Fatalf("did not observe live native worker: %+v", owned)
			}
			t.Logf("live QA processes before %s: %+v; parent group=%d", mode, owned, parentGroup)
			start := time.Now()
			if mode == "cancel" {
				stop()
			}
			select {
			case err := <-finished:
				want := context.Canceled
				if mode == "deadline" {
					want = context.DeadlineExceeded
				}
				if !errors.Is(err, want) {
					t.Fatalf("completion=%v want=%v", err, want)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("native cancellation unbounded")
			}
			until = time.Now().Add(3 * time.Second)
			for {
				left := false
				for _, p := range owned {
					if current, ok := isolationProcess(p.pid); ok && current.start == p.start {
						left = true
					}
				}
				if !left {
					break
				}
				if time.Now().After(until) {
					t.Fatalf("QA processes remained after %s: %+v", mode, isolationProcesses(t, dir))
				}
				time.Sleep(10 * time.Millisecond)
			}
			if unix.Getpgrp() != parentGroup {
				t.Fatal("host process group changed")
			}
			t.Logf("%s completed in %s; every observed QA PID reaped", mode, time.Since(start))
		})
	}
}

// Test-only opt-in: the application never accepts this environment variable.
// Read through a pinned, bounded regular file and validate identity before use.
func copyIsolationPackagedWorker(source, destination string) (string, error) {
	if os.Getenv("GRXFIRMA_REQUIRE_PKCS11_SANDBOX") != "1" || !filepath.IsAbs(source) {
		return "", fmt.Errorf("explicit absolute worker path and strict sandbox opt-in required")
	}
	fd, err := unix.Open(source, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	input := os.NewFile(uintptr(fd), "packaged-qa-worker")
	defer input.Close()
	stat, err := input.Stat()
	const maxWorkerSize = 128 * 1024 * 1024
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > maxWorkerSize || stat.Mode().Perm()&0111 == 0 || stat.Mode().Perm()&0022 != 0 || stat.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return "", fmt.Errorf("QA worker must be a bounded non-writable regular executable")
	}
	info, err := buildinfo.Read(input)
	if err != nil {
		return "", fmt.Errorf("missing Go build identity: %w", err)
	}
	settings := make(map[string]string)
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	production := false
	for _, tag := range strings.FieldsFunc(settings["-tags"], func(r rune) bool { return r == ',' || r == ' ' }) {
		if tag == "production" {
			production = true
		}
	}
	if info.Path != "grxfirma/cmd/grxfirma-pkcs11-worker" || settings["CGO_ENABLED"] != "1" || settings["GOOS"] != "linux" || !production {
		return "", fmt.Errorf("QA worker identity, real CGo or production tag mismatch")
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return "", err
	}
	originalHash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(output, originalHash), io.LimitReader(input, maxWorkerSize+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || n != stat.Size() || n > maxWorkerSize {
		return "", fmt.Errorf("QA worker copy incomplete or source changed")
	}
	copied, err := os.Open(destination)
	if err != nil {
		return "", err
	}
	defer copied.Close()
	copyHash := sha256.New()
	if _, err := io.Copy(copyHash, io.LimitReader(copied, maxWorkerSize+1)); err != nil {
		return "", err
	}
	if !strings.EqualFold(fmt.Sprintf("%x", originalHash.Sum(nil)), fmt.Sprintf("%x", copyHash.Sum(nil))) {
		return "", fmt.Errorf("QA worker copied bytes differ")
	}
	return fmt.Sprintf("%x", copyHash.Sum(nil)), nil
}

func TestPackagedWorkerOptInRejectsUnsafeInputs(t *testing.T) {
	t.Setenv("GRXFIRMA_REQUIRE_PKCS11_SANDBOX", "1")
	dir := t.TempDir()
	plain := filepath.Join(dir, "not-a-go-worker")
	if err := os.WriteFile(plain, []byte("synthetic non-executable content"), 0500); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(plain, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for i, source := range []string{"", "relative", dir, plain, link, fifo} {
		if _, err := copyIsolationPackagedWorker(source, filepath.Join(dir, strconv.Itoa(i))); err == nil {
			t.Fatalf("accepted invalid packaged worker %q", source)
		}
	}
	t.Setenv("GRXFIRMA_REQUIRE_PKCS11_SANDBOX", "")
	if _, err := copyIsolationPackagedWorker(plain, filepath.Join(dir, "without-opt-in")); err == nil {
		t.Fatal("accepted absent strict opt-in")
	}
}

type isolationProcessInfo struct {
	pid, parent, group int
	start, executable  string
}

func isolationProcess(pid int) (isolationProcessInfo, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return isolationProcessInfo{}, false
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return isolationProcessInfo{}, false
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return isolationProcessInfo{}, false
	}
	group, _ := strconv.Atoi(fields[2])
	parent, _ := strconv.Atoi(fields[1])
	return isolationProcessInfo{pid: pid, parent: parent, group: group, start: fields[19]}, true
}

func isolationProcesses(t *testing.T, prefix string) []isolationProcessInfo {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	var result []isolationProcessInfo
	processes := make(map[int]isolationProcessInfo)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if p, ok := isolationProcess(pid); ok {
			processes[pid] = p
		}
	}
	for pid, p := range processes {
		ancestor := p.parent
		for hops := 0; ancestor > 1 && ancestor != os.Getpid() && hops < 32; hops++ {
			ancestor = processes[ancestor].parent
		}
		if ancestor != os.Getpid() {
			continue
		} // Never inspect unrelated processes' arguments.
		args, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
		if err != nil || !strings.Contains(string(args), prefix) {
			continue
		}
		p.executable = strings.Split(string(args), "\x00")[0]
		result = append(result, p)
	}
	return result
}
