// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"bytes"
	"encoding/json"
	"net/url"
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

func TestCuerpoConSecretosEsJSONValidoYEscapa(t *testing.T) {
	base := struct {
		CredentialID string `json:"credentialID"`
	}{"c-1"}
	pin := []byte("12\"3\\4\x01")
	otp := []byte("ñ567")
	cuerpo, err := cuerpoConSecretos(base, []campoSecreto{{"PIN", pin}, {"OTP", otp}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]string
	if err := json.Unmarshal(cuerpo, &v); err != nil {
		t.Fatalf("JSON inválido %q: %v", cuerpo, err)
	}
	if v["credentialID"] != "c-1" || v["PIN"] != string(pin) || v["OTP"] != string(otp) {
		t.Fatalf("valores: %v", v)
	}
	cuerpo, err = cuerpoConSecretos(base, nil, []datoAuthSecreto{{"PIN", pin}, {"OTP", otp}})
	if err != nil {
		t.Fatal(err)
	}
	var w struct {
		AuthData []struct{ ID, Value string } `json:"authData"`
	}
	if err := json.Unmarshal(cuerpo, &w); err != nil || len(w.AuthData) != 2 || w.AuthData[0].Value != string(pin) || w.AuthData[1].ID != "OTP" {
		t.Fatalf("authData %q: %v %+v", cuerpo, err, w)
	}
	vacio, err := cuerpoConSecretos(struct{}{}, []campoSecreto{{"SAD", []byte("s")}}, nil)
	if err != nil || string(vacio) != `{"SAD":"s"}` {
		t.Fatalf("objeto vacío: %q %v", vacio, err)
	}
}

func TestAnadirPorcentajeEsFormularioValido(t *testing.T) {
	valor := []byte("a b+c/=&ñ~")
	cuerpo := append([]byte("token="), anadirPorcentaje(nil, valor)...)
	q, err := url.ParseQuery(string(cuerpo))
	if err != nil || q.Get("token") != string(valor) {
		t.Fatalf("%q -> %v %v", cuerpo, q, err)
	}
}
