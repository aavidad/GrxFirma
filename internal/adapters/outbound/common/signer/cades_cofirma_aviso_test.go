// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"slices"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
)

// KD-03: en escritorio, CLI y afirma:// la cofirma y la contrafirma de una
// CAdES explícita siguen funcionando (retrocompatibilidad), pero ya no en
// silencio: quien firma recibe el aviso de que no ha visto el documento.
func TestCAdES_CofirmaExplicitaAvisaDeQueNoSeHaVistoElDocumento(t *testing.T) {
	titulo := localizador.Detectar().T("firma.cofirma_sin_original.aviso_titulo")
	explicita, _ := firmarCAdESParaPrueba(t, []byte("documento que no se enseña"), domain.ActionSign, "Firmante A", nil)
	implicita, _ := firmarCAdESParaPrueba(t, []byte("documento dentro"), domain.ActionSign, "Firmante A",
		map[string]string{"mode": "implicit"})
	for _, caso := range []struct {
		nombre string
		firma  []byte
		accion domain.SignatureAction
		aviso  bool
	}{
		{"cofirma_explicita", explicita, domain.ActionCoSign, true},
		{"contrafirma_explicita", explicita, domain.ActionCounterSign, true},
		{"cofirma_implicita", implicita, domain.ActionCoSign, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			avisos.Reiniciar()
			defer avisos.Reiniciar()
			firmarCAdESParaPrueba(t, caso.firma, caso.accion, "Cofirmante B", nil)
			hay := false
			for _, a := range avisos.Todos() {
				hay = hay || a.Titulo == titulo
			}
			if hay != caso.aviso {
				t.Fatalf("aviso = %v, want %v (avisos=%q)", hay, caso.aviso, avisos.Texto())
			}
		})
	}
}

// Lote 16d: el mismo aviso llega al resultado de la firma por el contexto,
// para que Qt y WinUI lo enseñen tras firmar en el idioma de la persona.
func TestCAdES_CofirmaExplicitaAnotaElAvisoEnElResultado(t *testing.T) {
	explicita, _ := firmarCAdESParaPrueba(t, []byte("documento que no se enseña"), domain.ActionSign, "Firmante A", nil)
	implicita, _ := firmarCAdESParaPrueba(t, []byte("documento dentro"), domain.ActionSign, "Firmante A",
		map[string]string{"mode": "implicit"})
	priv, cert := certForTest(t, "Cofirmante B")
	clave := &LocalSigningKey{ID: "b", Signer: priv, Certificate: cert}
	for _, caso := range []struct {
		nombre string
		firma  []byte
		accion domain.SignatureAction
		aviso  bool
	}{
		{"cofirma_explicita", explicita, domain.ActionCoSign, true},
		{"contrafirma_explicita", explicita, domain.ActionCounterSign, true},
		{"cofirma_implicita", implicita, domain.ActionCoSign, false},
		{"firma_simple", []byte("documento"), domain.ActionSign, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			doc, err := domain.NewDocument("datos.bin", caso.firma, "application/octet-stream")
			if err != nil {
				t.Fatal(err)
			}
			ctx, rec := avisos.ConRecolector(context.Background())
			if _, err := NewCAdESBESDetached().Sign(ctx, domain.SignatureJob{
				Document: doc, Format: domain.FormatCAdES, Action: caso.accion,
			}, clave); err != nil {
				t.Fatal(err)
			}
			want := []string(nil)
			if caso.aviso {
				want = []string{AvisoCofirmaSinOriginal}
			}
			if got := rec.Claves(); !slices.Equal(got, want) {
				t.Fatalf("avisos = %q, want %q", got, want)
			}
		})
	}
}
