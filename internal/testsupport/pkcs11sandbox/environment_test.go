// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11sandbox

import (
	"fmt"
	"strings"
	"testing"
)

type recordedReport struct {
	fatal, skip bool
	text        string
}

func (*recordedReport) Helper() {}
func (r *recordedReport) Fatalf(format string, args ...any) {
	r.fatal = true
	r.text = fmt.Sprintf(format, args...)
}
func (r *recordedReport) Skipf(format string, args ...any) {
	r.skip = true
	r.text = fmt.Sprintf(format, args...)
}

func TestRequirementDoesNotConvertMissingCoverageToSuccess(t *testing.T) {
	for _, mandatory := range []bool{false, true} {
		for _, broken := range []bool{false, true} {
			r := &recordedReport{}
			report(r, availability{reason: "synthetic_prerequisite_missing", broken: broken}, mandatory)
			if r.fatal != (mandatory || broken) || r.skip == (mandatory || broken) {
				t.Fatalf("mandatory=%v broken=%v result=%+v", mandatory, broken, r)
			}
			if !strings.Contains(r.text, EnvRequire) {
				t.Fatal("missing actionable gate name")
			}
		}
	}
	r := &recordedReport{}
	report(r, availability{}, true)
	if r.fatal || r.skip {
		t.Fatal("available prerequisite did not proceed")
	}
}

func TestSeparateOptInFromGeneralExternalValidators(t *testing.T) {
	t.Setenv("GRXFIRMA_REQUIRE_EXTERNAL_TOOLS", "1")
	t.Setenv(EnvRequire, "")
	if required() {
		t.Fatal("unrelated validator gate forced sandbox")
	}
	for _, value := range []string{"1", "true", "YES", " on "} {
		t.Setenv(EnvRequire, value)
		if !required() {
			t.Fatal(value)
		}
	}
	t.Setenv(EnvRequire, "0")
	if required() {
		t.Fatal("explicit disabled requirement ignored")
	}
}
