// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto/x509"
	"encoding/asn1"
	"strings"
	"testing"

	"github.com/deatil/go-cryptobin/ber"
	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

// Lote 17a, F-06: un EnvelopedData con un destinatario ktri RSA nuestro y
// otros de tipo kari o kekri (otro programa cifró también para quien usa
// acuerdo de claves o una clave compartida).
var (
	// KeyAgreeRecipientInfo [1] y KEKRecipientInfo [2]: basta su forma; nadie
	// los lee.
	destinatarioKARIPrueba  = []byte{0xa1, 0x03, 0x02, 0x01, 0x03}
	destinatarioKEKRIPrueba = []byte{0xa2, 0x03, 0x02, 0x01, 0x04}
)

// conDestinatarios rehace el sobre con los destinatarios dados alrededor de
// los que ya tenía: antes van delante y despues detrás.
func conDestinatarios(t *testing.T, doc domain.Document, antes, despues [][]byte, quitarPropios bool) domain.Document {
	t.Helper()
	der, err := ber.Ber2der(doc.Content)
	if err != nil {
		t.Fatal(err)
	}
	var info cmsContentInfo
	if _, err := asn1.Unmarshal(der, &info); err != nil {
		t.Fatal(err)
	}
	var sobre asn1.RawValue
	if _, err := asn1.Unmarshal(info.Content.Bytes, &sobre); err != nil {
		t.Fatal(err)
	}
	var version int
	resto, err := asn1.Unmarshal(sobre.Bytes, &version)
	if err != nil {
		t.Fatal(err)
	}
	var set asn1.RawValue
	if resto, err = asn1.Unmarshal(resto, &set); err != nil || set.Tag != asn1.TagSet {
		t.Fatalf("se esperaban los destinatarios: %v", err)
	}
	var contenido []byte
	for _, d := range antes {
		contenido = append(contenido, d...)
	}
	if !quitarPropios {
		contenido = append(contenido, set.Bytes...)
	}
	for _, d := range despues {
		contenido = append(contenido, d...)
	}
	nuevoSet, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: contenido})
	if err != nil {
		t.Fatal(err)
	}
	// Con kari, la versión de EnvelopedData es 2 (RFC 5652, 6.1).
	v2, _ := asn1.Marshal(2)
	cuerpo := append(append(v2, nuevoSet...), resto...)
	env, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: cuerpo})
	if err != nil {
		t.Fatal(err)
	}
	blob, err := contentInfoCMS(info.ContentType, env)
	if err != nil {
		t.Fatal(err)
	}
	doc.Content = blob
	return doc
}

func claveRSAPrueba(t *testing.T) ([]domain.ProtectionKeyMaterial, []byte) {
	t.Helper()
	priv, certDER := destinatarioCMSPrueba(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return []domain.ProtectionKeyMaterial{{RecipientID: "dest", CertificateDER: certDER, RSAOAEP256PrivateKeyPKCS8: pkcs8}}, certDER
}

func TestCMSUnprotect_DestinatariosAjenosSeIgnoranConAviso(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, false)
	claves, certDER := claveRSAPrueba(t)
	base := envelopedConCifrado(t, cryptobinpkcs7.AES256GCM, certDER)
	casos := map[string]struct {
		doc     domain.Document
		detalle string
	}{
		"ajenos detrás":   {conDestinatarios(t, base, nil, [][]byte{destinatarioKARIPrueba, destinatarioKEKRIPrueba}, false), "kari,kekri"},
		"ajeno delante":   {conDestinatarios(t, base, [][]byte{destinatarioKEKRIPrueba}, nil, false), "kekri"},
		"a los dos lados": {conDestinatarios(t, base, [][]byte{destinatarioKARIPrueba}, [][]byte{destinatarioKEKRIPrueba}, false), "kari,kekri"},
	}
	for nombre, caso := range casos {
		out, err := NuevoCMSProtector().Unprotect(context.Background(), caso.doc, claves)
		if err != nil {
			t.Errorf("%s: con un destinatario nuestro válido el sobre se abre: %v", nombre, err)
			continue
		}
		if string(out.Document.Content) != contenidoCMSPrueba {
			t.Errorf("%s: contenido %q", nombre, out.Document.Content)
		}
		if len(out.Avisos) != 1 || out.Avisos[0] != avisoCMSDestinatariosAjenos {
			t.Errorf("%s: falta el aviso visible: %v", nombre, out.Avisos)
		}
		if len(out.Detalles) != 1 || out.Detalles[0] != detalleCMSDestinatariosAjenos+caso.detalle {
			t.Errorf("%s: falta el detalle de auditoría: %v", nombre, out.Detalles)
		}
	}

	// Con un cifrado heredado, los dos avisos.
	heredado := conDestinatarios(t, envelopedConCifrado(t, cryptobinpkcs7.DESEDE3CBC, certDER), nil, [][]byte{destinatarioKARIPrueba}, false)
	out, err := NuevoCMSProtector().Unprotect(context.Background(), heredado, claves)
	if err != nil || len(out.Avisos) != 2 || len(out.Detalles) != 2 {
		t.Fatalf("heredado y ajeno: %v %v %v", err, out.Avisos, out.Detalles)
	}

	// Sin ajenos, nada cambia: ni aviso ni sobre reescrito.
	out, err = NuevoCMSProtector().Unprotect(context.Background(), base, claves)
	if err != nil || len(out.Avisos) != 0 || len(out.Detalles) != 0 {
		t.Fatalf("sin ajenos: %v %v", err, out.Avisos)
	}
	if revision, err := revisarCMS(base.Content); err != nil || revision.sinAjenos != nil || revision.ajenos != nil {
		t.Fatalf("sin ajenos no se reescribe el sobre: %v %+v", err, revision)
	}
}

