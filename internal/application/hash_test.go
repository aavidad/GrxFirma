// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"encoding/base64"
	"testing"

	"grxfirma/internal/application"
)

func TestCreateHashUseCase_SHA256Hex(t *testing.T) {
	uc := application.NuevoCreateHashUseCase()
	res, err := uc.Execute(context.Background(), application.CreateHashCommand{
		Data:      []byte("hola"),
		Algorithm: "SHA-256",
		Format:    application.HashFormatHex,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if res.Algorithm != "SHA-256" {
		t.Fatalf("algoritmo inesperado: %q", res.Algorithm)
	}
	if res.Encoded == "" || res.Encoded[len(res.Encoded)-1] != 'h' {
		t.Fatalf("salida HEX inesperada: %q", res.Encoded)
	}
}

func TestCheckHashUseCase_ValidBase64(t *testing.T) {
	ucCreate := application.NuevoCreateHashUseCase()
	created, err := ucCreate.Execute(context.Background(), application.CreateHashCommand{
		Data:      []byte("hola"),
		Algorithm: "SHA-256",
		Format:    application.HashFormatBase64,
	})
	if err != nil {
		t.Fatalf("create error = %v", err)
	}
	expected, err := base64.StdEncoding.DecodeString(created.Encoded)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	ucCheck := application.NuevoCheckHashUseCase()
	checked, err := ucCheck.Execute(context.Background(), application.CheckHashCommand{
		Data:         []byte("hola"),
		ExpectedHash: expected,
		Algorithm:    "SHA-256",
		Format:       application.HashFormatBase64,
	})
	if err != nil {
		t.Fatalf("check error = %v", err)
	}
	if !checked.Valid {
		t.Fatal("se esperaba huella valida")
	}
}

func TestParseStoredHash_Hex(t *testing.T) {
	digest, alg, format, err := application.ParseStoredHash([]byte("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824h"), "fichero.hexhash")
	if err != nil {
		t.Fatalf("ParseStoredHash() error = %v", err)
	}
	if alg != "SHA-256" {
		t.Fatalf("algoritmo inesperado: %q", alg)
	}
	if format != application.HashFormatHex {
		t.Fatalf("formato inesperado: %q", format)
	}
	if len(digest) != 32 {
		t.Fatalf("digest len inesperada: %d", len(digest))
	}
}
