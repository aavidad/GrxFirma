// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"grxfirma/internal/appdirs"
)

func TestLaunchControlSocketSoloEnXDGPrivado(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	t.Setenv(legacyLaunchControlSocketEnv, "")
	xdg := filepath.Join(t.TempDir(), "xdg")
	if err := os.Mkdir(xdg, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	if got, gestionada, err := legacyLaunchControlSocketPath(); err != nil || !gestionada || got != filepath.Join(xdg, "grxfirma-afirmauri-control.sock") {
		t.Fatalf("XDG privado: %q %v %v", got, gestionada, err)
	}
	if err := os.Chmod(xdg, 0o777); err != nil {
		t.Fatal(err)
	}
	casa, _ := os.UserHomeDir()
	if got, _, err := legacyLaunchControlSocketPath(); err != nil || got != filepath.Join(appdirs.Cache(casa), "afirmauri-control.sock") {
		t.Fatalf("XDG compartido usado: %q %v", got, err)
	}
}

// Un socket en una carpeta que no es privada no recibe la URI.
func TestLaunchControlNoReenviaASocketDeCarpetaNoPrivada(t *testing.T) {
	// Ruta corta: un socket unix no admite más de ~100 bytes.
	home, err := os.MkdirTemp("/tmp", "lc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	setTestUserHome(t, home)
	t.Setenv(legacyLaunchControlSocketEnv, "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	socketPath, _, err := legacyLaunchControlSocketPath()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("socket unix no disponible: %v", err)
	}
	defer ln.Close()
	aceptada := make(chan struct{}, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			aceptada <- struct{}{}
			_ = c.Close()
		}
	}()
	if reenviada, err := tryForwardLaunchToResident("afirma://sign?id=uno"); reenviada || err != nil {
		t.Fatalf("reenviada=%v err=%v", reenviada, err)
	}
	select {
	case <-aceptada:
		t.Fatal("se conectó a un socket de una carpeta no privada")
	case <-time.After(200 * time.Millisecond):
	}
}
