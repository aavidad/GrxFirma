// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestFacturaE_SignAndVerify(t *testing.T) {
	priv, cert := certForTest(t, "FacturaE-Test")
	xmlData, err := os.ReadFile("testdata/sample-facturae.xml")
	if err != nil {
		t.Fatalf("leyendo factura de ejemplo: %v", err)
	}
	engine := NewFacturaESigner()
	job := domain.SignatureJob{
		Document: domain.Document{Name: "factura.xml", MIMEType: "application/xml", Content: xmlData},
		Format:   formatFacturaE,
		Action:   domain.ActionSign,
	}
	result, err := engine.Sign(context.Background(), job, &LocalSigningKey{Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("Sign(FacturaE) error = %v", err)
	}
	if !looksLikeSignedXML(result.Data) || !strings.Contains(string(result.Data), "<Facturae") {
		t.Fatalf("la salida no parece una factura firmada: %.200s", string(result.Data))
	}
	verify, signers, err := NewFacturaEVerifier().Verify(context.Background(), domain.Document{Name: "factura_firmada.xml", MIMEType: "application/xml", Content: result.Data}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(FacturaE) error = %v", err)
	}
	if !verify.Valid || len(signers) != 1 {
		t.Fatalf("resultado inesperado: %+v signers=%d", verify, len(signers))
	}
}

func TestFacturaE_SignAndVerify_Official322Namespace(t *testing.T) {
	priv, cert := certForTest(t, "FacturaE-322-Test")
	xmlData := []byte(
		`<?xml version="1.0" encoding="UTF-8"?>` +
			`<Facturae xmlns="` + facturaeNamespace322 + `">` +
			`<FileHeader xmlns=""></FileHeader>` +
			`<Parties xmlns=""></Parties>` +
			`<Invoices xmlns=""></Invoices>` +
			`</Facturae>`,
	)
	job := domain.SignatureJob{
		Document: domain.Document{
			Name:     "factura-3.2.2.xml",
			MIMEType: "application/xml",
			Content:  xmlData,
		},
		Format: formatFacturaE,
		Action: domain.ActionSign,
	}

	result, err := NewFacturaESigner().Sign(
		context.Background(),
		job,
		&LocalSigningKey{Signer: priv, Certificate: cert},
	)
	if err != nil {
		t.Fatalf("Sign(FacturaE 3.2.2) error = %v", err)
	}
	verify, signers, err := NewFacturaEVerifier().Verify(
		context.Background(),
		domain.Document{
			Name:     "factura-3.2.2-firmada.xml",
			MIMEType: "application/xml",
			Content:  result.Data,
		},
		domain.CertificateChain{},
	)
	if err != nil {
		t.Fatalf("Verify(FacturaE 3.2.2) error = %v", err)
	}
	if !verify.Valid || len(signers) != 1 {
		t.Fatalf(
			"resultado FacturaE 3.2.2 inesperado: %+v signers=%d",
			verify,
			len(signers),
		)
	}
}

func TestFacturaE_AcceptsOfficial32xNamespaces(t *testing.T) {
	for _, namespace := range []string{
		facturaeNamespace32,
		facturaeNamespace321,
		facturaeNamespace322,
	} {
		xmlData := []byte(
			`<Facturae xmlns="` + namespace + `">` +
				`<FileHeader xmlns=""/>` +
				`<Parties xmlns=""/>` +
				`<Invoices xmlns=""/>` +
				`</Facturae>`,
		)
		if !isFacturaEXML(xmlData) {
			t.Errorf(
				"el namespace oficial Facturae no fue reconocido: %s",
				namespace,
			)
		}
	}
}

func TestFacturaE_SignIncludesPolicyQualifierWhenConfigured(t *testing.T) {
	priv, cert := certForTest(t, "FacturaE-Qualifier-Test")
	xmlData, err := os.ReadFile("testdata/sample-facturae.xml")
	if err != nil {
		t.Fatalf("leyendo factura de ejemplo: %v", err)
	}
	engine := NewFacturaESigner()
	job := domain.SignatureJob{
		Document: domain.Document{Name: "factura.xml", MIMEType: "application/xml", Content: xmlData},
		Format:   formatFacturaE,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"policyQualifier": "https://www.facturae.gob.es/politica.html",
		},
	}
	result, err := engine.Sign(context.Background(), job, &LocalSigningKey{Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("Sign(FacturaE qualifier) error = %v", err)
	}
	out := string(result.Data)
	if !strings.Contains(out, "<xades:SigPolicyQualifiers>") || !strings.Contains(out, "<xades:SPURI>https://www.facturae.gob.es/politica.html</xades:SPURI>") {
		t.Fatalf("la firma FacturaE no contiene el qualifier esperado: %.400s", out)
	}
}

// La muestra vive fuera del repositorio, en el arbol de clienteafirma 1.9. Se
// localiza con GRXFIRMA_V19_SOURCE, misma convencion que GRXFIRMA_V1_SAMPLES
// en test/regression. Antes habia aqui una ruta absoluta al equipo de
// desarrollo, con lo que el test no podia ejecutarse en ningun otro sitio.
func TestFacturaEVerifier_AcceptsOfficialV19Sample(t *testing.T) {
	raiz := os.Getenv("GRXFIRMA_V19_SOURCE")
	if strings.TrimSpace(raiz) == "" {
		t.Skip("defina GRXFIRMA_V19_SOURCE con la raiz de clienteafirma-1.9-oficial")
	}
	xmlData, err := os.ReadFile(filepath.Join(raiz, "afirma-simple", "src", "test", "resources", "sample-facturae-firmada.xsig.xml"))
	if err != nil {
		t.Skipf("muestra oficial FacturaE no disponible: %v", err)
	}
	result, signers, err := NewFacturaEVerifier().Verify(context.Background(), domain.Document{Name: "sample-facturae-firmada.xsig.xml", MIMEType: "application/xml", Content: xmlData}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(sample FacturaE oficial) error = %v", err)
	}
	// La muestra se firmó en 2009 y su certificado está caducado. La prueba
	// exige interoperabilidad criptográfica, no vigencia actual del firmante.
	if result.Integrity.Status != domain.VerificationStatusValid || len(signers) == 0 {
		t.Fatalf("resultado inesperado: %+v signers=%d", result, len(signers))
	}
}
