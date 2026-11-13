// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"testing"

	"github.com/digitorus/pkcs7"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

func tieneEvidenciaAlgoritmoAntiguo(result domain.VerificationResult) bool {
	for _, evidencia := range result.Evidence {
		if evidencia.Type == domain.EvidenciaAlgoritmoAntiguo {
			return true
		}
	}
	return false
}

func exigirAlgoritmoAntiguo(t *testing.T, result domain.VerificationResult) {
	t.Helper()
	if result.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("una firma SHA-1 correcta debe dar integridad comprobada: %+v", result.Integrity)
	}
	if result.Valid {
		t.Fatalf("una firma SHA-1 no puede salir válida: %+v", result)
	}
	if !tieneEvidenciaAlgoritmoAntiguo(result) || !contieneDetalle(result.Warnings, motivoAlgoritmoAntiguo) {
		t.Fatalf("falta el aviso de algoritmo antiguo: avisos=%v evidencia=%v", result.Warnings, result.Evidence)
	}
	for _, evidencia := range result.Evidence {
		if evidencia.Type == xmlCompatibilityEvidence {
			t.Fatalf("SHA-1 no es compatibilidad de canonicalización: %v", evidencia)
		}
	}
}

// Lote 13b, punto 1: FacturaE 3.1 con prefijo en la raíz y firmada con
// SHA-1, como las facturas oficiales de @firma de 2009. Antes salía
// «Compatibilidad histórica; validación XML estándar no acreditada» con
// integridad en advertencia (y el verificador FacturaE ni la reconocía). La
// integridad se comprueba por el camino estándar y el aviso es el de
// algoritmo antiguo.
func TestFacturaEVerifier_SHA1ConPrefijoIntegridadComprobadaConAviso(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, true)
	priv, cert := certForTest(t, "FacturaE SHA-1 histórica")
	payload := []byte(`<fe:Facturae xmlns:fe="` + facturaeNamespace31 + `"><FileHeader/><Parties/><Invoices>QA 2009</Invoices></fe:Facturae>`)
	firmado, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "factura.xml", MIMEType: "application/xml", Content: payload},
		Format:   domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"format": "XAdES Enveloped", "algorithm": "SHA1withRSA"},
	}, &LocalSigningKey{Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	if !isFacturaEXML(firmado.Data) {
		t.Fatal("una FacturaE con prefijo en la raíz debe reconocerse")
	}
	result, signers, err := NewFacturaEVerifier().Verify(context.Background(), domain.Document{Name: "factura.xsig", MIMEType: "application/xml", Content: firmado.Data}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(signers) != 1 || result.Coverage != domain.CoberturaCompleta {
		t.Fatalf("firmantes=%d cobertura=%s", len(signers), result.Coverage)
	}
	exigirAlgoritmoAntiguo(t, result)

	// Alterar la factura sigue invalidando, con SHA-1 o sin él.
	alterado := bytes.Replace(firmado.Data, []byte("QA 2009"), []byte("QA 2010"), 1)
	if _, _, err := NewFacturaEVerifier().Verify(context.Background(), domain.Document{Name: "factura.xsig", MIMEType: "application/xml", Content: alterado}, domain.CertificateChain{}); err == nil {
		t.Fatal("una factura SHA-1 alterada no puede verificarse")
	}
}

// La raíz con prefijo no abre la puerta a una firma ajena metida en una
// factura (E1-02): el verificador exige la raíz Facturae, la firma como hija
// directa y la referencia enveloped al documento.
func TestFacturaEVerifier_RaizConPrefijoSinNamespaceNoEsFactura(t *testing.T) {
	for _, entrada := range []string{
		`<fe:Facturae xmlns:fe="urn:otra:cosa"><FileHeader/><Parties/><Invoices/></fe:Facturae>`,
		`<fe:Otra xmlns:fe="` + facturaeNamespace31 + `"><FileHeader/><Parties/><Invoices/></fe:Otra>`,
	} {
		if isFacturaEXML([]byte(entrada)) {
			t.Fatalf("no es una FacturaE: %s", entrada)
		}
	}
}

// Lote 13b, punto 2: CAdES con SHA-1 (AutoFirma lo usaba hasta 2014). Antes
// se rechazaba con error «SignerInfo no declara SHA-256...». Ahora la
// integridad se comprueba y la firma sale con el aviso de algoritmo antiguo.
func TestCAdESVerifier_SHA1IntegridadComprobadaConAviso(t *testing.T) {
	priv, cert := certForTest(t, "CAdES SHA-1 histórica")
	contenido := []byte("documento firmado en 2012")
	datos, err := pkcs7.NewSignedData(contenido)
	if err != nil {
		t.Fatal(err)
	}
	datos.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
	if err := datos.AddSigner(cert, priv, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	cms, err := datos.Finish()
	if err != nil {
		t.Fatal(err)
	}
	result, signers, err := NewCAdESVerifier().Verify(context.Background(), domain.Document{Name: "antigua.csig", Content: cms}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(signers) != 1 {
		t.Fatalf("firmantes=%d", len(signers))
	}
	exigirAlgoritmoAntiguo(t, result)

	// El mismo CMS sobre otro contenido (detached) sigue siendo inválido.
	datos, _ = pkcs7.NewSignedData(contenido)
	datos.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
	if err := datos.AddSigner(cert, priv, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	datos.Detach()
	separado, err := datos.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), separado, []byte("otro contenido")); err == nil {
		t.Fatal("una firma SHA-1 sobre otro contenido no puede verificarse")
	}
	if res, _, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), separado, contenido); err != nil {
		t.Fatalf("detached SHA-1 correcta: %v", err)
	} else {
		exigirAlgoritmoAntiguo(t, res)
	}
}
