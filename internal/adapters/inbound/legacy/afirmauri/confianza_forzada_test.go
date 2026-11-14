// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// Lote 14: una web no puede pedir por afirma:// (ni por el WebSocket, que
// usa el mismo intérprete) que se acepte, se quite o se lea la confianza
// forzada. Esas operaciones no existen en el protocolo.
func TestAfirmaURI_NoGestionaConfianzaForzada(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	huella := strings.Repeat("a", 64)
	for _, uri := range []string{
		"afirma://forced_trust_accept?type=ca_desconocida&fingerprint=" + huella,
		"afirma://forced_trust_list",
		"afirma://forced_trust_remove?type=algoritmo_sha1",
		"afirma://aceptar-confianza?tipo-confianza=algoritmo_sha1&acepto-el-aviso=true",
		"afirma://listar-confianza-aceptada",
		"afirma://quitar-confianza-aceptada?tipo-confianza=revocacion_no_comprobable",
		"afirma://?op=forced_trust_accept&type=revocacion_no_comprobable",
		"afirma://?op=aceptar-confianza&tipo-confianza=algoritmo_sha1",
	} {
		if s, err := adaptador.Parse(context.Background(), uri); err == nil {
			t.Fatalf("afirma:// aceptó %q como operación %q", uri, s.Operacion)
		}
		if s, err := adaptador.ParseSocket(context.Background(), uri); err == nil {
			t.Fatalf("WebSocket aceptó %q como operación %q", uri, s.Operacion)
		}
	}
	for _, op := range []string{"forced_trust_accept", "aceptar-confianza", "confianza_forzada", "forced-trust"} {
		if _, _, _, err := normalizarOperacion(op); err == nil {
			t.Fatalf("la operación %q no puede existir en afirma://", op)
		}
	}
}