func TestCMSUnprotect_SinDestinatarioNuestroUtilizableSigueSiendoError(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, false)
	claves, certDER := claveRSAPrueba(t)
	base := envelopedConCifrado(t, cryptobinpkcs7.AES256GCM, certDER)

	// Solo ajenos: el error de siempre.
	soloAjenos := conDestinatarios(t, base, nil, [][]byte{destinatarioKARIPrueba, destinatarioKEKRIPrueba}, true)
	if _, err := NuevoCMSProtector().Unprotect(context.Background(), soloAjenos, claves); err == nil || !strings.Contains(err.Error(), "solo se admiten destinatarios CMS con transporte de clave RSA") {
		t.Fatalf("solo ajenos: %v", err)
	}

	// Un ktri para otro certificado y un ajeno: con nuestra clave no se abre.
	otras, _ := claveRSAPrueba(t)
	ajeno := conDestinatarios(t, base, nil, [][]byte{destinatarioKARIPrueba}, false)
	if _, err := NuevoCMSProtector().Unprotect(context.Background(), ajeno, otras); err == nil || !strings.Contains(err.Error(), "no se pudo desproteger el sobre CMS") {
		t.Fatalf("sin destinatario nuestro: %v", err)
	}

	// Un ktri que no es RSA sigue rechazándose aunque haya ajenos.
	noRSA := ajeno
	noRSA.Content = append([]byte(nil), ajeno.Content...)
	rsa := []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01}
	i := strings.Index(string(noRSA.Content), string(rsa))
	if i < 0 {
		t.Fatal("falta rsaEncryption")
	}
	noRSA.Content[i+len(rsa)-1] = 0x0b
	if _, err := NuevoCMSProtector().Unprotect(context.Background(), noRSA, claves); err == nil || !strings.Contains(err.Error(), "no permitido") {
		t.Fatalf("ktri no RSA: %v", err)
	}

	// Quitar los ajenos no abre la puerta a un contenido alterado: AES-GCM
	// lo detecta.
	alterado := ajeno
	alterado.Content = append([]byte(nil), ajeno.Content...)
	alterado.Content[len(alterado.Content)-3] ^= 0x01
	if _, err := NuevoCMSProtector().Unprotect(context.Background(), alterado, claves); err == nil {
		t.Fatal("un contenido cifrado alterado no puede abrirse")
	}

	// Una etiqueta de destinatario desconocida ([5]) es un error, como antes.
	raro := conDestinatarios(t, base, nil, [][]byte{{0xa5, 0x03, 0x02, 0x01, 0x00}}, false)
	if _, err := NuevoCMSProtector().Unprotect(context.Background(), raro, claves); err == nil {
		t.Fatal("un destinatario de tipo desconocido es un error")
	}
}
