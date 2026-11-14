// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"encoding/json"
	"testing"
	"time"
)

// ID-04: expires_in es opcional en OAuth 2.0. Un token que no lo trae (o lo
// trae a cero o negativo) no puede valer mientras viva el proceso: caduca como
// mucho a las 24 horas, igual que uno que declara una vida mayor.
func TestTokenSinExpiresInCaducaA24Horas(t *testing.T) {
	for _, cuerpo := range []string{
		`{"access_token":"tok","token_type":"Bearer"}`,
		`{"access_token":"tok","token_type":"Bearer","expires_in":0}`,
		`{"access_token":"tok","token_type":"Bearer","expires_in":-5}`,
		`{"access_token":"tok","token_type":"Bearer","expires_in":999999999}`,
	} {
		var r respuestaToken
		if err := json.Unmarshal([]byte(cuerpo), &r); err != nil {
			t.Fatal(err)
		}
		antes := time.Now()
		tok, err := tokenDesdeRespuesta(&r)
		if err != nil {
			t.Fatalf("%s: %v", cuerpo, err)
		}
		if tok.caduca.IsZero() {
			t.Fatalf("%s: el token no caduca nunca", cuerpo)
		}
		if !tok.vigente(antes.Add(maxVidaToken - time.Minute)) {
			t.Fatalf("%s: debe seguir vigente antes de 24 h", cuerpo)
		}
		if tok.vigente(antes.Add(maxVidaToken + time.Hour)) {
			t.Fatalf("%s: sigue vigente pasadas 25 h", cuerpo)
		}
		tok.destruir()
	}
}

func TestTokenConExpiresInCortoConservaSuVida(t *testing.T) {
	var r respuestaToken
	if err := json.Unmarshal([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":60}`), &r); err != nil {
		t.Fatal(err)
	}
	tok, err := tokenDesdeRespuesta(&r)
	if err != nil {
		t.Fatal(err)
	}
	defer tok.destruir()
	if tok.vigente(time.Now().Add(2 * time.Minute)) {
		t.Fatal("un token de 60 s no puede seguir vigente a los 2 min")
	}
}
