// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

func entornoConfianzaForzada(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
}

// firmaAutofirmadaPrueba firma con un certificado que ninguna raíz del
// equipo respalda y sin datos de revocación: los dos obstáculos que el
// usuario puede aceptar.
func firmaAutofirmadaPrueba(t *testing.T) domain.Document {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ahora := time.Now()
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1414), Subject: pkix.Name{CommonName: "Firmante lote 14", Organization: []string{"Pruebas"}},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	datos, err := pkcs7.NewSignedData([]byte("documento del lote 14"))
	if err != nil {
		t.Fatal(err)
	}
	datos.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := datos.AddSigner(cert, clave, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	cms, err := datos.Finish()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewDocument("firma.csig", cms, "application/pkcs7-signature")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// Lote 14: lo aceptado en config.json vale en la CLI local y no en el
// servidor REST, que compone sus servicios sin la confianza forzada.
func TestConfianzaForzada_CLIAplicaYRESTNo(t *testing.T) {
	entornoConfianzaForzada(t)
	doc := firmaAutofirmadaPrueba(t)
	ctx := context.Background()

	adaptador, err := construirAdaptador("", "", "", "")
	if err != nil {
		t.Fatalf("construirAdaptador: %v", err)
	}
	inicial, err := adaptador.Verificar.Execute(ctx, application.VerifyCommand{SignedDocument: doc})
	if err != nil {
		t.Fatal(err)
	}
	if inicial.Verification.Valid {
		t.Fatal("una firma autofirmada no puede ser válida sin aceptar nada")
	}
	if len(inicial.ConfianzaForzada.Forzable) == 0 {
		t.Fatalf("la CLI debe ofrecer aceptar la firma: avisos=%v", inicial.Verification.Warnings)
	}
	servicios, err := construirServicios("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	almacen := config.NuevoAlmacenConfianzaForzada(servicios.configDir)
	for _, o := range inicial.ConfianzaForzada.Forzable {
		if err := almacen.AceptarConfianzaForzada(ctx, domain.AceptacionConfianza{Tipo: o.Tipo, Huella: o.Huella, Sujeto: o.Sujeto, AceptadaEn: time.Now()}); err != nil {
			t.Fatalf("guardar la aceptación: %v", err)
		}
	}

	cli, err := adaptador.Verificar.Execute(ctx, application.VerifyCommand{SignedDocument: doc})
	if err != nil {
		t.Fatal(err)
	}
	if !cli.Verification.Valid || len(cli.ConfianzaForzada.Aplicada) == 0 {
		t.Fatalf("en la CLI lo aceptado debe aplicarse: valid=%v %+v avisos=%v", cli.Verification.Valid, cli.ConfianzaForzada, cli.Verification.Warnings)
	}

	// El servidor REST (runRESTServer) usa construirServicios tal cual.
	rest, err := servicios.verificar.Execute(ctx, application.VerifyCommand{SignedDocument: doc})
	if err != nil {
		t.Fatal(err)
	}
	if rest.Verification.Valid {
		t.Fatal("el servidor REST aplicó la confianza aceptada en el equipo")
	}
	if len(rest.ConfianzaForzada.Forzable) != 0 || len(rest.ConfianzaForzada.Aplicada) != 0 {
		t.Fatalf("el servidor REST no puede leer ni ofrecer la confianza aceptada: %+v", rest.ConfianzaForzada)
	}
	for _, aviso := range rest.Verification.Warnings {
		if aviso == domain.ClavePistaConfianzaForzable {
			t.Fatal("el servidor REST no puede sugerir aceptar la firma")
		}
	}
}
