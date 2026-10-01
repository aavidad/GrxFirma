// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestCertificateExportPublic_DERyPEMSinClavePrivada(t *testing.T) {
	der := generarCertificadoExportableIPC(t, x509.KeyUsageKeyEncipherment)
	m := &Manejador{Catalogo: &stubCatalogo{certs: []domain.CertificateRef{{
		ID: "mio", DER: der, HasSigningKey: true, HasLocalDecryptionKey: true,
	}}}}
	for _, format := range []string{"der", "pem"} {
		ext := ".cer"
		if format == "pem" {
			ext = ".pem"
		}
		path := filepath.Join(t.TempDir(), "publico"+ext)
		params, _ := json.Marshal(map[string]string{
			"certificateId": "mio", "outputPath": path, "format": format,
		})
		response := m.despachar(context.Background(), peticion{Action: "certificate_export_public", Params: params})
		if !response.OK {
			t.Fatalf("%s: %s", format, response.Error)
		}
		data := response.Data.(map[string]any)
		returned, err := base64.StdEncoding.DecodeString(data["certificateDerBase64"].(string))
		if err != nil || !bytes.Equal(returned, der) {
			t.Fatalf("DER devuelto distinto: %v", err)
		}
		file, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(file, []byte("PRIVATE KEY")) {
			t.Fatal("salida con clave privada")
		}
		if format == "pem" {
			block, rest := pem.Decode(file)
			if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 || !bytes.Equal(block.Bytes, der) {
				t.Fatal("PEM no contiene exclusivamente el certificado")
			}
		} else if !bytes.Equal(file, der) {
			t.Fatal("fichero DER distinto al certificado")
		}
		if _, err := x509.ParseCertificate(returned); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCertificateExportPublic_RechazaCertificadoNoAptoYRutaInsegura(t *testing.T) {
	der := generarCertificadoExportableIPC(t, x509.KeyUsageDigitalSignature)
	m := &Manejador{Catalogo: &stubCatalogo{certs: []domain.CertificateRef{{
		ID: "firma", DER: der, HasSigningKey: true, HasLocalDecryptionKey: true,
	}}}}
	path := filepath.Join(t.TempDir(), "no-creado.cer")
	params, _ := json.Marshal(map[string]string{"certificateId": "firma", "outputPath": path})
	response := m.despachar(context.Background(), peticion{Action: "certificate_export_public", Params: params})
	if response.OK || !strings.Contains(response.Error, "keyEncipherment") || response.Diagnostic == nil {
		t.Fatalf("rechazo KeyUsage sin motivo claro: %+v", response)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("se creó salida no apta")
	}
	m.Catalogo = &stubCatalogo{certs: []domain.CertificateRef{{ID: "firma", DER: der}}}
	response = m.despachar(context.Background(), peticion{Action: "certificate_export_public", Params: params})
	if response.OK || !strings.Contains(response.Error, "clave privada no está disponible") {
		t.Fatalf("certificado ajeno aceptado: %+v", response)
	}
	m.Catalogo = &stubCatalogo{certs: []domain.CertificateRef{{ID: "firma", DER: generarCertificadoExportableIPC(t, x509.KeyUsageKeyEncipherment), HasSigningKey: true, HasLocalDecryptionKey: true}}}
	params, _ = json.Marshal(map[string]string{"certificateId": "firma", "outputPath": "/etc/ssh/clave.cer"})
	response = m.despachar(context.Background(), peticion{Action: "certificate_export_public", Params: params})
	if response.OK {
		t.Fatal("ruta prohibida aceptada")
	}
	target := filepath.Join(t.TempDir(), "original.cer")
	if err := os.WriteFile(target, []byte("conservar"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "enlace.cer")
	if err := os.Symlink(target, link); err == nil {
		params, _ = json.Marshal(map[string]string{"certificateId": "firma", "outputPath": link})
		response = m.despachar(context.Background(), peticion{Action: "certificate_export_public", Params: params})
		if response.OK {
			t.Fatal("enlace simbólico de salida aceptado")
		}
		content, err := os.ReadFile(target)
		if err != nil || string(content) != "conservar" {
			t.Fatal("se alteró el destino del enlace")
		}
	}
}

func TestCertificateExportPublic_RechazaClavesNoRSAOInsuficientes(t *testing.T) {
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []crypto.Signer{weak, ec} {
		der := certificarClavePublicaIPC(t, key, x509.KeyUsageKeyEncipherment)
		m := &Manejador{Catalogo: &stubCatalogo{certs: []domain.CertificateRef{{ID: "mio", DER: der, HasSigningKey: true, HasLocalDecryptionKey: true}}}}
		response := m.despachar(context.Background(), peticion{Action: "certificate_export_public", Params: json.RawMessage(`{"certificateId":"mio"}`)})
		if response.OK || !strings.Contains(response.Error, "RSA de al menos 2048 bits") {
			t.Fatalf("clave no compatible aceptada o motivo ausente: %+v", response)
		}
	}
}

func generarCertificadoExportableIPC(t *testing.T, usage x509.KeyUsage) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return certificarClavePublicaIPC(t, key, usage)
}

func certificarClavePublicaIPC(t *testing.T, key crypto.Signer, usage x509.KeyUsage) []byte {
	t.Helper()
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(812), Subject: pkix.Name{CommonName: "Prueba sintética"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: usage,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}
