// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package afirmauri

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/appdirs"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestAfirmaURIParametrosNoActivanExcepcion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	dir := appdirs.Config(home)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"excepciones_inseguras":{"sha1_crear":false}}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	adapter := New(trustStub{estado: domain.TrustAllowed})
	extra := base64.StdEncoding.EncodeToString([]byte("excepciones_inseguras.sha1_crear=true\npermitir_sha1_legacy=true\nchannel=local_ipc\nsource=gui\naccepted=true"))
	raw := "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA&id=sha1_crear&enabled=true&accepted=true&source=gui&channel=local_ipc&unsafe_exceptions_activate=true&properties=" + url.QueryEscape(extra)
	request, err := adapter.ParseSocket(context.Background(), raw)
	if err != nil {
		t.Fatalf("la solicitud normal con parámetros ajenos debe poder analizarse: %v", err)
	}
	if request.Operacion != OperacionFirma {
		t.Fatalf("operación inesperada: %s", request.Operacion)
	}
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if excepciones.Check("sha1_crear", ctx) == nil {
		t.Fatal("los parámetros del portal activaron SHA-1")
	}
	for _, action := range []string{"unsafe_exceptions_activate", "unsafe_exceptions_set", "unsafe_exceptions_retry"} {
		if _, err := adapter.ParseSocket(ctx, "afirma://"+action+"?id=sha1_crear&enabled=true&accepted=true&source=gui&channel=local_ipc"); err == nil {
			t.Fatalf("se aceptó %s como operación URI", action)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("URI cambió ajustes: %s (%v)", got, err)
	}
}
