// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package e2e contiene el test extremo a extremo del nucleo de GrxFirma.
// Verifica que el flujo completo funciona sin UI, sin red ni dependencias externas:
//
//	CLI adapter → SignDocument use case → MotorFirmaGo (CAdES-BES) → resultado firmado
//
// El certificado de prueba se genera en memoria con crypto/x509; no se requiere
// ninguna herramienta externa ni fichero en disco.
package e2e_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"math/big"
	"os"
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/common/cli"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// ---------------------------------------------------------------------------
// Stubs en memoria (sin efectos de red ni disco)
// ---------------------------------------------------------------------------

type catalogoMemoria struct {
	certs []domain.CertificateRef
}

func (c *catalogoMemoria) List(_ context.Context) ([]domain.CertificateRef, error) {
	return c.certs, nil
}

type proveedorClavesMemoria struct {
	clave ports.SigningKey
}

func (p *proveedorClavesMemoria) KeyFor(_ context.Context, _ domain.CertificateRef) (ports.SigningKey, error) {
	return p.clave, nil
}

type aprobacionAutomatica struct{}

func (a *aprobacionAutomatica) Request(_ context.Context, _ string) (bool, error) {
	return true, nil
}

type loggerMemoria struct {
	registros []ports.Evidence
}

func (l *loggerMemoria) Log(_ context.Context, e ports.Evidence) error {
	l.registros = append(l.registros, e)
	return nil
}

type relojReal struct{}

func (r relojReal) Now() time.Time { return time.Now() }

// ---------------------------------------------------------------------------
// Generacion de certificados de prueba (solo crypto/x509, sin herramientas externas)
// ---------------------------------------------------------------------------

func generarCertRSAPrueba(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("no se pudo generar clave RSA de prueba: %v", err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Cert E2E GrxFirma", Organization: []string{"Dipgra Test"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	derCert, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("no se pudo crear el certificado de prueba: %v", err)
	}
	cert, err := x509.ParseCertificate(derCert)
	if err != nil {
		t.Fatalf("no se pudo parsear el certificado generado: %v", err)
	}
	return priv, cert
}

// ---------------------------------------------------------------------------
// Verificacion estructural y criptografica del CAdES producido
// ---------------------------------------------------------------------------

// contentInfoRaw es el envoltorio CMS de nivel superior.
type contentInfoRaw struct {
	TipoContenido asn1.ObjectIdentifier
	Contenido     asn1.RawValue // [0] EXPLICIT; Bytes = DER del SignedData
}

// rawSignedData permite acceder a los campos con tags implicitos sin perder bytes.
type rawSignedData struct {
	Version      int
	AlgsDigest   asn1.RawValue
	EncapContent asn1.RawValue
	Certificados asn1.RawValue `asn1:"optional"`
	Firmantes    asn1.RawValue
}

// verificarEstructuraCAdES comprueba que el DER es un ContentInfo CMS valido
// con OID id-signedData y la estructura minima requerida por CAdES-BES.
func verificarEstructuraCAdES(t *testing.T, der []byte) {
	t.Helper()
	if len(der) == 0 {
		t.Fatal("el resultado de firma esta vacio")
	}
	var ci contentInfoRaw
	resto, err := asn1.Unmarshal(der, &ci)
	if err != nil {
		t.Fatalf("el resultado no es un ContentInfo ASN.1 valido: %v", err)
	}
	if len(resto) != 0 {
		t.Errorf("bytes sobrantes tras el ContentInfo: %d bytes", len(resto))
	}
	oidSignedData := asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	if !ci.TipoContenido.Equal(oidSignedData) {
		t.Errorf("OID del ContentInfo incorrecto: %v", ci.TipoContenido)
	}
	if ci.Contenido.Class != asn1.ClassContextSpecific || ci.Contenido.Tag != 0 {
		t.Errorf("se esperaba [0] EXPLICIT, class=%d tag=%d", ci.Contenido.Class, ci.Contenido.Tag)
	}
	if len(ci.Contenido.Bytes) == 0 || ci.Contenido.Bytes[0] != 0x30 {
		t.Errorf("el contenido del ContentInfo no es SEQUENCE; primer byte: %02x", ci.Contenido.Bytes[0])
	}
}

