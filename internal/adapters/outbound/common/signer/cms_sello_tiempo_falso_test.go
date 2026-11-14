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

	"github.com/digitorus/timestamp"
	"grxfirma/internal/domain"
)

// KD-01: un sello de tiempo solo da la fecha «según el sello de tiempo» si
// su autoridad se ha validado. Cualquiera puede añadir a una firma ajena un
// signatureTimeStampToken hecho con una «TSA» propia y la hora que quiera:
// los atributos no firmados no están protegidos.

type entidadSelloPrueba struct {
	cert  *x509.Certificate
	clave crypto.Signer
}

func emitirCertSelloPrueba(t *testing.T, plantilla *x509.Certificate, emisor *entidadSelloPrueba) entidadSelloPrueba {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	padre, firmaCon := plantilla, crypto.Signer(clave)
	if emisor != nil {
		padre, firmaCon = emisor.cert, emisor.clave
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, padre, clave.Public(), firmaCon)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return entidadSelloPrueba{cert: cert, clave: clave}
}

var serieSelloPrueba int64 = 7000

func plantillaSelloPrueba(nombre string, ca bool, desde time.Time) *x509.Certificate {
	serieSelloPrueba++
	p := &x509.Certificate{
		SerialNumber:          big.NewInt(serieSelloPrueba),
		Subject:               pkix.Name{CommonName: nombre},
		NotBefore:             desde,
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		BasicConstraintsValid: true,
	}
	if ca {
		p.IsCA = true
		p.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	} else {
		p.KeyUsage = x509.KeyUsageDigitalSignature
		p.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}
	}
	return p
}

// tsaSelloPrueba emite tokens RFC 3161 reales con la fecha que se le pida.
type tsaSelloPrueba struct {
	emisor entidadSelloPrueba
	fecha  time.Time
}

func (s tsaSelloPrueba) RequestTimestamp(_ context.Context, resumen []byte, algoritmo crypto.Hash) ([]byte, error) {
	sello := timestamp.Timestamp{
		HashAlgorithm:     algoritmo,
		HashedMessage:     append([]byte(nil), resumen...),
		Time:              s.fecha,
		SerialNumber:      big.NewInt(time.Now().UnixNano()),
		Policy:            asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1},
		AddTSACertificate: true,
	}
	respuesta, err := sello.CreateResponseWithOpts(s.emisor.cert, s.emisor.clave, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	analizado, err := timestamp.ParseResponse(respuesta)
	if err != nil {
		return nil, err
	}
	return analizado.RawToken, nil
}

