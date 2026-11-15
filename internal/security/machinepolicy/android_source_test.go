// SPDX-License-Identifier: EUPL-1.2
package machinepolicy

import (
	"go/build"
	"testing"
)

// Android no dispone de directiva de máquina en /etc. El lector Unix que
// exige propietario root y rechaza enlaces queda solo en escritorio.
func TestAndroidNoCompilaLectorUnixDePolitica(t *testing.T) {
	ctx := build.Default
	ctx.GOOS = "android"
	ctx.GOARCH = "arm64"
	for _, name := range []string{"machinepolicy_other.go", "policyfile_unix.go"} {
		included, err := ctx.MatchFile(".", name)
		if err != nil {
			t.Fatal(err)
		}
		if included {
			t.Errorf("Android incluye %s: intentaría leer /etc", name)
		}
	}
	included, err := ctx.MatchFile(".", "machinepolicy_android.go")
	if err != nil {
		t.Fatal(err)
	}
	if !included {
		t.Fatal("Android carece de fuente de política ausente")
	}
}
