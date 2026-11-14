// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// pkiLote3 es una PKI de prueba de tres niveles: raíz, intermedia y hoja.
type pkiLote3 struct {
	raiz, intermedia, hoja     *x509.Certificate
	claveRaiz, claveIntermedia *ecdsa.PrivateKey
	claveHoja                  *ecdsa.PrivateKey
	anclas                     domain.CertificateChain
}

func emitirLote3(t *testing.T, plantilla, emisor *x509.Certificate, pub crypto.PublicKey, clave crypto.Signer) *x509.Certificate {
	t.Helper()
	if emisor == nil {
		emisor = plantilla
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, emisor, pub, clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func claveLote3(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return clave
}

// nuevaPKILote3 crea la PKI; la hoja lleva el CRLDP y los EKU indicados.
func nuevaPKILote3(t *testing.T, crldp []string, eku ...x509.ExtKeyUsage) pkiLote3 {
	t.Helper()
	ahora := time.Now()
	var p pkiLote3
	p.claveRaiz = claveLote3(t)
	plantillaRaiz := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Raíz lote 3"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(48 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	p.raiz = emitirLote3(t, plantillaRaiz, nil, &p.claveRaiz.PublicKey, p.claveRaiz)
	p.claveIntermedia = claveLote3(t)
	p.intermedia = emitirLote3(t, &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Intermedia lote 3"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(48 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}, p.raiz, &p.claveIntermedia.PublicKey, p.claveRaiz)
	p.claveHoja = claveLote3(t)
	p.hoja = emitirLote3(t, &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "Titular lote 3"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(48 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		ExtKeyUsage: eku, CRLDistributionPoints: crldp,
	}, p.intermedia, &p.claveHoja.PublicKey, p.claveIntermedia)
	p.anclas = domain.CertificateChain{DERCertificates: [][]byte{p.raiz.Raw}}
	return p
}

// idpLote3 construye un IssuingDistributionPoint con un único URI.
func idpLote3(t *testing.T, uri string) pkix.Extension {
	t.Helper()
	general, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 6, Bytes: []byte(uri)})
	fullName, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: general})
	dpn, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: fullName})
	valor, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: dpn})
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Critical: true, Value: valor}
}

