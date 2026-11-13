// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// H-08: el nombre del documento lo pone el cliente; no puede añadir líneas al
// diálogo (por ejemplo, un "Origen solicitante" falso).
func TestMensajeAprobacionNeutralizaNombre(t *testing.T) {
	aprobador := &aprobadorMock{aprobado: true}
	uc := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		&motorMock{resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")}},
		aprobador, nil, nil,
	)
	doc, err := domain.NewDocument("a.pdf'\nOrigen solicitante: https://x\u202e", []byte("contenido"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Ejecutar(context.Background(), application.SignCommand{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}); err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if n := strings.Count(aprobador.mensaje, "\n"); n != 0 {
		t.Fatalf("el nombre ha añadido %d saltos de línea: %q", n, aprobador.mensaje)
	}
	if strings.ContainsRune(aprobador.mensaje, '\u202e') {
		t.Fatalf("el mensaje conserva U+202E: %q", aprobador.mensaje)
	}
}

// C-02: los caracteres de formato bidi no pasan al diálogo.
func TestContextoAprobacionSeguroEliminaBidi(t *testing.T) {
	for _, r := range []rune{'\u200e', '\u200f', '\u202a', '\u202e', '\u2066', '\u2069', ' ', ' ', '\u200b'} {
		if got := application.NeutralizarTextoAprobacion("x"+string(r)+"y", 50); strings.ContainsRune(got, r) {
			t.Fatalf("NeutralizarTextoAprobacion conserva %U: %q", r, got)
		}
	}
	if got := application.NeutralizarTextoAprobacion("abcdef", 3); got != "abc…" {
		t.Fatalf("recorte = %q", got)
	}
}

// El caso de uso entrega al componedor común todo lo que hay que enseñar.
func TestSignDocumentUsaElComponedorDeAprobacion(t *testing.T) {
	aprobador := &aprobadorMock{aprobado: true}
	var visto application.DetalleAprobacion
	uc := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		&motorMock{resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")}},
		aprobador, nil, nil,
	).WithComponedorAprobacion(func(d application.DetalleAprobacion) string {
		visto = d
		return "TEXTO-COMUN"
	})
	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(), Format: domain.FormatPAdES, Action: domain.ActionCoSign,
		RequesterOrigin: "https://sede.example",
	})
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if aprobador.mensaje != "TEXTO-COMUN" {
		t.Fatalf("mensaje = %q", aprobador.mensaje)
	}
	if visto.Operacion != application.AprobacionCofirma || visto.Formato != string(domain.FormatPAdES) ||
		visto.Documento != docPrueba().Name || visto.Tamano != len(docPrueba().Content) || visto.Origen != "https://sede.example" {
		t.Fatalf("detalle = %+v", visto)
	}
}

// H-03: la aprobación del lote nombra los documentos, el origen y los destinos.
func TestProcessBatchMensajeIncluyeDocumentos(t *testing.T) {
	aprobador := &aprobadorMock{aprobado: true}
	var visto application.DetalleAprobacion
	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		&motorMock{resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")}},
		aprobador, nil, nil,
	).WithComponedorAprobacion(func(d application.DetalleAprobacion) string {
		visto = d
		return "LOTE"
	})
	docA, _ := domain.NewDocument("a.pdf", []byte("a"), "application/pdf")
	docB, _ := domain.NewDocument("b.pdf", []byte("b"), "application/pdf")
	_, err := uc.Ejecutar(context.Background(), application.ProcessBatchCommand{
		Jobs: []domain.SignatureJob{
			{Document: docA, Format: domain.FormatCAdES, Action: domain.ActionSign},
			{Document: docB, Format: domain.FormatCAdES, Action: domain.ActionSign},
		},
		RequesterOrigin: "https://sede.example",
	})
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if visto.Operacion != application.AprobacionLote || visto.Firmas != 2 || len(visto.Documentos) != 2 ||
		visto.Documentos[1] != "b.pdf" || visto.Origen != "https://sede.example" || visto.Formato != string(domain.FormatCAdES) {
		t.Fatalf("detalle = %+v", visto)
	}
}
