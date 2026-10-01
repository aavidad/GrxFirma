// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import "testing"

func TestValidarSinCredencialesProxyEnClaro_RechazaSecretosNoVacios(t *testing.T) {
	t.Parallel()

	datos := map[string]any{
		"proxyEnabled":  true,
		"proxySecretId": "secret-1",
		"proxyPassword": "supersecreta",
	}
	err := ValidarSinCredencialesProxyEnClaro(datos)
	if err == nil {
		t.Fatal("se esperaba rechazo de proxyPassword en claro")
	}
	if err != ErrProxySecretEnClaro {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestValidarSinCredencialesProxyEnClaro_LimpiaClavesVacias(t *testing.T) {
	t.Parallel()

	datos := map[string]any{
		"proxyUsername": "   ",
		"proxyPassword": "",
		"proxySecretId": "secret-1",
	}
	if err := ValidarSinCredencialesProxyEnClaro(datos); err != nil {
		t.Fatalf("ValidarSinCredencialesProxyEnClaro() error = %v", err)
	}
	if _, ok := datos["proxyUsername"]; ok {
		t.Fatalf("proxyUsername vacio debio eliminarse: %#v", datos)
	}
	if _, ok := datos["proxyPassword"]; ok {
		t.Fatalf("proxyPassword vacio debio eliminarse: %#v", datos)
	}
	if datos["proxySecretId"] != "secret-1" {
		t.Fatalf("proxySecretId no debe tocarse: %#v", datos)
	}
}
