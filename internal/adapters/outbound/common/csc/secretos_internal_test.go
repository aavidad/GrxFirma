// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"bytes"
	"encoding/json"
	"testing"
)

func esCero(b []byte) bool { return len(bytes.Trim(b, "\x00")) == 0 }

func TestTokenYSADDecodificadosSeBorranTambienSiFallan(t *testing.T) {
	var r respuestaToken
	if err := json.Unmarshal([]byte(`{"access_token":"tok-secreto","token_type":"Bearer","expires_in":60}`), &r); err != nil {
		t.Fatal(err)
	}
	vista := r.AccessToken
	tok, err := tokenDesdeRespuesta(&r)
	if err != nil || string(tok.bytes()) != "tok-secreto" {
		t.Fatalf("token: %v", err)
	}
	if !esCero(vista) {
		t.Fatalf("el texto decodificado del token no se borró: %q", vista)
	}
	tok.destruir()

	if err := json.Unmarshal([]byte(`{"access_token":"tok-secreto","token_type":"MAC"}`), &r); err != nil {
		t.Fatal(err)
	}
	vista = r.AccessToken
	if _, err := tokenDesdeRespuesta(&r); err == nil || !esCero(vista) {
		t.Fatalf("con error también debe borrarse: %v %q", err, vista)
	}

	var a respuestaAutorizar
	if err := json.Unmarshal([]byte(`{"SAD":"sad-secreto","expiresIn":300}`), &a); err != nil {
		t.Fatal(err)
	}
	vistaSAD := []byte(a.SADTexto)
	aut, err := sadDesdeRespuesta(&a)
	if err != nil || string(aut.sad.Bytes()) != "sad-secreto" || !esCero(vistaSAD) {
		t.Fatalf("SAD: %v %q", err, vistaSAD)
	}
	aut.destruir()
}