func firmarCAdESTConTSA(t *testing.T, contenido []byte, tsa tsaSelloPrueba) []byte {
	t.Helper()
	priv, cert := certForTest(t, "Victima")
	doc, err := domain.NewDocument("documento.txt", contenido, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewSignerCAdEST(NewCAdESBESDetached(), tsa).Sign(context.Background(),
		domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign},
		&LocalSigningKey{ID: "clave", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return res.Data
}

func fechaUnicaVerificada(t *testing.T, cms, contenido []byte, anclas domain.CertificateChain) domain.VerificationSigningTime {
	t.Helper()
	vr, _, err := NewCAdESVerifier().VerifyDetachedCMSWithAnchors(context.Background(), cms, contenido, anclas)
	if err != nil {
		t.Fatal(err)
	}
	if len(vr.SigningTimes) != 1 {
		t.Fatalf("fechas = %+v", vr.SigningTimes)
	}
	return vr.SigningTimes[0]
}

var fechaFalsaSello = time.Date(2019, 3, 1, 9, 0, 0, 0, time.UTC)

// TSA autofirmada inventada por quien manipula la firma, fechada en 2019
// sobre un certificado emitido hoy: no puede salir como «según el sello».
func TestFechaFirmaCMS_SelloDeTSAInventadaNoEsSegunElSello(t *testing.T) {
	desde := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	tsa := emitirCertSelloPrueba(t, plantillaSelloPrueba("TSA inventada", false, desde), nil)
	contenido := []byte("acta con plazo")
	cms := firmarCAdESTConTSA(t, contenido, tsaSelloPrueba{emisor: tsa, fecha: fechaFalsaSello})
	for nombre, anclas := range map[string]domain.CertificateChain{
		"sin anclas":               {},
		"raíces del sistema":       {UseSystemRoots: true},
		"ancla ajena a la TSA":     {DERCertificates: [][]byte{emitirCertSelloPrueba(t, plantillaSelloPrueba("Otra CA", true, desde), nil).cert.Raw}},
		"la propia TSA como ancla": {DERCertificates: [][]byte{tsa.cert.Raw}},
	} {
		fecha := fechaUnicaVerificada(t, cms, contenido, anclas)
		if fecha.Source == domain.SigningTimeSourceTimestamp {
			t.Fatalf("%s: la fecha de un sello no validado sale como «según el sello»: %+v", nombre, fecha)
		}
		if fecha.Time.Equal(fechaFalsaSello) {
			t.Fatalf("%s: la fecha inventada del sello manda sobre el signingTime firmado: %+v", nombre, fecha)
		}
		if fecha.Source != domain.SigningTimeSourceSignedAttribute {
			t.Fatalf("%s: con signingTime firmado se esperaba ese origen: %+v", nombre, fecha)
		}
	}
}

// Una TSA que encadena a un ancla configurada sigue dando la fecha del sello.
func TestFechaFirmaCMS_SelloDeTSAAncladaSigueSiendoSegunElSello(t *testing.T) {
	desde := time.Now().Add(-48 * time.Hour)
	ca := emitirCertSelloPrueba(t, plantillaSelloPrueba("CA de sellado", true, desde), nil)
	tsa := emitirCertSelloPrueba(t, plantillaSelloPrueba("TSA de confianza", false, desde), &ca)
	instante := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	contenido := []byte("acta sellada")
	cms := firmarCAdESTConTSA(t, contenido, tsaSelloPrueba{emisor: tsa, fecha: instante})
	fecha := fechaUnicaVerificada(t, cms, contenido, domain.CertificateChain{DERCertificates: [][]byte{ca.cert.Raw}})
	if fecha.Source != domain.SigningTimeSourceTimestamp || !fecha.Time.Equal(instante) {
		t.Fatalf("se esperaba la fecha del sello validado: %+v (sello %s)", fecha, instante)
	}
}

// Una TSA de confianza tampoco puede fechar la firma antes de que existiera
// el certificado del firmante.
func TestFechaFirmaCMS_SelloFueraDeLaVigenciaDelFirmanteNoEsSegunElSello(t *testing.T) {
	desde := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	ca := emitirCertSelloPrueba(t, plantillaSelloPrueba("CA de sellado", true, desde), nil)
	tsa := emitirCertSelloPrueba(t, plantillaSelloPrueba("TSA de confianza", false, desde), &ca)
	contenido := []byte("acta antedatada")
	cms := firmarCAdESTConTSA(t, contenido, tsaSelloPrueba{emisor: tsa, fecha: fechaFalsaSello})
	fecha := fechaUnicaVerificada(t, cms, contenido, domain.CertificateChain{DERCertificates: [][]byte{ca.cert.Raw}})
	if fecha.Source == domain.SigningTimeSourceTimestamp || fecha.Time.Equal(fechaFalsaSello) {
		t.Fatalf("un sello anterior al certificado del firmante no puede dar la fecha: %+v", fecha)
	}
}

// La fecha del sello solo se informa: ni siquiera un sello validado hace
// válida una firma cuyo certificado ya ha caducado (la vigencia se evalúa en
// el instante actual).
func TestFechaFirmaCMS_SelloNoSalvaUnCertificadoCaducado(t *testing.T) {
	desde := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	ca := emitirCertSelloPrueba(t, plantillaSelloPrueba("CA de sellado", true, desde), nil)
	tsa := emitirCertSelloPrueba(t, plantillaSelloPrueba("TSA de confianza", false, desde), &ca)
	plantilla := plantillaSelloPrueba("Firmante caducado", false, desde)
	plantilla.NotAfter = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	plantilla.ExtKeyUsage = nil
	plantilla.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment
	firmante := emitirCertSelloPrueba(t, plantilla, &ca)
	contenido := []byte("acta firmada en 2019")
	doc, _ := domain.NewDocument("acta.txt", contenido, "text/plain")
	res, err := NewSignerCAdEST(NewCAdESBESDetached(), tsaSelloPrueba{emisor: tsa, fecha: fechaFalsaSello}).Sign(context.Background(),
		domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign},
		&LocalSigningKey{ID: "clave", Signer: firmante.clave, Certificate: firmante.cert})
	if err != nil {
		t.Skipf("el firmador no firma con un certificado caducado: %v", err)
	}
	vr, _, err := NewCAdESVerifier().VerifyDetachedCMSWithAnchors(context.Background(), res.Data, contenido,
		domain.CertificateChain{DERCertificates: [][]byte{ca.cert.Raw}})
	if err != nil {
		t.Fatal(err)
	}
	if len(vr.SigningTimes) != 1 || vr.SigningTimes[0].Source != domain.SigningTimeSourceTimestamp {
		t.Fatalf("se esperaba la fecha del sello validado: %+v", vr.SigningTimes)
	}
	if vr.Valid || vr.Certificate.Status != domain.VerificationStatusInvalid {
		t.Fatalf("un certificado caducado no se da por bueno por el sello: valid=%v certificado=%+v", vr.Valid, vr.Certificate)
	}
}

// Sin signingTime firmado, el sello sin comprobar se informa con su propio
// origen («declarada por el sello, sin comprobar»), nunca como validado.
func TestFechaFirmaCMS_SinSigningTimeElSelloSinComprobarTieneSuOrigen(t *testing.T) {
	desde := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	tsa := emitirCertSelloPrueba(t, plantillaSelloPrueba("TSA inventada", false, desde), nil)
	cms := firmarCAdESTConTSA(t, []byte("acta"), tsaSelloPrueba{emisor: tsa, fecha: fechaFalsaSello})
	ci, err := parseContentInfo(cms)
	if err != nil {
		t.Fatal(err)
	}
	sd, err := parseSignedData(ci.Content.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := parseSignerInfos(sd.SignerInfos)
	if err != nil || len(infos) != 1 {
		t.Fatalf("SignerInfos: %v", err)
	}
	certs, _, err := parseCertificateSet(sd.Certificates)
	if err != nil {
		t.Fatal(err)
	}
	firmante, err := findSignerCertificate(certs, infos[0].SID)
	if err != nil {
		t.Fatal(err)
	}
	si := infos[0]
	si.SignedAttributes = asn1.RawValue{}
	fecha, origen, ok := fechaFirmaCMS(si, firmante, domain.CertificateChain{DERCertificates: [][]byte{tsa.cert.Raw}})
	if !ok || origen != domain.SigningTimeSourceTimestampUnverified || !fecha.Equal(fechaFalsaSello) {
		t.Fatalf("fecha=%s origen=%q ok=%v", fecha, origen, ok)
	}
	if !sinFechaFirmada([]domain.VerificationSigningTime{{Source: origen}}) {
		t.Fatal("un sello sin comprobar no cuenta como fecha firmada en PAdES (la /M manda)")
	}
	if sinFechaFirmada([]domain.VerificationSigningTime{{Source: domain.SigningTimeSourceTimestamp}}) {
		t.Fatal("un sello validado sí es fecha firmada")
	}
}