func crlLote3(t *testing.T, emisor *x509.Certificate, clave crypto.Signer, revocado *big.Int, extra ...pkix.Extension) []byte {
	t.Helper()
	ahora := time.Now()
	plantilla := &x509.RevocationList{
		Number: big.NewInt(ahora.UnixNano()), ThisUpdate: ahora.Add(-time.Minute),
		NextUpdate: ahora.Add(time.Hour), ExtraExtensions: extra,
	}
	if revocado != nil {
		plantilla.RevokedCertificateEntries = []x509.RevocationListEntry{{
			SerialNumber: revocado, RevocationTime: ahora.Add(-2 * time.Minute),
		}}
	}
	der, err := x509.CreateRevocationList(rand.Reader, plantilla, emisor, clave)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// firmarLTLote3 firma con la hoja una CAdES-LT que embebe la evidencia dada.
func firmarLTLote3(t *testing.T, p pkiLote3, contenido []byte, evidencia ports.RevocationEvidence) []byte {
	t.Helper()
	doc, _ := domain.NewDocument("doc.txt", contenido, "text/plain")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}
	firmante := NewSignerCAdESLT(
		NewSignerCAdEST(NewCAdESBESDetached(), &tsaLTMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x01}}),
		&revocationProviderMock{evidence: evidencia},
	)
	res, err := firmante.Sign(context.Background(), job, &LocalSigningKey{
		ID: "lote3", Signer: p.claveHoja, Certificate: p.hoja,
		Chain: []*x509.Certificate{p.intermedia, p.raiz},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return res.Data
}

// firmarBESLote3 firma una CAdES-BES con la hoja y los certificados de cadena dados.
func firmarBESLote3(t *testing.T, p pkiLote3, contenido []byte, cadena ...*x509.Certificate) []byte {
	t.Helper()
	doc, _ := domain.NewDocument("doc.txt", contenido, "text/plain")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}
	res, err := NewCAdESBESDetached().Sign(context.Background(), job, &LocalSigningKey{
		ID: "lote3-bes", Signer: p.claveHoja, Certificate: p.hoja, Chain: cadena,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return res.Data
}

// comprobadorLote3 responde como un servicio OCSP/CRL autenticado: los
// certificados de la lista están revocados y el resto vigentes.
func comprobadorLote3(revocados ...*x509.Certificate) *RevocationChecker {
	return &RevocationChecker{consulta: func(_ context.Context, cert, _ *x509.Certificate) (RevocationResult, error) {
		for _, r := range revocados {
			if r.Equal(cert) {
				return RevocationResult{Status: RevocationRevoked, Method: "ocsp", CheckedAt: time.Now(), RevokedAt: time.Now().Add(-time.Hour)}, nil
			}
		}
		return RevocationResult{Status: RevocationGood, Method: "ocsp", CheckedAt: time.Now()}, nil
	}}
}

// TestCAdESLT_CRLEmbebidaDeOtraParticionNoAcreditaVigencia reproduce E1-04:
// la firma de un certificado revocado embebe una CRL auténtica y vigente del
// mismo emisor, pero de otra partición. No puede salir «no revocado».
func TestCAdESLT_CRLEmbebidaDeOtraParticionNoAcreditaVigencia(t *testing.T) {
	p := nuevaPKILote3(t, []string{"http://crl.invalid/mia.crl"})
	contenido := []byte("contrato")
	otraParticion := crlLote3(t, p.intermedia, p.claveIntermedia, nil, idpLote3(t, "http://crl.invalid/otra.crl"))
	arl := crlLote3(t, p.raiz, p.claveRaiz, nil)
	cms := firmarLTLote3(t, p, contenido, ports.RevocationEvidence{CRLs: [][]byte{otraParticion, arl}})

	verificador := NewCAdESVerifierWithChecker(NewRevocationCheckerOffline())
	res, _, err := verificador.VerifyDetachedCMSWithAnchors(context.Background(), cms, contenido, p.anclas)
	if err != nil {
		t.Fatalf("VerifyDetachedCMSWithAnchors: %v", err)
	}
	if res.Valid {
		t.Fatalf("una CRL de otra partición dio la firma por válida: %s %v", res.Reason, res.Details)
	}
	if res.Certificate.Status == domain.VerificationStatusValid {
		t.Fatal("una CRL de otra partición no puede acreditar que el certificado no está revocado")
	}
}

// TestCAdESLT_EvidenciaEmbebidaNoAplicableConsultaEnLinea: si la evidencia
// embebida no sirve para un certificado, se consulta en línea en vez de
// quedarse con ella; y lo que diga la consulta prevalece.
func TestCAdESLT_EvidenciaEmbebidaNoAplicableConsultaEnLinea(t *testing.T) {
	p := nuevaPKILote3(t, []string{"http://crl.invalid/mia.crl"})
	contenido := []byte("contrato")
	otraParticion := crlLote3(t, p.intermedia, p.claveIntermedia, nil, idpLote3(t, "http://crl.invalid/otra.crl"))
	arl := crlLote3(t, p.raiz, p.claveRaiz, nil)
	cms := firmarLTLote3(t, p, contenido, ports.RevocationEvidence{CRLs: [][]byte{otraParticion, arl}})

	revocada := NewCAdESVerifierWithChecker(comprobadorLote3(p.hoja))
	res, _, err := revocada.VerifyDetachedCMSWithAnchors(context.Background(), cms, contenido, p.anclas)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid || res.Certificate.Status != domain.VerificationStatusInvalid {
		t.Fatalf("la consulta en línea decía revocado y salió Valid=%v certificado=%s (%s)", res.Valid, res.Certificate.Status, res.Reason)
	}

	vigente := NewCAdESVerifierWithChecker(comprobadorLote3())
	res, _, err = vigente.VerifyDetachedCMSWithAnchors(context.Background(), cms, contenido, p.anclas)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("con la consulta en línea vigente la firma debería ser válida: %s %v", res.Reason, res.Warnings)
	}
}

// TestCAdESLT_RevocacionEmbebidaNoOtorgaConfianza: una CRL embebida emitida
// por la propia CA del firmante no puede convertir en «de confianza» una
// cadena que ninguna ancla respalda.
func TestCAdESLT_RevocacionEmbebidaNoOtorgaConfianza(t *testing.T) {
	p := nuevaPKILote3(t, nil)
	contenido := []byte("contrato")
	cms := firmarLTLote3(t, p, contenido, ports.RevocationEvidence{CRLs: [][]byte{
		crlLote3(t, p.intermedia, p.claveIntermedia, nil),
		crlLote3(t, p.raiz, p.claveRaiz, nil),
	}})
	res, _, err := NewCAdESVerifierWithChecker(NewRevocationCheckerOffline()).VerifyDetachedCMS(context.Background(), cms, contenido)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trust.Status == domain.VerificationStatusValid {
		t.Fatalf("la evidencia embebida otorgó confianza sin anclas: %s", res.Trust.Reason)
	}
}
