// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package pkcs11sandbox checks external prerequisites for integration tests.
// It never imports the production launcher or converts its failures to skips.
package pkcs11sandbox

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

const EnvRequire = "GRXFIRMA_REQUIRE_PKCS11_SANDBOX"

func required() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvRequire))) {
	case "1", "true", "yes", "si", "on":
		return true
	default:
		return false
	}
}

type availability struct {
	reason string
	broken bool
}
type reporter interface {
	Helper()
	Fatalf(string, ...any)
	Skipf(string, ...any)
}

func report(t reporter, state availability, mandatory bool) {
	t.Helper()
	if state.reason == "" {
		return
	}
	if state.broken || mandatory {
		t.Fatalf("PKCS11 sandbox prerequisite failed: %s (%s=%t); no isolation validation was completed", state.reason, EnvRequire, mandatory)
		return
	}
	t.Skipf("PKCS11 sandbox integration NOT VALIDATED: %s; set %s=1 to require it", state.reason, EnvRequire)
}

// RequireCompiler is for synthetic native integration fixtures only. It does
// not install anything or treat a compiler/build failure as unavailability.
func RequireCompiler(t testing.TB) string {
	t.Helper()
	path, err := exec.LookPath("cc")
	if err != nil {
		report(t, availability{reason: "c_compiler_missing"}, required())
		return ""
	}
	return path
}

// RequireDriverArchitecture applies only to positive native-driver tests. Use
// a subtest so unsupported targets still run namespace and pure protocol tests.
func RequireDriverArchitecture(t testing.TB) {
	t.Helper()
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		report(t, availability{reason: "driver_architecture_unsupported"}, required())
	}
}
