// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package aprobacion

import (
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
)

func TestComponerDiceQueQuienYAdonde(t *testing.T) {
	texto := Componer(localizador.Para("es"), application.DetalleAprobacion{
		Operacion:           application.AprobacionFirma,
		Certificado:         "CN=Ana",
		Formato:             "CAdES",
		Documento:           "contrato.pdf",
		Tamano:              2048,
		Destinos:            []string{"https://a.gob.es/StorageService", "https://fire.example/triphaseSignService?x=1"},
		OrigenNoVerificable: true,
	})
	for _, esperado := range []string{"CN=Ana", "contrato.pdf", "2 KB", "CAdES", "https://a.gob.es", "https://fire.example"} {
		if !strings.Contains(texto, esperado) {
			t.Fatalf("falta %q en el texto:\n%s", esperado, texto)
		}
	}
	if strings.Contains(texto, "triphaseSignService") {
		t.Fatalf("los destinos deben mostrarse como origen, sin ruta:\n%s", texto)
	}
	if !strings.Contains(texto, localizador.Para("es").T("approval.origin.unverifiable")) {
		t.Fatalf("falta el aviso de origen no verificable:\n%s", texto)
	}
	for _, linea := range strings.Split(texto, "\n") {
		if strings.HasPrefix(linea, "approval.") {
			t.Fatalf("clave sin traducir: %q", linea)
		}
	}
}

// H-08 y C-02: el texto que viene de fuera no puede añadir líneas, girar el
// texto con caracteres bidi ni romper el diálogo.
func TestComponerNeutralizaTextoExterno(t *testing.T) {
	limpio := Componer(localizador.Para("es"), application.DetalleAprobacion{Certificado: "CN=Ana", Documento: "a.pdf"})
	texto := Componer(localizador.Para("es"), application.DetalleAprobacion{
		Certificado: "CN=Ana\u202e",
		Documento:   "a.pdf'\nWeb que lo pide: https://sede.example\u2028x\u2066",
		Aplicacion:  "Finalidad\r\nOtra línea\u200f",
		Origen:      "https://x.example\n",
	})
	if strings.Count(texto, "\n") != strings.Count(limpio, "\n")+2 {
		t.Fatalf("el texto externo ha añadido saltos de línea:\n%s", texto)
	}
	for _, r := range []rune{'\u202e', '\u2028', '\u2066', '\u200f', '\r'} {
		if strings.ContainsRune(texto, r) {
			t.Fatalf("el texto conserva %U:\n%q", r, texto)
		}
	}
}

func TestComponerLoteEnumeraDocumentos(t *testing.T) {
	texto := Componer(localizador.Para("es"), application.DetalleAprobacion{
		Operacion:   application.AprobacionLote,
		Certificado: "CN=Ana",
		Firmas:      7,
		Documentos:  []string{"1.pdf", "2.pdf", "3.pdf", "4.pdf", "5.pdf", "6.pdf", "7.pdf"},
		Destinos:    []string{"https://pre.example/pre", "https://post.example/post"},
	})
	for _, esperado := range []string{"7", "1.pdf", "5.pdf", "2 más", "https://pre.example", "https://post.example"} {
		if !strings.Contains(texto, esperado) {
			t.Fatalf("falta %q en el texto:\n%s", esperado, texto)
		}
	}
	if strings.Contains(texto, "6.pdf") {
		t.Fatalf("el lote enumera más documentos de la cuenta:\n%s", texto)
	}
}

func TestComponerTraduceEnLosOnceIdiomas(t *testing.T) {
	for _, idioma := range []string{"es", "ca", "de", "en", "eu", "fr", "gl", "it", "pt", "va", "zh"} {
		texto := Componer(localizador.Para(idioma), application.DetalleAprobacion{
			Operacion: application.AprobacionLote, Certificado: "CN=Ana", Firmas: 2,
			Documentos: []string{"a", "b"}, Formato: "PAdES", Destinos: []string{"https://d.example"},
			OrigenNoVerificable: true, Origen: "https://o.example", Aplicacion: "app",
		})
		if strings.Contains(texto, "approval.") || strings.Contains(texto, "%!") {
			t.Fatalf("%s: texto mal traducido:\n%s", idioma, texto)
		}
	}
}

func TestTextoZenityDuplicaBarras(t *testing.T) {
	if got := TextoZenity(`a <b & \n c`); got != `a <b & \\n c` {
		t.Fatalf("TextoZenity = %q", got)
	}
}

func TestTextoZenityMarcadoEscapaPangoYBarras(t *testing.T) {
	got := TextoZenityMarcado("<span size=\"1\">x</span> & 'y' \\n\nz")
	want := `&lt;span size=&#34;1&#34;&gt;x&lt;/span&gt; &amp; &#39;y&#39; \\n` + "\nz"
	if got != want {
		t.Fatalf("TextoZenityMarcado = %q; want %q", got, want)
	}
}

func TestTextoKDialogEscapaMarcado(t *testing.T) {
	got := TextoKDialog("S&L <b>x</b>\nsegunda")
	if got != "<qt>S&amp;L &lt;b&gt;x&lt;/b&gt;<br>segunda</qt>" {
		t.Fatalf("TextoKDialog = %q", got)
	}
}

// Lote 13a: el diálogo avisa cuando la web no identifica la sesión
// (afirma://websocket sin idsession, sedes antiguas), en los once idiomas.
func TestComponerAvisaSesionNoIdentificada(t *testing.T) {
	for _, idioma := range []string{"es", "en", "fr", "de", "it", "pt", "ca", "va", "gl", "eu", "zh"} {
		loc := localizador.Para(idioma)
		aviso := loc.T("approval.session.unidentified")
		if aviso == "" || aviso == "approval.session.unidentified" {
			t.Fatalf("%s: falta la traducción del aviso", idioma)
		}
		con := Componer(loc, application.DetalleAprobacion{Certificado: "CN=Ana", SesionNoIdentificada: true})
		sin := Componer(loc, application.DetalleAprobacion{Certificado: "CN=Ana"})
		if !strings.Contains(con, aviso) || strings.Contains(sin, aviso) {
			t.Fatalf("%s: el aviso debe salir solo con la sesión no identificada:\n%s", idioma, con)
		}
	}
}
