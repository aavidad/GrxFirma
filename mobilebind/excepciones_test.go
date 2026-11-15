// SPDX-License-Identifier: EUPL-1.2
package mobilebind

import (
	"context"
	"encoding/json"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"testing"
)

func TestMobileExceptionsUsePrivateDirectory(t *testing.T) {
	f := &Facade{configDir: t.TempDir()}
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	raw, err := f.SetUnsafeExceptionJSON(`{"id":"tsa_http","enabled":true}`)
	if err != nil {
		t.Fatal(err)
	}
	var values []excepciones.State
	if json.Unmarshal([]byte(raw), &values) != nil || len(values) != 26 {
		t.Fatal("catálogo móvil")
	}
	if err := excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), f.configDir)); err != nil {
		t.Fatal(err)
	}
	other := &Facade{configDir: t.TempDir()}
	if err := excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), other.configDir)); err == nil {
		t.Fatal("ajuste compartido entre fachadas")
	}
	machinepolicy.SetForTest(t, "permitir_tsa_http", false)
	if _, err = f.SetUnsafeExceptionJSON(`{"id":"tsa_http","enabled":true}`); err == nil {
		t.Fatal("soslaya política")
	}
}
