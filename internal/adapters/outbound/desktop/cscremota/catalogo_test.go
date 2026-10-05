// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cscremota_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/adapters/outbound/desktop/cscremota"
)

// TestCadaCodigoCSCTieneTextoEnTodosLosIdiomas: la interfaz traduce cada
// código con «csc.error.<código>»; un código sin texto se vería como clave.
func TestCadaCodigoCSCTieneTextoEnTodosLosIdiomas(t *testing.T) {
	codigos := []csc.Codigo{
		csc.CodigoURLInvalida, csc.CodigoSoloHTTPS, csc.CodigoRedireccion, csc.CodigoRespuestaGrande,
		csc.CodigoRespuestaInvalida, csc.CodigoServicio, csc.CodigoRed, csc.CodigoSinOAuth,
		csc.CodigoOAuthOtroHost, csc.CodigoAutorizacionDenegada, csc.CodigoAutorizacionCaducada,
		csc.CodigoNavegador, csc.CodigoCredencialNoValida, csc.CodigoAlgoritmoNoSoportado,
		csc.CodigoFirmaInvalida, csc.CodigoSecreto, csc.CodigoSesionCerrada, csc.CodigoParametroInvalido,
		csc.CodigoDemasiadasCredenciales, csc.CodigoSesionCaducada, csc.CodigoOAuthMetadatos,
		csc.CodigoLoteRepetido, csc.CodigoLoteMixto, csc.CodigoOTPLoteExcede,
		cscremota.CodigoDesactivada, cscremota.CodigoProhibida, cscremota.CodigoNoConfigurada,
		cscremota.CodigoNoConectada, cscremota.CodigoParesOAuthInvalidos, cscremota.CodigoOTPLote,
		cscremota.CodigoSecretoNoPedido,
	}
	ficheros, err := filepath.Glob(filepath.Join("..", "..", "common", "localizador", "locales", "*.json"))
	if err != nil || len(ficheros) != 11 {
		t.Fatalf("catálogos = %d, %v", len(ficheros), err)
	}
	for _, f := range ficheros {
		datos, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var catalogo map[string]string
		if err := json.Unmarshal(datos, &catalogo); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, c := range codigos {
			if catalogo["csc.error."+string(c)] == "" {
				t.Errorf("%s: falta csc.error.%s", filepath.Base(f), c)
			}
		}
	}
}
