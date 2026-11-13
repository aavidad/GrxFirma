// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// afpDeAtacante fabrica un .afp como lo haría cualquiera que conozca la clave
// pública del destinatario: el sobre no autentica al emisor, así que elige
// libremente el nombre, el MIME y la versión.
func afpDeAtacante(t *testing.T, version int, nombre, mime string, pub *rsa.PublicKey) domain.Document {
	t.Helper()
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	cek := make([]byte, 32)
	nonce := make([]byte, 12)
	if _, err := rand.Read(cek); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	recipient, err := wrapCompat(domain.ProtectionRecipient{ID: "dest", RSAOAEP256PublicKeyDER: pubDER}, cek)
	if err != nil {
		t.Fatal(err)
	}
	env := protectedEnvelope{
		Version:    version,
		Profile:    string(domain.ProtectionProfileCompat),
		Algorithm:  contentAEADAlg,
		Name:       nombre,
		MIMEType:   mime,
		NonceB64:   base64.StdEncoding.EncodeToString(nonce),
		Recipients: []protectedRecipientEnvelope{recipient},
	}
	cipherText, err := encryptAESGCM(cek, nonce, []byte("<script>alert(1)</script>"), buildContentAAD(env))
	if err != nil {
		t.Fatal(err)
	}
	env.CipherB64 = base64.StdEncoding.EncodeToString(cipherText)
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewDocument("factura.pdf.afp", raw, domain.MIMETypeProtectedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func claveCompat(t *testing.T, priv *rsa.PrivateKey) []domain.ProtectionKeyMaterial {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return []domain.ProtectionKeyMaterial{{RecipientID: "dest", RSAOAEP256PrivateKeyPKCS8: der}}
}

// F-08: el nombre y el MIME de un .afp vienen de un emisor no autenticado.
// Unprotect no debe devolver separadores, controles ni una extensión que se
// ejecute al hacer doble clic.
func TestUnprotect_SaneaNombreYMIMEDelEmisor(t *testing.T) {
	t.Parallel()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		nombre, mime, wantNombre, wantMIME string
	}{
		{`..\x.hta`, "text/html", "x.bin", "text/html"},
		{"../../.config/autostart/evil.desktop", "application/x-desktop", "evil.bin", "application/x-desktop"},
		{"factura‮fdp.lnk", "application/pdf\nmime=x", "facturafdp.bin", "application/octet-stream"},
		{"informe.hta.", "", "informe.bin", "application/octet-stream"},
		{"sin-extension", "text/plain", "sin-extension.bin", "text/plain"},
		{"..", "application/pdf", "documento.bin", "application/pdf"},
		{"Factura 2026.PDF", "application/pdf; charset=binary", "Factura 2026.PDF", "application/pdf; charset=binary"},
		{"C:datos.xml", "application/xml", "C_datos.xml", "application/xml"},
	}
	engine := NuevoEnvelopeProtector()
	for _, caso := range casos {
		doc := afpDeAtacante(t, envelopeVersion, caso.nombre, caso.mime, &priv.PublicKey)
		out, err := engine.Unprotect(context.Background(), doc, claveCompat(t, priv))
		if err != nil {
			t.Fatalf("Unprotect(%q) error = %v", caso.nombre, err)
		}
		if out.Document.Name != caso.wantNombre || out.Document.MIMEType != caso.wantMIME {
			t.Errorf("Unprotect(%q, %q) = %q, %q; want %q, %q", caso.nombre, caso.mime,
				out.Document.Name, out.Document.MIMEType, caso.wantNombre, caso.wantMIME)
		}
		if strings.ContainsAny(out.Document.Name, `/\`) {
			t.Errorf("el nombre conserva separadores: %q", out.Document.Name)
		}
	}
}

// F-08: en v2 la AAD lleva cada campo con su longitud, así que no se puede
// pasar texto del nombre al MIME sin cambiarla. Los sobres v1 se siguen
// leyendo.
func TestContentAAD_V2SinAmbiguedadYV1SeLee(t *testing.T) {
	t.Parallel()
	if envelopeVersion != 2 {
		t.Fatalf("Protect escribe sobres v%d; want v2", envelopeVersion)
	}
	a := protectedEnvelope{Version: 2, Profile: "compat", Name: "a\nmime=b", MIMEType: ""}
	b := protectedEnvelope{Version: 2, Profile: "compat", Name: "a", MIMEType: "b\nmime="}
	if bytes.Equal(buildContentAAD(a), buildContentAAD(b)) {
		t.Fatal("la AAD v2 no distingue qué texto es nombre y cuál MIME")
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	engine := NuevoEnvelopeProtector()
	v1 := afpDeAtacante(t, 1, "antiguo.pdf", "application/pdf", &priv.PublicKey)
	if out, err := engine.Unprotect(context.Background(), v1, claveCompat(t, priv)); err != nil || out.Document.Name != "antiguo.pdf" {
		t.Fatalf("un sobre v1 debe seguir abriéndose: %+v, %v", out.Document.Name, err)
	}
	v9 := afpDeAtacante(t, 9, "futuro.pdf", "application/pdf", &priv.PublicKey)
	if _, err := engine.Unprotect(context.Background(), v9, claveCompat(t, priv)); err == nil {
		t.Fatal("se aceptó un sobre de versión desconocida")
	}
}
