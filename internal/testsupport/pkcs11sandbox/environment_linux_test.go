// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11sandbox

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRequiredOptionsDetectedWithoutVersionGuessing(t *testing.T) {
	if state := optionsAvailable("bubblewrap 0.backport\n" + requiredOptions); state.reason != "" {
		t.Fatal(state)
	}
	for _, option := range strings.Fields(requiredOptions) {
		if state := optionsAvailable(strings.ReplaceAll(requiredOptions, option, "")); state.reason != "bubblewrap_required_options_missing" || state.broken {
			t.Fatalf("missing %s: %+v", option, state)
		}
	}
}

type fakeBinaryInfo struct {
	mode os.FileMode
	uid  uint32
}

func (i fakeBinaryInfo) Name() string       { return "bwrap" }
func (i fakeBinaryInfo) Size() int64        { return 123 }
func (i fakeBinaryInfo) Mode() os.FileMode  { return i.mode }
func (i fakeBinaryInfo) ModTime() time.Time { return time.Time{} }
func (i fakeBinaryInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeBinaryInfo) Sys() any           { return &syscall.Stat_t{Uid: i.uid} }

func TestUnsafeBinaryIsNeverAnOptionalAbsence(t *testing.T) {
	if !binaryPermissionsAllowed(fakeBinaryInfo{mode: 0755}) {
		t.Fatal("administrator binary rejected")
	}
	for _, info := range []fakeBinaryInfo{{mode: 0755, uid: 1000}, {mode: 0775}, {mode: 0644}, {mode: 0755 | os.ModeSetuid}, {mode: 0755 | os.ModeSetgid}, {mode: 0755 | os.ModeSymlink}, {mode: 0755 | os.ModeDir}} {
		if binaryPermissionsAllowed(info) {
			t.Fatalf("unsafe binary accepted: %+v", info)
		}
	}
	r := &recordedReport{}
	report(r, availability{reason: "bubblewrap_unsafe_permissions", broken: true}, false)
	if !r.fatal || r.skip {
		t.Fatal("unsafe installed binary skipped")
	}
}

func TestExternalPreflightWithoutApplicationOrNativeModule(t *testing.T) { Require(t) }

func TestExternalOutputBound(t *testing.T) {
	var output boundedOutput
	data := []byte(strings.Repeat("x", 131072))
	n, err := output.Write(data)
	if n != len(data) || err != nil || output.Len() != 65536 {
		t.Fatal("output not bounded")
	}
}
