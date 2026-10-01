// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import (
	"errors"
	"strings"
	"testing"
)

func TestBackendReadinessForPlatform(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		goos          string
		wantBackend   string
		wantReasonHas string
	}{
		{
			name:          "darwin uses keychain readiness",
			goos:          "darwin",
			wantBackend:   "keychain",
			wantReasonHas: "Keychain",
		},
		{
			name:          "windows uses dpapi readiness",
			goos:          "windows",
			wantBackend:   windowsBackendName,
			wantReasonHas: "DPAPI",
		},
		{
			name:          "freebsd is unsupported",
			goos:          "freebsd",
			wantBackend:   "unsupported",
			wantReasonHas: "freebsd",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotBackend, gotReason := backendReadinessForPlatform(tc.goos)
			if gotBackend != tc.wantBackend {
				t.Fatalf("backendReadinessForPlatform(%q) backend = %q, want %q", tc.goos, gotBackend, tc.wantBackend)
			}
			if !strings.Contains(gotReason, tc.wantReasonHas) {
				t.Fatalf("backendReadinessForPlatform(%q) reason = %q, want substring %q", tc.goos, gotReason, tc.wantReasonHas)
			}
		})
	}
}

// DPAPI es nativo: la disponibilidad no depende de herramientas externas.
func TestBackendReadinessForPlatformWithProbe_WindowsNoDependeDeHerramientas(t *testing.T) {
	t.Parallel()

	for _, probe := range []func(string) (string, error){
		func(name string) (string, error) { return "C:\\Windows\\System32\\" + name, nil },
		func(string) (string, error) { return "", errors.New("not found") },
	} {
		backend, reason := backendReadinessForPlatformWithProbe("windows", probe)
		if backend != windowsBackendName {
			t.Fatalf("backend = %q, want %s", backend, windowsBackendName)
		}
		for _, needle := range []string{"nativo", "proxySecretId", "preparado"} {
			if !strings.Contains(reason, needle) {
				t.Fatalf("reason = %q, want substring %q", reason, needle)
			}
		}
		if strings.Contains(reason, "powershell") {
			t.Fatalf("reason sigue dependiendo de PowerShell: %q", reason)
		}
	}
}
