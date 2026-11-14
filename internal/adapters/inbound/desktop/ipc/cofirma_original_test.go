// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// cadesPruebaIPC firma datos en CAdES (explícita por defecto).
func cadesPruebaIPC(t *testing.T, datos []byte, implicita bool) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(17),
		Subject:      pkix.Name{CommonName: "Firmante KD-03"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewDocument("acta.txt", datos, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	opciones := map[string]string{}
	if implicita {
		opciones["mode"] = "implicit"
	}
	res, err := signer.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign, Options: opciones,
	}, &signer.LocalSigningKey{ID: "a", Signer: key, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return res.Data
}

func escribirPruebaIPC(t *testing.T, dir, nombre string, datos []byte) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// Lote 17b (KD-03): Qt y WinUI preguntan al motor, antes de cofirmar o
// contrafirmar, si la firma no lleva el documento dentro.
func TestCofirmaOriginalCheckDetectaLaCAdESExplicita(t *testing.T) {
	dir := t.TempDir()
	explicita := escribirPruebaIPC(t, dir, "explicita.csig", cadesPruebaIPC(t, []byte("acta"), false))
	implicita := escribirPruebaIPC(t, dir, "implicita.csig", cadesPruebaIPC(t, []byte("acta"), true))
	texto := escribirPruebaIPC(t, dir, "acta.txt", []byte("acta"))
	m := manejadorVacio()
	for _, caso := range []struct {
		nombre, ruta, formato, accion string
		want                          bool
	}{
		{"cofirma_explicita", explicita, "CAdES", "cosign", true},
		{"contrafirma_explicita", explicita, "", "countersign", true},
		{"cofirma_explicita_auto", explicita, "auto", "cosign", true},
		{"firma_simple", explicita, "CAdES", "sign", false},
		{"cofirma_implicita", implicita, "CAdES", "cosign", false},
		{"no_es_cms", texto, "CAdES", "cosign", false},
		{"otro_formato", explicita, "xades", "cosign", false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			resp := m.despachar(context.Background(), peticionJSON(t, accionCofirmaOriginal, paramsCofirmaOriginal{
				InputPath: caso.ruta, Format: caso.formato, Action: caso.accion,
			}))
			if !resp.OK {
				t.Fatal(resp.Error)
			}
			if got := resp.Data.(resultadoCofirmaOriginal).NeedsOriginal; got != caso.want {
				t.Fatalf("needsOriginal = %v, want %v", got, caso.want)
			}
		})
	}
	if !slices.Contains(desktopIPCHello().Actions, accionCofirmaOriginal) {
		t.Fatal("hello debe anunciar la consulta previa a la cofirma")
	}
}

type firmaQueGuardaOriginal struct {
	original []byte
	err      error
}

func (f *firmaQueGuardaOriginal) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	f.original = append([]byte(nil), cmd.OriginalContent...)
	if f.err != nil {
		return application.SignResult{}, f.err
	}
	return application.SignResult{Result: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("QA firmado")}}, nil
}

// «sign» pasa al motor el original elegido solo al cofirmar o contrafirmar,
// y explica en el idioma de la persona por qué no firma si no corresponde.
func TestFirmaIPCPasaElOriginalDeLaCofirma(t *testing.T) {
	dir := t.TempDir()
	entrada := escribirPruebaIPC(t, dir, "firma.csig", []byte("QA firma"))
	original := escribirPruebaIPC(t, dir, "acta.pdf", []byte("QA original"))
	peticion := func(accion string) peticion {
		return peticionJSON(t, "sign", paramsFirma{
			InputPath: entrada, OutputPath: filepath.Join(dir, "salida-"+accion+".csig"),
			CertificateID: "qa", Format: "CAdES", Action: accion, OriginalPath: original,
		})
	}
	for _, caso := range []struct {
		accion string
		want   []byte
	}{
		{"cosign", []byte("QA original")},
		{"countersign", []byte("QA original")},
		{"sign", nil},
	} {
		firma := &firmaQueGuardaOriginal{}
		m := &Manejador{Firmar: firma, ultimosCerts: []domain.CertificateRef{{ID: "qa"}}}
		if resp := m.despachar(context.Background(), peticion(caso.accion)); !resp.OK {
			t.Fatalf("%s: %s", caso.accion, resp.Error)
		}
		if !bytes.Equal(firma.original, caso.want) {
			t.Fatalf("%s: original = %q, want %q", caso.accion, firma.original, caso.want)
		}
	}

	loc := localizador.Para("es")
	for err, clave := range map[error]string{
		fmt.Errorf("error durante la operacion de firma: %w", signer.ErrFirmaNoCorrespondeOriginal): "firma.cofirma_sin_original.original_no_corresponde",
		fmt.Errorf("error durante la operacion de firma: %w", signer.ErrCotejoSHA1Prohibido):        "firma.sha1_corresponde.prohibido",
	} {
		m := &Manejador{Firmar: &firmaQueGuardaOriginal{err: err}, Loc: loc, ultimosCerts: []domain.CertificateRef{{ID: "qa"}}}
		resp := m.despachar(context.Background(), peticion("cosign"))
		if resp.OK || resp.Error != loc.T(clave) || resp.Error == clave {
			t.Fatalf("error = %q, want el texto de %s", resp.Error, clave)
		}
	}
}
