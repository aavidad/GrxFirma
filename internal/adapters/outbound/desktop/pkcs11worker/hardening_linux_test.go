// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestValidateModulePathControlledAncestors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string) string
		valid bool
	}{
		{"regular", func(t *testing.T, base string) string { return moduleFixture(t, base) }, true},
		{"relative", func(*testing.T, string) string { return "module.so" }, false},
		{"missing", func(_ *testing.T, base string) string { return filepath.Join(base, "missing.so") }, false},
		{"empty", func(t *testing.T, base string) string {
			p := moduleFixture(t, base)
			if err := os.Truncate(p, 0); err != nil {
				t.Fatal(err)
			}
			return p
		}, false},
		{"oversized", func(t *testing.T, base string) string {
			p := moduleFixture(t, base)
			if err := os.Truncate(p, 128*1024*1024+1); err != nil {
				t.Fatal(err)
			}
			return p
		}, false},
		{"group_writable_file", func(t *testing.T, base string) string {
			p := moduleFixture(t, base)
			if err := os.Chmod(p, 0660); err != nil {
				t.Fatal(err)
			}
			return p
		}, false},
		{"world_writable_file", func(t *testing.T, base string) string {
			p := moduleFixture(t, base)
			if err := os.Chmod(p, 0602); err != nil {
				t.Fatal(err)
			}
			return p
		}, false},
		{"writable_ancestor", func(t *testing.T, base string) string {
			p := moduleFixture(t, base)
			if err := os.Chmod(base, 0777); err != nil {
				t.Fatal(err)
			}
			return p
		}, false},
		{"directory", func(_ *testing.T, base string) string { return base }, false},
		{"fifo", func(t *testing.T, base string) string {
			p := filepath.Join(base, "fifo")
			if err := unix.Mkfifo(p, 0600); err != nil {
				t.Fatal(err)
			}
			return p
		}, false},
		{"device", func(*testing.T, string) string { return "/dev/null" }, false},
		{"symlink_to_controlled_file", func(t *testing.T, base string) string {
			p := moduleFixture(t, base)
			link := filepath.Join(base, "link.so")
			if err := os.Symlink(p, link); err != nil {
				t.Fatal(err)
			}
			return link
		}, true},
		{"symlink_to_writable_target", func(t *testing.T, base string) string {
			p := moduleFixture(t, base)
			if err := os.Chmod(p, 0666); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(base, "link.so")
			if err := os.Symlink(p, link); err != nil {
				t.Fatal(err)
			}
			return link
		}, false},
		{"dangling_symlink", func(t *testing.T, base string) string {
			link := filepath.Join(base, "link.so")
			if err := os.Symlink("missing.so", link); err != nil {
				t.Fatal(err)
			}
			return link
		}, false},
		{"symlink_ancestor", func(t *testing.T, base string) string {
			sub := filepath.Join(base, "actual")
			if err := os.Mkdir(sub, 0700); err != nil {
				t.Fatal(err)
			}
			moduleFixture(t, sub)
			link := filepath.Join(base, "alias")
			if err := os.Symlink(sub, link); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(link, "module.so")
		}, true},
		{"nul", func(_ *testing.T, base string) string { return filepath.Join(base, "module.so") + "\x00" }, false},
		{"too_long", func(_ *testing.T, base string) string { return base + "/" + strings.Repeat("a", 4096) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			if err := os.Chmod(base, 0700); err != nil {
				t.Fatal(err)
			}
			path := tc.setup(t, base)
			resolved, err := ValidateModulePath(path)
			if tc.valid {
				want, e := filepath.EvalSymlinks(path)
				if e != nil || err != nil || resolved != want {
					t.Fatalf("resolved=%q error=%v want=%q", resolved, err, want)
				}
			} else if err == nil || resolved != "" {
				t.Fatalf("ruta insegura admitida: %q %v", resolved, err)
			}
		})
	}
}

func moduleFixture(t *testing.T, base string) string {
	t.Helper()
	path := filepath.Join(base, "module.so")
	if err := os.WriteFile(path, []byte("fixture sin código ni certificados"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Simula propietarios para probar la política sin chown ni privilegios.
type moduleStatFixture struct {
	os.FileInfo
	owner       uint32
	mode        os.FileMode
	invalidStat bool
}

func (s moduleStatFixture) Sys() any {
	if s.invalidStat {
		return nil
	}
	return &syscall.Stat_t{Uid: s.owner}
}
func (s moduleStatFixture) Mode() os.FileMode { return s.mode }
func (s moduleStatFixture) IsDir() bool       { return s.mode.IsDir() }

func TestModuleOwnerAndStickyDirectoryPolicy(t *testing.T) {
	path := moduleFixture(t, t.TempDir())
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	const user = uint32(1000)
	for _, tc := range []struct {
		name        string
		owner       uint32
		mode        os.FileMode
		leaf, valid bool
	}{
		{"user_file", user, 0600, true, true}, {"root_file", 0, 0644, true, true}, {"foreign_file", 1001, 0600, true, false},
		{"root_private_directory", 0, os.ModeDir | 0755, false, true}, {"foreign_directory", 1001, os.ModeDir | 0700, false, false},
		{"root_sticky_tmp", 0, os.ModeDir | os.ModeSticky | 0777, false, true},
		{"root_writable_without_sticky", 0, os.ModeDir | 0777, false, false},
		{"user_writable_sticky", user, os.ModeDir | os.ModeSticky | 0777, false, false},
		{"file_instead_of_parent", user, 0600, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := moduleEntryAllowed(moduleStatFixture{FileInfo: info, owner: tc.owner, mode: tc.mode}, user, tc.leaf); got != tc.valid {
				t.Fatalf("allowed=%v want=%v", got, tc.valid)
			}
		})
	}
	if moduleEntryAllowed(moduleStatFixture{FileInfo: info, invalidStat: true, mode: 0600}, user, true) {
		t.Fatal("aceptó metadatos sin propietario")
	}
}

const hardeningHelperEnv = "GRXFIRMA_TEST_HARDENING_HELPER"

var privateFrame = []byte{0, 0, 0, 4, 0, 1, 254, 255}

func helperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHardeningHelperProcess$")
	cmd.Env = append(cmd.Environ(), hardeningHelperEnv+"="+mode)
	return cmd
}

func TestHardenProcessOnlyMutatesSubprocess(t *testing.T) {
	beforeDump, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeNNP, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var beforeCore unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &beforeCore); err != nil {
		t.Fatal(err)
	}
	output, err := helperCommand(t, "harden").CombinedOutput()
	if err != nil {
		t.Fatalf("subproceso: %v: %s", err, output)
	}
	if string(output) != "hardened-all-threads" && string(output) != "privileged-process-rejected" {
		t.Fatalf("resultado inesperado: %q", output)
	}
	afterDump, _ := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	afterNNP, _ := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	var afterCore unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &afterCore); err != nil {
		t.Fatal(err)
	}
	if beforeDump != afterDump || beforeNNP != afterNNP || beforeCore != afterCore {
		t.Fatal("el hardening alteró el proceso principal de pruebas")
	}
}

