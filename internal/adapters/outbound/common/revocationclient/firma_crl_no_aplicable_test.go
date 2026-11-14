// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"grxfirma/internal/domain"
)

func servidorCRL(t *testing.T, der []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(der)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Lote 14, retrocompatibilidad al firmar: E1-04 hizo que una CRL auténtica
// pero no aplicable (otra partición, delta) dejara de servir como evidencia.
// Al firmar en CAdES-LT o PAdES-LT con una CA que solo publica CRL así, la
// firma fallaba; en 80e7dd5 se firmaba. Fetch (solo lo usa la firma) vuelve
// a incrustar esa CRL con un aviso en el registro. Check (verificación) sigue
// sin dar «no revocado» con ella.
func TestFetch_FirmaConCRLNoAplicableIncrustaConAviso(t *testing.T) {
	f := newValidationFixture(t)
	srv := servidorCRL(t, crlConExtensiones(t, f, nil, extensionIDPPrueba(t, []string{"http://x/otra.crl"})))
	hoja := hojaConCRLDP(t, f, 401, false, srv.URL)
	client := newValidationClient(t, f.now)

	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	evidencia, err := client.Fetch(ctx, hoja, f.ca)
	if err != nil {
		t.Fatalf("al firmar, una CRL auténtica no aplicable no puede impedir la firma: %v", err)
	}
	if len(evidencia.CRLs) != 1 {
		t.Fatalf("se esperaba la CRL incrustada: %d", len(evidencia.CRLs))
	}
	// Lote 16c: el aviso llega también al resultado de la firma.
	if claves := avisos.Claves(); len(claves) != 1 || claves[0] != domain.AvisoCRLNoAplicable {
		t.Fatalf("falta el aviso de CRL no aplicable en la operación: %q", claves)
	}

	resultado, err := client.Check(context.Background(), hoja, f.ca)
	if err == nil || resultado.Status == CertificateStatusGood {
		t.Fatalf("al verificar, una CRL no aplicable no acredita «no revocado»: estado=%d err=%v", resultado.Status, err)
	}
}

// Si esa CRL lista la serie, la firma se rechaza como siempre.
func TestFetch_FirmaConCRLNoAplicableQueRevocaFalla(t *testing.T) {
	f := newValidationFixture(t)
	hojaProvisional := hojaConCRLDP(t, f, 402, false)
	srv := servidorCRL(t, crlConExtensiones(t, f, hojaProvisional.SerialNumber, extensionIDPPrueba(t, []string{"http://x/otra.crl"})))
	hoja := hojaConCRLDP(t, f, 402, false, srv.URL)
	client := newValidationClient(t, f.now)
	if _, err := client.Fetch(context.Background(), hoja, f.ca); !errors.Is(err, ErrCertificateRevoked) {
		t.Fatalf("un certificado revocado no puede firmar en LT: %v", err)
	}
}
