// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package officecontainer

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestExtractOOXMLSignatureXMLsPreservesFullPaths(t *testing.T) {
	entries := []signatureZIPEntry{
		{"_xmlsignatures/a/signature.xml", "<Signature>primera</Signature>"},
		{"_xmlsignatures/b/signature.xml", "<Signature>segunda</Signature>"},
		{"_xmlsignatures/signature.xml", "<Signature>tercera</Signature>"},
	}
	for _, reverse := range []bool{false, true} {
		ordered := append([]signatureZIPEntry(nil), entries...)
		if reverse {
			ordered[0], ordered[2] = ordered[2], ordered[0]
		}
		got, err := ExtractOOXMLSignatureXMLs(makeOrderedSignatureZIP(t, ordered))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(entries) {
			t.Fatalf("reverse=%v: se perdieron firmas homónimas: got=%d want=%d", reverse, len(got), len(entries))
		}
		for _, entry := range entries {
			if string(got[entry.name]) != entry.body {
				t.Errorf("ruta completa %q no conservada", entry.name)
			}
		}
	}
}

func TestExtractOOXMLSignatureXMLsRejectsDuplicateAndAmbiguousPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"exact_duplicate", []string{"_xmlsignatures/sig.xml", "_xmlsignatures/sig.xml"}},
		{"case_alias", []string{"_xmlsignatures/sig.xml", "_XMLSIGNATURES/SIG.XML"}},
		{"separator_alias", []string{"_xmlsignatures/a/sig.xml", `_xmlsignatures\a\sig.xml`}},
		{"dot_alias", []string{"_xmlsignatures/a/./sig.xml"}},
		{"parent_alias", []string{"_xmlsignatures/a/../sig.xml"}},
		{"escape", []string{"_xmlsignatures/../../sig.xml"}},
		{"empty_segment", []string{"_xmlsignatures/a//sig.xml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var entries []signatureZIPEntry
			for _, name := range tc.paths {
				entries = append(entries, signatureZIPEntry{name, "<Signature/>"})
			}
			got, err := ExtractOOXMLSignatureXMLs(makeOrderedSignatureZIP(t, entries))
			if err == nil || got != nil {
				t.Fatalf("ambigüedad no rechazada explícitamente: firmas=%v error=%v", got, err)
			}
		})
	}
}

func TestExtractOOXMLSignatureXMLsKeepsExistingCaseAndSeparatorCompatibility(t *testing.T) {
	got, err := ExtractOOXMLSignatureXMLs(makeOrderedSignatureZIP(t, []signatureZIPEntry{
		{`_XMLSIGNATURES\Subdir\SIG.XML`, "<Signature/>"},
		{"_xmlsignatures/origin.sigs.xml", "<origin/>"},
		{"other/signature.xml", "<ignored/>"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || string(got["_xmlsignatures/subdir/sig.xml"]) != "<Signature/>" {
		t.Fatalf("compatibilidad normalizada perdida: %v", got)
	}
}

type signatureZIPEntry struct{ name, body string }

func makeOrderedSignatureZIP(t *testing.T, entries []signatureZIPEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		output, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := output.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