func TestPrivateProtocolOutputPreservesBinaryPipe(t *testing.T) {
	for _, mode := range []string{"private_output", "private_output_closed_stderr"} {
		t.Run(mode, func(t *testing.T) {
			cmd := helperCommand(t, mode)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("subproceso: %v: %s", err, stderr.String())
			}
			if !bytes.Equal(output, privateFrame) || stderr.Len() != 0 {
				t.Fatalf("canal contaminado: salida=%x stderr=%q", output, stderr.String())
			}
		})
	}
}

func TestPrivateProtocolOutputRejectsFileWithoutRedirecting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := helperCommand(t, "reject_file")
	cmd.Stdout = file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("subproceso: %v %v %s", err, closeErr, stderr.String())
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("salida modificada: %q %v", content, err)
	}
}

// Todas las llamadas mutadoras de hardening y descriptores se ejecutan aquí,
// exclusivamente en un proceso hijo con marcador de prueba explícito.
func TestHardeningHelperProcess(t *testing.T) {
	mode := os.Getenv(hardeningHelperEnv)
	if mode == "" {
		return
	}
	if err := runHardeningHelper(mode); err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func runHardeningHelper(mode string) error {
	switch mode {
	case "harden":
		if os.Geteuid() == 0 {
			if HardenProcess() == nil {
				return errors.New("root aceptado")
			}
			_, err := os.Stdout.WriteString("privileged-process-rejected")
			return err
		}
		ready := make(chan struct{}, 4)
		release := make(chan struct{})
		var group sync.WaitGroup
		for i := 0; i < 4; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				ready <- struct{}{}
				<-release
			}()
		}
		defer func() { close(release); group.Wait() }()
		for i := 0; i < 4; i++ {
			<-ready
		}
		if err := HardenProcess(); err != nil {
			return err
		}
		threads, err := os.ReadDir("/proc/self/task")
		if err != nil {
			return err
		}
		for _, thread := range threads {
			status, err := os.ReadFile(filepath.Join("/proc/self/task", thread.Name(), "status"))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			found := false
			for _, line := range strings.Split(string(status), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[0] == "NoNewPrivs:" {
					found = fields[1] == "1"
				}
			}
			if !found {
				return errors.New("hilo sin no_new_privs")
			}
		}
		dump, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
		if err != nil || dump != 0 {
			return errors.New("dumpable no desactivado")
		}
		var core unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_CORE, &core); err != nil || core.Cur != 0 || core.Max != 0 {
			return errors.New("core no desactivado")
		}
		if unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 0, 0, 0, 0) == nil {
			return errors.New("no_new_privs reversible")
		}
		_, err = os.Stdout.WriteString("hardened-all-threads")
		return err
	case "private_output", "private_output_closed_stderr":
		if mode == "private_output_closed_stderr" {
			if err := unix.Close(unix.Stderr); err != nil {
				return err
			}
		}
		output, err := PrivateProtocolOutput()
		if err != nil {
			return err
		}
		defer output.Close()
		flags, err := unix.FcntlInt(output.Fd(), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			return errors.New("canal privado heredable")
		}
		var null unix.Stat_t
		if err := unix.Stat("/dev/null", &null); err != nil {
			return err
		}
		for _, fd := range []int{unix.Stdout, unix.Stderr} {
			var stat unix.Stat_t
			if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFCHR || stat.Rdev != null.Rdev {
				return errors.New("salida del driver no protegida")
			}
		}
		_, _ = fmt.Fprint(os.Stdout, "synthetic-driver-secret")
		_, _ = unix.Write(unix.Stdout, []byte("synthetic-native-secret"))
		_, _ = unix.Write(unix.Stderr, []byte("synthetic-stderr-secret"))
		_, err = output.Write(privateFrame)
		return err
	case "reject_file":
		output, err := PrivateProtocolOutput()
		if output != nil {
			output.Close()
		}
		if err == nil {
			return errors.New("archivo aceptado como canal privado")
		}
		_, err = os.Stdout.WriteString("unchanged")
		return err
	default:
		return errors.New("modo de prueba no valido")
	}
}
