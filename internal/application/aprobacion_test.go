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

// Lote 13a: una firma o un lote que llegan por un canal afirma://websocket
// lanzado sin idsession lo dicen en el detalle de la aprobación.
func TestAprobacionMarcaSesionNoIdentificada(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), application.ConSesionNoIdentificada(context.Background())} {
		quiere := application.SesionNoIdentificada(ctx)
		var firma, lote application.DetalleAprobacion
		ucFirma := application.NuevoSignDocumentUseCase(
			&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
			&proveedorClavesMock{clave: claveMock{"clave-001"}},
			&motorMock{resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")}},
			&aprobadorMock{aprobado: true}, nil, nil,
		).WithComponedorAprobacion(func(d application.DetalleAprobacion) string { firma = d; return "F" })
		if _, err := ucFirma.Ejecutar(ctx, application.SignCommand{Document: docPrueba(), Format: domain.FormatCAdES, Action: domain.ActionSign}); err != nil {
			t.Fatalf("firma: %v", err)
		}
		ucLote := application.NuevoProcessBatchUseCase(
			&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
			&proveedorClavesMock{clave: claveMock{"clave-001"}},
			&motorMock{resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")}},
			&aprobadorMock{aprobado: true}, nil, nil,
		).WithComponedorAprobacion(func(d application.DetalleAprobacion) string { lote = d; return "L" })
		if _, err := ucLote.Ejecutar(ctx, application.ProcessBatchCommand{Jobs: []domain.SignatureJob{{Document: docPrueba(), Format: domain.FormatCAdES, Action: domain.ActionSign}}}); err != nil {
			t.Fatalf("lote: %v", err)
		}
		if firma.SesionNoIdentificada != quiere || lote.SesionNoIdentificada != quiere {
			t.Fatalf("contexto marcado=%v: firma=%v lote=%v", quiere, firma.SesionNoIdentificada, lote.SesionNoIdentificada)
		}
	}
}

// Lote 13c (A-03): la marca de destino http de intranet llega al detalle de
// la aprobación de la firma y del lote.
func TestAprobacionMarcaDestinoSinCifrar(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), application.ConDestinoSinCifrar(context.Background())} {
		quiere := application.DestinoSinCifrar(ctx)
		var firma, lote application.DetalleAprobacion
		ucFirma := application.NuevoSignDocumentUseCase(
			&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
			&proveedorClavesMock{clave: claveMock{"clave-001"}},
			&motorMock{resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")}},
			&aprobadorMock{aprobado: true}, nil, nil,
		).WithComponedorAprobacion(func(d application.DetalleAprobacion) string { firma = d; return "F" })
		if _, err := ucFirma.Ejecutar(ctx, application.SignCommand{Document: docPrueba(), Format: domain.FormatCAdES, Action: domain.ActionSign}); err != nil {
			t.Fatalf("firma: %v", err)
		}
		ucLote := application.NuevoProcessBatchUseCase(
			&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
			&proveedorClavesMock{clave: claveMock{"clave-001"}},
			&motorMock{resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")}},
			&aprobadorMock{aprobado: true}, nil, nil,
		).WithComponedorAprobacion(func(d application.DetalleAprobacion) string { lote = d; return "L" })
		if _, err := ucLote.Ejecutar(ctx, application.ProcessBatchCommand{Jobs: []domain.SignatureJob{{Document: docPrueba(), Format: domain.FormatCAdES, Action: domain.ActionSign}}}); err != nil {
			t.Fatalf("lote: %v", err)
		}
		if firma.DestinoSinCifrar != quiere || lote.DestinoSinCifrar != quiere {
			t.Fatalf("contexto marcado=%v: firma=%v lote=%v", quiere, firma.DestinoSinCifrar, lote.DestinoSinCifrar)
		}
	}
}

func TestEsDestinoSinCifrar(t *testing.T) {
	for raw, quiere := range map[string]bool{
		"http://intranet.example:8080/x": true,
		"http://10.0.0.5":                true,
		"https://sede.example":           false,
		"http://127.0.0.1:8080":          false,
		"http://localhost":               false,
		"http://[::1]:80":                false,
		"no es una url":                  false,
	} {
		if got := application.EsDestinoSinCifrar(raw); got != quiere {
			t.Fatalf("EsDestinoSinCifrar(%q) = %v", raw, got)
		}
	}
}
