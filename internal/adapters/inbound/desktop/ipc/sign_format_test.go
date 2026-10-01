// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

func TestSignResponseReportsEngineFormat(t *testing.T) {
	for _, action := range []string{"sign", "sign_multicosign"} {
		for _, requested := range []string{"", "auto", "PAdES"} {
			for _, inline := range []bool{false, true} {
				t.Run(action+"/"+requested+"/"+map[bool]string{false: "file", true: "base64"}[inline], func(t *testing.T) {
					dir := t.TempDir()
					input := filepath.Join(dir, "input.pdf")
					output := filepath.Join(dir, "output.pdf")
					if err := os.WriteFile(input, []byte("QA synthetic input"), 0o600); err != nil {
						t.Fatal(err)
					}
					// Contradice deliberadamente extensión y petición: manda el motor.
					engine := application.SignResult{Result: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("QA signed")}}
					m := &Manejador{Firmar: &stubFirmar{result: engine}, MultiCofirmar: &stubMultiCofirmar{result: engine}, ultimosCerts: []domain.CertificateRef{{ID: "qa"}}}
					resp := m.despachar(context.Background(), peticionJSON(t, action, paramsFirma{InputPath: input, OutputPath: output, CertificateID: "qa", Format: requested, Action: "sign", ReturnSignatureB64: inline}))
					if !resp.OK {
						t.Fatal(resp.Error)
					}
					data := resp.Data.(resultadoFirma)
					if data.Format != "CAdES" {
						t.Fatalf("format=%q", data.Format)
					}
					if inline {
						if data.SignatureB64 != base64.StdEncoding.EncodeToString(engine.Result.Data) || data.OutputPath != "" {
							t.Fatal("base64 response changed")
						}
						if _, err := os.Stat(output); !os.IsNotExist(err) {
							t.Fatal("base64 wrote output")
						}
					} else {
						if data.OutputPath != output || data.SignatureB64 != "" {
							t.Fatal("file response changed")
						}
						got, err := os.ReadFile(output)
						if err != nil || string(got) != string(engine.Result.Data) {
							t.Fatal("output changed", err)
						}
					}
					raw, err := json.Marshal(data)
					if err != nil {
						t.Fatal(err)
					}
					var fields map[string]any
					if err := json.Unmarshal(raw, &fields); err != nil {
						t.Fatal(err)
					}
					if fields["format"] != "CAdES" {
						t.Fatalf("wire format missing: %s", raw)
					}
				})
			}
		}
	}
}

func TestSignResponseFormatRemainsOptional(t *testing.T) {
	raw, err := json.Marshal(resultadoFirma{OutputPath: "qa-output"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["format"]; exists {
		t.Fatal("empty format must be omitted")
	}
}
