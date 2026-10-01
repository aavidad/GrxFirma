// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func firmarXAdESParaWrapping(t *testing.T) string {
	t.Helper()
	priv, cert := certForTest(t, "XAdES-Wrapping")
	doc, err := domain.NewDocument("contrato.xml", []byte("<contrato>original</contrato>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	resultado, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "clave-xades", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("firmando XAdES: %v", err)
	}
	return string(resultado.Data)
}

func signedPropertiesLiteral(t *testing.T, firmado string) string {
	t.Helper()
	inicio := strings.Index(firmado, "<xades:SignedProperties")
	fin := strings.Index(firmado, "</xades:SignedProperties>")
	if inicio < 0 || fin < inicio {
		t.Fatal("la firma de prueba no contiene xades:SignedProperties")
	}
	return firmado[inicio : fin+len("</xades:SignedProperties>")]
}

func verificarXAdES(t *testing.T, contenido string) (domain.VerificationResult, error) {
	t.Helper()
	doc, err := domain.NewDocument("firma.xsig", []byte(contenido), "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	return vr, err
}

// Un segundo elemento con el mismo Id permite que el verificador y el visor
// resuelvan nodos distintos: debe rechazarse.
func TestXAdESVerifier_RechazaIdDuplicado(t *testing.T) {
	firmado := firmarXAdESParaWrapping(t)
	copia := signedPropertiesLiteral(t, firmado)
	manipulado := strings.Replace(firmado, "</ds:Signature>", copia+"</ds:Signature>", 1)

	vr, err := verificarXAdES(t, manipulado)
	if err == nil && vr.Valid {
		t.Fatal("una firma con Id duplicado no debe verificarse como válida")
	}
}

// El SignedProperties que se usa para comprobar el certificado debe ser el
// referenciado por SignedInfo, no el primer literal del documento.
func TestXAdESVerifier_IgnoraSignedPropertiesNoReferenciado(t *testing.T) {
	firmado := firmarXAdESParaWrapping(t)
	original := signedPropertiesLiteral(t, firmado)
	senuelo := strings.Replace(original, `Id="`, `Id="senuelo-`, 1)
	if i := strings.Index(senuelo, "<ds:DigestValue>"); i >= 0 {
		j := strings.Index(senuelo[i:], "</ds:DigestValue>")
		senuelo = senuelo[:i] + "<ds:DigestValue>AAAA</ds:DigestValue>" + senuelo[i+j+len("</ds:DigestValue>"):]
	}
	inicio := strings.Index(firmado, "<ds:Signature")
	cierre := strings.Index(firmado[inicio:], ">") + inicio + 1
	manipulado := firmado[:cierre] + "<ds:Object>" + senuelo + "</ds:Object>" + firmado[cierre:]

	vr, err := verificarXAdES(t, manipulado)
	if err != nil {
		t.Fatalf("el señuelo no referenciado no debe impedir verificar la firma real: %v", err)
	}
	if !vr.Valid {
		t.Fatalf("se esperaba firma válida con el SignedProperties referenciado: %s", vr.Reason)
	}
}

func TestLiteralElementOffset_ExigeNombreCompleto(t *testing.T) {
	fuente := `<raiz><CONTENTX Id="a"/><CONTENT Id="b">x</CONTENT></raiz>`
	if got, want := literalElementOffset(fuente, "CONTENT"), strings.Index(fuente, `<CONTENT Id="b"`); got != want {
		t.Fatalf("literalElementOffset = %d, want %d", got, want)
	}
}

func TestResolveDetachedContent_RechazaLiteralEnComentario(t *testing.T) {
	fuente := `<raiz><!-- <CONTENT Name="doc">ZmFsc28=</CONTENT> --><CONTENT Name="doc">cmVhbA==</CONTENT></raiz>`
	if _, err := resolveDetachedContent([]byte(fuente), "doc"); err == nil {
		t.Fatal("un CONTENT dentro de un comentario no debe confundirse con el real")
	}
	limpio := `<raiz><CONTENT Name="doc">cmVhbA==</CONTENT></raiz>`
	datos, err := resolveDetachedContent([]byte(limpio), "doc")
	if err != nil || string(datos) != "real" {
		t.Fatalf("resolveDetachedContent = %q, %v", datos, err)
	}
}

func TestResolveReferencedElement_RechazaIdDuplicado(t *testing.T) {
	fuente := `<raiz><a Id="x">1</a><b Id="x">2</b></raiz>`
	if _, err := resolveReferencedElement([]byte(fuente), "x"); err == nil {
		t.Fatal("se esperaba error con Id duplicado")
	}
}

// FuzzXAdESVerifier garantiza que ninguna entrada hace entrar en pánico al
// verificador y que nada se declara válido sin firma verificable.
func FuzzXAdESVerifier(f *testing.F) {
	f.Add([]byte(`<a/>`))
	f.Add([]byte(`<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:SignedInfo/></ds:Signature>`))
	f.Add([]byte(`<raiz><!-- <CONTENT Name="doc">x</CONTENT> --><CONTENT Name="doc">eA==</CONTENT></raiz>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := domain.NewDocument("f.xsig", data, "application/xml")
		if err != nil {
			return
		}
		vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
		if err == nil && vr.Valid && !strings.Contains(string(data), "SignatureValue") {
			t.Fatalf("documento sin SignatureValue declarado válido: %q", data)
		}
	})
}

func TestUsesSHA1_DetectaMetodoYDigest(t *testing.T) {
	const ds = `xmlns="http://www.w3.org/2000/09/xmldsig#"`
	casos := map[string]bool{
		`<SignedInfo ` + ds + `><SignatureMethod Algorithm="http://www.w3.org/2000/09/xmldsig#rsa-sha1"/></SignedInfo>`:                                                                                                         true,
		`<SignedInfo ` + ds + `><SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"/><Reference URI=""><DigestMethod Algorithm="http://www.w3.org/2000/09/xmldsig#sha1"/></Reference></SignedInfo>`:  true,
		`<SignedInfo ` + ds + `><SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"/><Reference URI=""><DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/></Reference></SignedInfo>`: false,
	}
	for entrada, esperado := range casos {
		if got := usesSHA1([]byte(entrada)); got != esperado {
			t.Errorf("usesSHA1(%q) = %v, want %v", entrada, got, esperado)
		}
	}
}
