// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package conformance_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/tsaclient"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/exttools"
	"grxfirma/internal/testsupport/pdffixture"
)

// Las herramientas reciben solo documentos y certificados sintéticos. No se
// consulta OCSP, AIA ni ningún validador público. La TSA usa HTTP en memoria.
func TestInteroperabilidadLocal(t *testing.T) {
	priv, cert := generarCertRSAConformance(t)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	for _, formato := range []string{"pades", "pades-t", "cades", "xades", "facturae"} {
		t.Run(formato, func(t *testing.T) {
			dir := t.TempDir()
			if base := os.Getenv("GRXFIRMA_CONFORMANCE_ARTIFACTS"); base != "" {
				dir = filepath.Join(base, formato)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			escribirLocal(t, dir, "firmante.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
			motor := desktopsigner.NuevoMotorFirmaGo(nil)
			contenido, mime, nombre, formatoFirma := []byte("muestra cades local\n"), "text/plain", "original.txt", domain.FormatCAdES
			opciones := map[string]string{}
			var peticion, respuesta []byte
			if strings.HasPrefix(formato, "pades") {
				contenido, mime, nombre, formatoFirma = pdffixture.Minimal(), "application/pdf", "original.pdf", domain.FormatPAdES
			}
			if formato == "pades-t" {
				responder := nuevaTSALocal(t)
				cliente := tsaclient.New("http://127.0.0.1/tsa-sintetica")
				// Transporte HTTP en memoria: funciona también en entornos sin
				// permiso para abrir sockets y conserva el protocolo RFC 3161.
				cliente.HTTPClient.Transport = transporteTSALocal(func(r *http.Request) (*http.Response, error) {
					cuerpo, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
					if err != nil {
						return nil, err
					}
					der, err := responder(cuerpo)
					if err != nil {
						return nil, err
					}
					peticion, respuesta = cuerpo, der
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/timestamp-reply"}}, Body: io.NopCloser(bytes.NewReader(der)), Request: r}, nil
				})
				motor = motor.WithTimestampAuthority(cliente)
				opciones = map[string]string{"level": "T"}
			}
			if formato == "xades" {
				contenido, mime, nombre, formatoFirma = []byte("<doc Id=\"documento\"><dato>muestra local</dato></doc>"), "application/xml", "original.xml", domain.FormatXAdES
				opciones = map[string]string{"format": "XAdES Enveloped"}
			}
			if formato == "facturae" {
				var err error
				contenido, err = os.ReadFile("../../internal/adapters/outbound/common/signer/testdata/sample-facturae.xml")
				if err != nil {
					t.Fatal(err)
				}
				mime, nombre, formatoFirma = "application/xml", "original.xml", domain.SignatureFormat("FacturaE")
			}
			escribirLocal(t, dir, nombre, contenido)
			doc, err := domain.NewDocument(nombre, contenido, mime)
			if err != nil {
				t.Fatal(err)
			}
			resultado, err := motor.Sign(context.Background(), domain.SignatureJob{Document: doc, Format: formatoFirma, Action: domain.ActionSign, Options: opciones}, clave)
			if err != nil {
				t.Fatal(err)
			}
			extension := ".xml"
			if strings.HasPrefix(formato, "pades") {
				extension = ".pdf"
			}
			if formato == "cades" {
				extension = ".p7s"
			}
			firmado := escribirLocal(t, dir, "firmado"+extension, resultado.Data)
			switch formato {
			case "pades", "pades-t":
				if exttools.Available(t, "qpdf") {
					ejecutarLocal(t, "qpdf", "--check", firmado)
				}
				if exttools.Available(t, "pdfsig") && exttools.Available(t, "certutil") {
					nss := filepath.Join(dir, "nss")
					if err := os.MkdirAll(nss, 0o700); err != nil {
						t.Fatal(err)
					}
					ejecutarLocal(t, "certutil", "-N", "-d", "sql:"+nss, "--empty-password")
					// -nocert evita consultas de confianza de red; OpenSSL
					// verifica la firma y el certificado sintético explícito.
					salida := ejecutarLocal(t, "pdfsig", "-nssdir", nss, "-nocert", "-no-ocsp", firmado)
					if !strings.Contains(salida, "Signature Validation: Signature is Valid.") {
						t.Fatalf("pdfsig no valida la firma: %s", salida)
					}
				}
				cms, datos := extraerCMSLocal(t, resultado.Data)
				validarCMSLocal(t, dir, cms, datos)
				if formato == "pades-t" {
					if resultado.Algorithm != "PAdES-B-T" || len(respuesta) == 0 {
						t.Fatalf("falta PAdES-B-T con respuesta TSA: %s", resultado.Algorithm)
					}
					sello, err := timestamp.ParseResponse(respuesta)
					if err != nil || len(sello.Certificates) == 0 {
						t.Fatalf("respuesta TSA inválida: %v", err)
					}
					if sello.Time.Before(time.Now().Add(-time.Minute)) || sello.Time.After(time.Now().Add(time.Minute)) {
						t.Fatalf("GenTime TSA fuera del ensayo: %v", sello.Time)
					}
					if !bytes.Contains(cms, sello.RawToken) {
						t.Fatal("el PDF no contiene el token RFC 3161 recibido")
					}
					p7, err := pkcs7.Parse(cms)
					if err != nil || len(p7.Signers) != 1 {
						t.Fatalf("CMS inválido: %v", err)
					}
					huella := sha256.Sum256(p7.Signers[0].EncryptedDigest)
					if !bytes.Equal(huella[:], sello.HashedMessage) {
						t.Fatal("el sello no está ligado a la firma CMS")
					}
					consulta := escribirLocal(t, dir, "tsa-solicitud.tsq", peticion)
					respuestaRuta := escribirLocal(t, dir, "tsa-respuesta.tsr", respuesta)
					tsaCert := escribirLocal(t, dir, "tsa.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: sello.Certificates[0].Raw}))
					if exttools.Available(t, "openssl") {
						ejecutarLocal(t, "openssl", "ts", "-verify", "-queryfile", consulta, "-in", respuestaRuta, "-CAfile", tsaCert)
					}
				}
			case "cades":
				validarCMSLocal(t, dir, resultado.Data, contenido)
			case "xades", "facturae":
				firmadoDoc := domain.Document{Name: "firmado.xml", MIMEType: mime, Content: resultado.Data}
				if formato == "facturae" {
					ver, _, err := commonsigner.NewFacturaEVerifier().Verify(context.Background(), firmadoDoc, domain.CertificateChain{})
					if err != nil || !ver.Valid {
						t.Fatalf("regresión FacturaE: %+v, %v", ver, err)
					}
				} else {
					ver, _, err := commonsigner.NewXAdESVerifier().Verify(context.Background(), firmadoDoc, domain.CertificateChain{})
					if err != nil || !ver.Valid {
						t.Fatalf("regresión XAdES: %+v, %v", ver, err)
					}
				}
				if exttools.Available(t, "xmlsec1") {
					ejecutarLocal(t, "xmlsec1", "--verify", "--trusted-pem", filepath.Join(dir, "firmante.pem"), "--enabled-reference-uris", "empty,same-doc", "--id-attr:Id", "http://uri.etsi.org/01903/v1.3.2#:SignedProperties", "--id-attr:Id", "http://www.w3.org/2000/09/xmldsig#:Signature", "--id-attr:Id", "http://www.w3.org/2000/09/xmldsig#:KeyInfo", "--id-attr:Id", "http://www.w3.org/2000/09/xmldsig#:Object", "--id-attr:Id", "doc", "--id-attr:Id", "Facturae", firmado)
				}
			}
		})
	}
}

func escribirLocal(t *testing.T, dir, nombre string, datos []byte) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func ejecutarLocal(t *testing.T, programa string, args ...string) string {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	cmd := exec.CommandContext(ctx, programa, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	salida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", programa, args, err, salida)
	}
	t.Logf("%s: %s", programa, strings.TrimSpace(string(salida)))
	return string(salida)
}

func validarCMSLocal(t *testing.T, dir string, cms, original []byte) {
	t.Helper()
	ruta := escribirLocal(t, dir, "firma.der", cms)
	contenido := escribirLocal(t, dir, "contenido.bin", original)
	if exttools.Available(t, "openssl") {
		args := []string{"cms", "-verify", "-binary", "-inform", "DER", "-in", ruta, "-content", contenido, "-CAfile", filepath.Join(dir, "firmante.pem"), "-purpose", "any", "-out", filepath.Join(dir, "verificado.bin")}
		ejecutarLocal(t, "openssl", args...)
		verificado, err := os.ReadFile(filepath.Join(dir, "verificado.bin"))
		if err != nil || !bytes.Equal(verificado, original) {
			t.Fatalf("OpenSSL no recupera el contenido original: %v", err)
		}
		alterado := append([]byte(nil), original...)
		alterado[0] ^= 1
		escribirLocal(t, dir, "contenido.bin", alterado)
		ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelar()
		negativos := append([]string(nil), args...)
		negativos[len(negativos)-1] = os.DevNull
		cmd := exec.CommandContext(ctx, "openssl", negativos...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		if salida, err := cmd.CombinedOutput(); err == nil || !bytes.Contains(salida, []byte("CMS Verification failure")) {
			t.Fatalf("OpenSSL debía rechazar criptográficamente el contenido manipulado: %v\n%s", err, salida)
		}
		escribirLocal(t, dir, "contenido.bin", original)
	}
}

func extraerCMSLocal(t *testing.T, pdf []byte) ([]byte, []byte) {
	t.Helper()
	coincidencias := regexp.MustCompile(`/ByteRange\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*\]`).FindSubmatch(pdf)
	if len(coincidencias) != 5 {
		t.Fatal("falta ByteRange PDF")
	}
	var a, b, c, d int
	if _, err := fmt.Sscanf(string(bytes.Join(coincidencias[1:], []byte(" "))), "%d %d %d %d", &a, &b, &c, &d); err != nil {
		t.Fatal(err)
	}
	if a != 0 || b < 0 || c <= b || d < 0 || c+d != len(pdf) {
		t.Fatal("ByteRange no cubre el documento")
	}
	bruto, err := hex.DecodeString(strings.Trim(string(pdf[b:c]), "<> \r\n"))
	if err != nil {
		t.Fatal(err)
	}
	var der asn1.RawValue
	if _, err := asn1.Unmarshal(bruto, &der); err != nil {
		t.Fatal(err)
	}
	return der.FullBytes, append(append([]byte(nil), pdf[:b]...), pdf[c:c+d]...)
}

type transporteTSALocal func(*http.Request) (*http.Response, error)

func (f transporteTSALocal) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// RFC 3161 exige que extendedKeyUsage sea crítico y solo contenga timestamping.
func nuevaTSALocal(t *testing.T) func([]byte) ([]byte, error) {
	t.Helper()
	priv, base := generarCertRSAConformance(t)
	eku, err := asn1.Marshal([]asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 8}})
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{SerialNumber: base.SerialNumber, Subject: pkix.Name{CommonName: "TSA sintética local"}, NotBefore: base.NotBefore, NotAfter: base.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Critical: true, Value: eku}}}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return func(consulta []byte) ([]byte, error) {
		req, err := timestamp.ParseRequest(consulta)
		if err != nil {
			return nil, err
		}
		resp := &timestamp.Timestamp{HashAlgorithm: req.HashAlgorithm, HashedMessage: req.HashedMessage, Nonce: req.Nonce, Time: time.Now().UTC(), Policy: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 26}, AddTSACertificate: true}
		return resp.CreateResponseWithOpts(cert, priv, crypto.SHA256)
	}
}
