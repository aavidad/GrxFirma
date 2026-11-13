// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal

// Prueba de regresión de E2-05: un sello de tiempo con millones de niveles de
// anidamiento BER agotaba la pila en ber2der de digitorus/pkcs7, un error
// fatal que tumbaba el proceso. Debe dar «sello no válido».

import (
	"bytes"
	"context"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

// selloAnidado envuelve millones de "30 80" en una SEQUENCE DER de longitud
// definida, para que pase el asn1.Unmarshal previo del sello de documento.
func selloAnidado() []byte {
	interior := bytes.Repeat([]byte{0x30, 0x80}, 8<<20-8)
	l := len(interior)
	// Longitud en tres bytes: DER exige la forma mínima.
	return append([]byte{0x30, 0x83, byte(l >> 16), byte(l >> 8), byte(l)}, interior...)
}

func TestEvaluarSello_AnidamientoProfundoNoTumbaElProceso(t *testing.T) {
	e := Nuevo(Configuracion{})
	aspecto := e.evaluarSello(context.Background(), selloAnidado(), []byte{1}, nil, time.Now())
	if aspecto.Estado != domain.SelloNoValido {
		t.Fatalf("estado %v, se esperaba sello no válido", aspecto.Estado)
	}
}

func TestEvaluarSelloDocumento_AnidamientoProfundoNoTumbaElProceso(t *testing.T) {
	e := Nuevo(Configuracion{})
	_, integridad := e.EvaluarSelloDocumento(context.Background(), selloAnidado(), []byte("contenido"), domain.CertificateChain{}, time.Now())
	if integridad.Estado != domain.IntegridadNoValida {
		t.Fatalf("estado %v, se esperaba integridad no válida", integridad.Estado)
	}
}
