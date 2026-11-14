// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/testsupport/pkcs11sandbox"

	"golang.org/x/sys/unix"
)

func TestSandboxDiagnosticPreservesPublicError(t *testing.T) {
	const private = "/home/persona/PIN-1234/modulo"
	external := &os.PathError{Op: "open", Path: private, Err: unix.ENOENT}
	for _, cause := range []error{ErrSandboxPolicy, ErrSandboxUnavailable} {
		inner := sandboxFailure(cause, private, "apertura sin enlaces", external)
		err := sandboxFailure(cause, "/etc/localtime", "apertura fija", inner)
		if err.Error() != cause.Error() || !errors.Is(err, cause) || !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("contrato público alterado: %v", err)
		}
		detail := SandboxDiagnostic(fmt.Errorf("envoltura: %w", err))
		if !strings.Contains(detail, "/etc/localtime") || !strings.Contains(detail, "apertura sin enlaces") || !strings.Contains(detail, "errno=2") || strings.Contains(detail, private) || strings.Contains(detail, "1234") {
			t.Fatalf("diagnóstico incompleto o sin sanear: %q", detail)
		}
		if got := SandboxDiagnostic(inner); strings.Contains(got, private) || !strings.HasPrefix(got, "recurso privado:") {
			t.Fatalf("ruta privada expuesta: %q", got)
		}
	}
	// Ningún error arbitrario del exterior se convierte en texto técnico.
	if got := SandboxDiagnostic(errors.New(private)); strings.Contains(got, private) {
		t.Fatal("error externo expuesto")
	}
}

func TestSandboxDiagnosticHardlink(t *testing.T) {
	dir := t.TempDir()
	path := sandboxWrite(t, dir, "origen")
	if err := os.Link(path, filepath.Join(dir, "segundo")); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stat, err := sandboxStat(file)
	if err != nil {
		t.Fatal(err)
	}
	if sandboxRegular(stat) || !strings.Contains(sandboxRegularCondition(stat, nil), "enlaces=2") {
		t.Fatalf("no se reprodujo el rechazo por enlaces: %s", sandboxRegularCondition(stat, nil))
	}
	t.Logf("Reproducción sin sudo: %s", sandboxRegularCondition(stat, nil))
	// El diagnóstico también distingue al dueño del número de enlaces.
	stat.Nlink = 1
	stat.Uid = 42
	if got := sandboxFileCondition(stat, nil, true, false); !strings.Contains(got, "dueño root") {
		t.Fatalf("dueño no identificado: %q", got)
	}
}

func TestSandboxDiagnosticRegularMatchesPolicy(t *testing.T) {
	for _, mode := range []uint32{unix.S_IFREG | 0600, unix.S_IFREG | 0660, unix.S_IFREG | 06000, unix.S_IFDIR | 0700, unix.S_IFLNK | 0777} {
		for _, size := range []int64{-1, 0, 1, sandboxMaxFile, sandboxMaxFile + 1} {
			stat := unix.Stat_t{Mode: mode, Size: size}
			for stat.Nlink = 0; stat.Nlink <= 2; stat.Nlink++ {
				if sandboxRegular(stat) != (sandboxRegularCondition(stat, nil) == "") {
					t.Fatalf("el diagnóstico difiere de la política: %s", sandboxMetadata(stat, nil))
				}
			}
		}
	}
}

// Prueba de diagnóstico: no sustituye ninguna prueba de aislamiento. CI la
// vuelve a ejecutar con -v tras un fallo para mostrar las condiciones del host.
func TestSandboxSystemDiagnostics(t *testing.T) {
	pkcs11sandbox.LogDiagnostics(t)
	for _, path := range []string{"/", "/tmp", "/usr/bin/bwrap", "/usr", "/lib", "/lib64", "/etc", "/etc/ld.so.cache", "/etc/localtime", "/etc/opensc.conf", "/etc/opensc/opensc.conf", "/etc/passwd", "/run", "/run/pcscd", "/run/pcscd/pcscd.comm"} {
		var stat unix.Stat_t
		err := unix.Lstat(path, &stat)
		t.Logf("Recurso fijo %s: %s; errno=%v", path, sandboxMetadata(stat, err), err)
		if err == nil && stat.Mode&unix.S_IFMT == unix.S_IFLNK {
			err = unix.Stat(path, &stat)
			t.Logf("Destino resuelto de %s: %s; errno=%v", path, sandboxMetadata(stat, err), err)
		}
	}
	dir := sandboxPrivateDir(t)
	executable := sandboxWrite(t, dir, "auxiliar")
	if err := os.Chmod(executable, 0700); err != nil {
		t.Fatal(err)
	}
	module := sandboxWrite(t, dir, "modulo")
	launch, err := NewSandboxCommand(context.Background(), executable, module, SandboxResources{ReadWriteDirs: []string{sandboxPrivateDir(t)}})
	if err != nil {
		t.Logf("Construcción del aislamiento: %v; detalle técnico: %s", err, SandboxDiagnostic(err))
		return
	}
	defer launch.Close()
	t.Log("Construcción del aislamiento correcta; esta prueba no ejecuta el auxiliar")
}
