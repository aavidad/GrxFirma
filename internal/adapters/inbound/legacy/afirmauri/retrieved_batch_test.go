// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"crypto/des"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/legacycrypto"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

func TestResolveRetrievedBatch_EnvelopeRemoto(t *testing.T) {
	payload := `{"singlesigns":[{"id":"1","datareference":"token"}]}`
	raw := []byte(
		`<batch>` +
			`<e k="jsonbatch" v="true"/>` +
			`<e k="batchpresignerurl" v="https%3A%2F%2Fpre.example%2Fbatch"/>` +
			`<e k="batchpostsignerurl" v="https%3A%2F%2Fpost.example%2Fbatch"/>` +
			`<e k="stservlet" v="https%3A%2F%2Fstore.example%2FStorageService"/>` +
			`<e k="rtservlet" v="https%3A%2F%2Fretrieve.example%2FRetrieveService"/>` +
			`<e k="id" v="req-xml"/>` +
			`<e k="needcert" v="true"/>` +
			`<e k="dat" v="` + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(payload))) + `"/>` +
			`</batch>`,
	)

	remote, cmd, origenes, err := ResolveRetrievedBatch(raw, domain.ExchangeSession{
		RequestID:        "req-inicial",
		RetrieveEndpoint: "https://retrieve.example/RetrieveService",
		UploadEndpoint:   "https://store.example/StorageService",
		State:            domain.SessionActive,
	})
	if err != nil {
		t.Fatalf("ResolveRetrievedBatch: %v", err)
	}
	if remote == nil {
		t.Fatal("se esperaba RemoteBatchCommand")
	}
	if cmd != nil {
		t.Fatal("no se esperaba ProcessBatchCommand")
	}
	if string(remote.Payload) != payload {
		t.Fatalf("payload inesperado: %s", string(remote.Payload))
	}
	if remote.Session.RequestID != "req-xml" {
		t.Fatalf("request id inesperado: %s", remote.Session.RequestID)
	}
	if len(origenes) != 4 {
		t.Fatalf("origenes inesperados: %#v", origenes)
	}
}

func TestResolveRetrievedBatch_RemotoLegacyDES(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	payload := `{"singlesigns":[{"id":"1","datareference":"` + base64.StdEncoding.EncodeToString([]byte("ABC")) + `"}]}`
	envuelto := base64.StdEncoding.EncodeToString([]byte(payload))
	cifrado, err := cifrarLegacyDESPrueba([]byte(envuelto), "abcd")
	if err != nil {
		t.Fatalf("cifrarLegacyDESPrueba: %v", err)
	}

	remote, cmd, _, err := ResolveRetrievedBatch(cifrado, domain.ExchangeSession{
		RequestID:        "req-batch",
		RetrieveEndpoint: "https://retrieve.example/RetrieveService",
		UploadEndpoint:   "https://store.example/StorageService",
		SessionKey:       "abcd",
		State:            domain.SessionActive,
	})
	if err != nil {
		t.Fatalf("ResolveRetrievedBatch: %v", err)
	}
	if remote != nil {
		t.Fatal("no se esperaba RemoteBatchCommand")
	}
	if cmd == nil {
		t.Fatal("se esperaba ProcessBatchCommand")
	}
	if got := len(cmd.Jobs); got != 1 {
		t.Fatalf("numero de trabajos inesperado: %d", got)
	}
}

func TestDescifrarRetrieveLegacyDES_RequiereOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)
	if _, err := descifrarRetrieveLegacyDES([]byte("0.QUJD"), "abcd"); !errors.Is(err, legacycrypto.ErrDESDisabled) {
		t.Fatalf("descifrarRetrieveLegacyDES() error = %v", err)
	}
}

func cifrarLegacyDESPrueba(datos []byte, claveRaw string) ([]byte, error) {
	clave := []byte(strings.TrimSpace(claveRaw))
	if len(clave) < 8 {
		padded := make([]byte, 8)
		copy(padded, clave)
		clave = padded
	} else if len(clave) > 8 {
		clave = clave[:8]
	}

	block, err := des.NewCipher(clave)
	if err != nil {
		return nil, err
	}

	padding := (block.BlockSize() - len(datos)%block.BlockSize()) % block.BlockSize()
	padded := make([]byte, len(datos)+padding)
	copy(padded, datos)

	cifrado := make([]byte, len(padded))
	for i := 0; i < len(padded); i += block.BlockSize() {
		block.Encrypt(cifrado[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
	}

	payload := base64.StdEncoding.EncodeToString(cifrado)
	payload = strings.ReplaceAll(payload, "+", "-")
	payload = strings.ReplaceAll(payload, "/", "_")
	return []byte(fmt.Sprintf("%d.%s", padding, payload)), nil
}