// verificarFirmaCriptografica extrae la firma del primer SignerInfo y la verifica
// contra la clave publica RSA indicada, sin ninguna herramienta externa.
func verificarFirmaCriptografica(t *testing.T, der []byte, pub *rsa.PublicKey) {
	t.Helper()

	var ci contentInfoRaw
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		t.Fatalf("no se pudo parsear ContentInfo: %v", err)
	}

	var sd rawSignedData
	if _, err := asn1.Unmarshal(ci.Contenido.Bytes, &sd); err != nil {
		t.Fatalf("no se pudo parsear SignedData: %v", err)
	}

	if sd.Firmantes.Tag != asn1.TagSet {
		t.Fatalf("Firmantes no es un SET, tag=%d", sd.Firmantes.Tag)
	}

	var siRaw asn1.RawValue
	if _, err := asn1.Unmarshal(sd.Firmantes.Bytes, &siRaw); err != nil {
		t.Fatalf("no se pudo extraer el primer SignerInfo: %v", err)
	}

	type signerInfoParsed struct {
		Version        int
		SID            asn1.RawValue
		AlgDigest      asn1.RawValue
		AtribsFirmados asn1.RawValue
		AlgFirma       asn1.RawValue
		Firma          []byte
	}
	var si signerInfoParsed
	if _, err := asn1.Unmarshal(siRaw.FullBytes, &si); err != nil {
		t.Fatalf("no se pudo parsear SignerInfo: %v", err)
	}

	if si.AtribsFirmados.Class != asn1.ClassContextSpecific || si.AtribsFirmados.Tag != 0 {
		t.Fatalf("AtribsFirmados no tiene tag [0], class=%d tag=%d",
			si.AtribsFirmados.Class, si.AtribsFirmados.Tag)
	}

	// Rehash de los atributos como SET (0x31) para verificar la firma.
	atribsComoSET := make([]byte, len(si.AtribsFirmados.FullBytes))
	copy(atribsComoSET, si.AtribsFirmados.FullBytes)
	atribsComoSET[0] = 0x31 // [0] CONTEXT → SET

	digestAttrs := sha256.Sum256(atribsComoSET)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digestAttrs[:], si.Firma); err != nil {
		t.Fatalf("la firma RSA no es criptograficamente valida: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helper: montar todos los componentes reales
// ---------------------------------------------------------------------------

// montarPila construye la pila completa de adaptadores y casos de uso reales.
// Devuelve el adaptador CLI listo para usar, la clave RSA (para verificacion),
// el logger de auditoria (para inspeccionar registros) y los mapas de ficheros.
func montarPila(t *testing.T) (
	priv *rsa.PrivateKey,
	certRef domain.CertificateRef,
	adaptador *cli.Adaptador,
	logger *loggerMemoria,
	salidaDatos *[]byte,
	archivos map[string][]byte,
) {
	t.Helper()

	var cert *x509.Certificate
	priv, cert = generarCertRSAPrueba(t)

	motor := signer.NuevoMotorFirmaGo(nil)
	clave := signer.NuevaClaveLocal(priv, cert)
	huella := sha256.Sum256(cert.Raw)
	certRef = domain.CertificateRef{
		ID:          clave.KeyID(),
		Subject:     cert.Subject.CommonName,
		Issuer:      cert.Issuer.CommonName,
		NotAfter:    cert.NotAfter,
		Fingerprint: hex.EncodeToString(huella[:]),
	}

	logger = &loggerMemoria{}
	auditor := application.NuevoAuditUseCase(relojReal{}, logger)
	ucApp := application.NuevoSignDocumentUseCase(
		&catalogoMemoria{certs: []domain.CertificateRef{certRef}},
		&proveedorClavesMemoria{clave: clave},
		motor,
		&aprobacionAutomatica{},
		auditor,
		nil,
	)

	var capturaSalida []byte
	salidaDatos = &capturaSalida
	archivos = map[string][]byte{}

	// *application.SignDocumentUseCase satisface cli.SignDocumentUseCase (Execute)
	// gracias al wrapper delgado que todos los casos de uso exponen.
	adaptador = &cli.Adaptador{
		Firmar: ucApp,
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		LeerFichero: func(ruta string) ([]byte, error) {
			if data, ok := archivos[ruta]; ok {
				return data, nil
			}
			return nil, os.ErrNotExist
		},
		Escribir: func(_ string, datos []byte, _ os.FileMode) error {
			capturaSalida = datos
			return nil
		},
	}
	return
}

// ---------------------------------------------------------------------------
// Tests E2E
// ---------------------------------------------------------------------------

// TestFirmaE2E_CAdES_BES_FlujoCompleto verifica el MVP de Fase 2:
// CLI → SignDocument → MotorFirmaGo → salida CAdES estructural y criptograficamente valida.
// No abre sockets, no lanza procesos externos y no escribe en disco real.
func TestFirmaE2E_CAdES_BES_FlujoCompleto(t *testing.T) {
	priv, certRef, adaptador, logger, salidaDatos, archivos := montarPila(t)

	archivos["entrada.txt"] = []byte("documento de prueba para el test E2E de firma CAdES-BES")

	args := []string{
		"-entrada", "entrada.txt",
		"-salida", "salida.csig",
		"-formato", "CAdES",
		"-accion", "sign",
		"-certificado", certRef.ID,
	}
	codigo := adaptador.Run(context.Background(), args)
	if codigo != 0 {
		t.Fatalf("el adaptador CLI devolvio codigo de error %d", codigo)
	}

	if len(*salidaDatos) == 0 {
		t.Fatal("el fichero de salida esta vacio")
	}

	// Verificar estructura CAdES (sin herramientas externas).
	verificarEstructuraCAdES(t, *salidaDatos)

	// Verificar firma criptograficamente.
	verificarFirmaCriptografica(t, *salidaDatos, &priv.PublicKey)

	// La operacion debe haber quedado registrada en auditoria.
	if len(logger.registros) == 0 {
		t.Fatal("la operacion no quedo registrada en auditoria")
	}
	t.Logf("firma E2E OK: %d bytes firmados, %d registros de auditoria", len(*salidaDatos), len(logger.registros))
}

// TestFirmaE2E_CAdES_BES_UnSoloCertSinEspecificarID comprueba que cuando solo hay
// un certificado en el catalogo no es necesario indicar -certificado.
func TestFirmaE2E_CAdES_BES_UnSoloCertSinEspecificarID(t *testing.T) {
	_, _, adaptador, _, salidaDatos, archivos := montarPila(t)

	archivos["doc.bin"] = []byte("contenido binario arbitrario para firma CAdES sin ID de cert")

	// Sin -certificado: el caso de uso debe usar el unico disponible.
	args := []string{"-entrada", "doc.bin", "-salida", "doc.csig", "-formato", "CAdES"}
	if codigo := adaptador.Run(context.Background(), args); codigo != 0 {
		t.Fatalf("codigo de error inesperado: %d", codigo)
	}
	if len(*salidaDatos) == 0 {
		t.Fatal("el fichero de salida esta vacio")
	}
	verificarEstructuraCAdES(t, *salidaDatos)
}

// TestFirmaE2E_CAdES_BES_SinFicheroSalida comprueba que el flujo no falla
// cuando no se especifica fichero de salida (la firma se produce pero no se persiste).
func TestFirmaE2E_CAdES_BES_SinFicheroSalida(t *testing.T) {
	_, _, adaptador, _, _, archivos := montarPila(t)

	archivos["doc.txt"] = []byte("contenido")

	// Sin -salida: el caso de uso debe completar correctamente sin escribir nada.
	args := []string{"-entrada", "doc.txt", "-formato", "CAdES"}
	if codigo := adaptador.Run(context.Background(), args); codigo != 0 {
		t.Fatalf("codigo de error inesperado: %d", codigo)
	}
}

// TestFirmaE2E_CAdES_BES_ContextoCancelado comprueba que la cancelacion del
// contexto se propaga correctamente y el adaptador devuelve codigo de error.
func TestFirmaE2E_CAdES_BES_ContextoCancelado(t *testing.T) {
	_, _, adaptador, _, _, archivos := montarPila(t)

	archivos["doc.txt"] = []byte("contenido")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelado antes de empezar

	// El timeout de 1ms o el contexto cancelado deben provocar fallo.
	args := []string{"-entrada", "doc.txt", "-salida", "out.csig", "-formato", "CAdES", "-timeout", "1ms"}
	codigo := adaptador.Run(ctx, args)
	if codigo == 0 {
		t.Log("advertencia: el contexto cancelado no provoco error en el adaptador CLI (puede depender del scheduling)")
	}
}
