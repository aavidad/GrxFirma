// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestIsVersionRequest(t *testing.T) {
	for _, flag := range []string{"--version", "-version"} {
		if !isVersionRequest([]string{flag}) {
			t.Fatalf("diagnóstico no reconocido: %s", flag)
		}
	}
	for _, args := range [][]string{
		nil, {}, {"version"}, {"--Version"}, {" --version"},
		{"chrome-extension://example/"},
		{"--version", "chrome-extension://example/"},
		{"chrome-extension://example/", "--version"},
		{"-version", "--parent-window=0"}, {"--version", "--version"},
	} {
		if isVersionRequest(args) {
			t.Fatalf("argumentos normales desviados al diagnóstico: %q", args)
		}
	}
}

func TestNativeHostVersionBeforeBootstrap(t *testing.T) {
	if flag := os.Getenv("GRXFIRMA_NATIVEHOST_VERSION_TEST_CHILD"); flag != "" {
		if flag != "--version" && flag != "-version" {
			t.Fatal("flag de arnés no permitido")
		}
		os.Args = []string{"grxfirma-nativehost", flag}
		version = "2.0.1-rc.1+qa"
		main()
		os.Exit(0)
	}
	for _, flag := range []string{"--version", "-version"} {
		t.Run(flag, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeHostVersionBeforeBootstrap$")
			cmd.Env = append(os.Environ(), "GRXFIRMA_NATIVEHOST_VERSION_TEST_CHILD="+flag)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("diagnóstico terminó con error: %v; stderr=%q", err, stderr.String())
			}
			if got := stdout.String(); got != "grxfirma-nativehost 2.0.1-rc.1+qa\n" {
				t.Fatalf("salida inesperada: %q", got)
			}
			if stderr.Len() != 0 {
				t.Fatalf("diagnóstico inicializó logging/bootstrap: %q", stderr.String())
			}
		})
	}
}
