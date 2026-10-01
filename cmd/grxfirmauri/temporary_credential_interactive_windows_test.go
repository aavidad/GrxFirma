// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows && fyne_gui && amd64

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desksigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

// Sonda opt-in para UIA/operador: solo firma datos sintéticos después de
// comprobar que el fichero elegido es exactamente el P12 QA autorizado.
func TestNativeInteractiveTemporaryCredentialImport(t *testing.T) {
	if os.Getenv("GRXFIRMA_NATIVE_UI_PROBE") != "1" {
		t.Skip("sonda interactiva Windows desactivada")
	}
	expected := strings.ToLower(strings.TrimSpace(os.Getenv("GRXFIRMA_NATIVE_QA_P12_SHA256")))
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		t.Fatal("Debe fijarse la huella SHA-256 del P12 QA autorizado antes de abrir el selector.")
	}
	if !enableNativeProtocolUIFallback(errors.New("sonda de carga temporal QA")) {
		t.Fatal("No se pudo activar la interfaz nativa de pruebas.")
	}
	defer disableNativeProtocolUIFallback()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	data, password, err := solicitarCredencialTemporal(ctx)
	defer clear(data)
	defer clear(password)
	if err != nil {
		t.Fatal("La selección local del P12 QA y su contraseña no se completó.")
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != expected {
		t.Fatal("El archivo seleccionado no coincide con el P12 QA autorizado; no se ha firmado.")
	}
	selector, _, _ := nuevoSelectorCredenciales(nil, certificateCatalogStub{}, keyProviderStub{}, nil, nil)
	defer selector.ClearCredentials()
	if err := selector.cargar(ctx, data, password); err != nil {
		t.Fatal("El P12 QA seleccionado no se pudo cargar como credencial de firma vigente.")
	}
	clear(data)
	clear(password)
	refs, err := selector.temporal.List(ctx)
	if err != nil || len(refs) != 1 {
		t.Fatal("La carga no produjo una única identidad temporal.")
	}
	key, err := selector.temporal.KeyFor(ctx, refs[0])
	if err != nil {
		t.Fatal("No se pudo resolver la identidad temporal QA.")
	}
	doc, err := domain.NewDocument("qa-carga-p12.txt", []byte("Prueba sintética de carga temporal de P12 mediante la interfaz nativa de GrxFirma."), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	result, err := desksigner.NuevoMotorFirmaGo(relojReal{}).Sign(ctx, domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}, key)
	if err != nil {
		t.Fatal("La identidad QA no pudo producir la firma CAdES sintética.")
	}
	verification, _, err := commonsigner.NewCAdESVerifier().VerifyDetachedCMS(ctx, result.Data, doc.Content)
	if err != nil || verification.Integrity.Status != domain.VerificationStatusValid {
		t.Fatal("La firma CAdES sintética no superó la verificación de integridad.")
	}
	selector.ClearCredentials()
	remaining, _ := selector.temporal.List(ctx)
	if len(remaining) != 0 {
		t.Fatal("La identidad QA no fue retirada tras la firma.")
	}
	t.Log("P12 QA autorizado: selector y contraseña nativos, carga temporal, firma CAdES, integridad válida y retirada de credencial completados.")
}
