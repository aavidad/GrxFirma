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
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
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
	excepciones.SetForTest(t, "afp_aad_ambigua", true)
	machinepolicy.UnsetForTest(t, "permitir_afp_aad_ambigua")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		nombre, mime, wantNombre, wantMIME string
	}{
		{`..\x.hta`, "text/html", "x.bin", "text/html"},
		{"../../.config/autostart/evil.desktop", "application/x-desktop", "evil.bin", "application/x-desktop"},
		{"factura\u202efdp.lnk", "application/pdf\nmime=x", "facturafdp.bin", "application/octet-stream"},
		{"informe.hta.", "", "informe.bin", "application/octet-stream"},
		{"sin-extension", "text/plain", "sin-extension.bin", "text/plain"},
		{"..", "application/pdf", "documento.bin", "application/pdf"},
		{"Factura 2026.PDF", "application/pdf; charset=binary", "Factura 2026.PDF", "application/pdf; charset=binary"},
		{"C:datos.xml", "application/xml", "C_datos.xml", "application/xml"},
		{"con.pdf", "application/pdf", "_con.pdf", "application/pdf"},
		{"LPT1.txt", "text/plain", "_LPT1.txt", "text/plain"},
		{"consola.pdf", "application/pdf", "consola.pdf", "application/pdf"},
	}
	engine := NuevoEnvelopeProtector()
	for _, version := range []int{envelopeVersionV1, envelopeVersionV2} {
		for _, caso := range casos {
			doc := afpDeAtacante(t, version, caso.nombre, caso.mime, &priv.PublicKey)
			out, err := engine.Unprotect(context.Background(), doc, claveCompat(t, priv))
			if err != nil {
				t.Fatalf("Unprotect(v%d, %q) error = %v", version, caso.nombre, err)
			}
			if out.Document.Name != caso.wantNombre || out.Document.MIMEType != caso.wantMIME {
				t.Errorf("Unprotect(v%d, %q, %q) = %q, %q; want %q, %q", version, caso.nombre, caso.mime,
					out.Document.Name, out.Document.MIMEType, caso.wantNombre, caso.wantMIME)
			}
			if strings.ContainsAny(out.Document.Name, `/\`) {
				t.Errorf("el nombre conserva separadores: %q", out.Document.Name)
			}
		}
	}
}

// F-08: en v2 la AAD lleva cada campo con su longitud, así que no se puede
// pasar texto del nombre al MIME sin cambiarla.
func TestContentAAD_V2SinAmbiguedad(t *testing.T) {
	t.Parallel()
	a := protectedEnvelope{Version: 2, Profile: "compat", Name: "a\nmime=b", MIMEType: ""}
	b := protectedEnvelope{Version: 2, Profile: "compat", Name: "a", MIMEType: "b\nmime="}
	if bytes.Equal(buildContentAAD(a), buildContentAAD(b)) {
		t.Fatal("la AAD v2 no distingue qué texto es nombre y cuál MIME")
	}
}

func protegerAFP(t *testing.T, nombre, mime string, options map[string]string, pub *rsa.PublicKey) (protectedEnvelope, domain.Document) {
	t.Helper()
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewDocument(nombre, []byte("contenido del expediente"), mime)
	if err != nil {
		t.Fatal(err)
	}
	out, err := NuevoEnvelopeProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc, Profile: domain.ProtectionProfileCompat, Options: options,
	}, []domain.ProtectionRecipient{{ID: "dest", RSAOAEP256PublicKeyDER: pubDER}})
	if err != nil {
		t.Fatal(err)
	}
	var env protectedEnvelope
	if err := json.Unmarshal(out.Document.Content, &env); err != nil {
		t.Fatal(err)
	}
	return env, out.Document
}

// Lote 13b, punto 5: los .afp que escribía la v2 no los abría una versión
// anterior de GrxFirma. Ahora se escribe v1 por defecto, sin saltos de línea
// en el nombre ni en el MIME, de modo que la AAD v1 solo tiene una lectura y
// el lector nuevo recibe el nombre y el MIME autenticados. La v2 queda como
// opción explícita (contenedor «json-v2»).
func TestProtect_EscribeV1InequivocaPorDefectoYV2APeticion(t *testing.T) {
	t.Parallel()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	engine := NuevoEnvelopeProtector()

	env, doc := protegerAFP(t, "acta\nmime=text/html.pdf", "application/pdf\nx", nil, &priv.PublicKey)
	if env.Version != envelopeVersionV1 {
		t.Fatalf("Protect escribe v%d por defecto; want v1, la que abren las versiones anteriores", env.Version)
	}
	if strings.ContainsAny(env.Name+env.MIMEType, "\r\n") || env.MIMEType != "application/octet-stream" {
		t.Fatalf("la v1 no puede llevar saltos de línea en sus campos: %q %q", env.Name, env.MIMEType)
	}
	if aadV1Ambigua(env) {
		t.Fatal("Protect escribió una AAD v1 ambigua")
	}
	out, err := engine.Unprotect(context.Background(), doc, claveCompat(t, priv))
	if err != nil || len(out.Avisos) != 0 || len(out.Detalles) != 0 {
		t.Fatalf("un .afp v1 propio se abre sin avisos: %v %v %v", err, out.Avisos, out.Detalles)
	}

	env, doc = protegerAFP(t, "acta.pdf", "application/pdf", map[string]string{"container": "json-v2"}, &priv.PublicKey)
	if env.Version != envelopeVersionV2 {
		t.Fatalf("el contenedor json-v2 debe escribir v2; escribió v%d", env.Version)
	}
	if out, err := engine.Unprotect(context.Background(), doc, claveCompat(t, priv)); err != nil || out.Document.Name != "acta.pdf" || len(out.Avisos) != 0 {
		t.Fatalf("el lector abre v2 sin avisos: %+v %v", out, err)
	}
}

// Un .afp v1 con saltos de línea en el nombre o el MIME (de una versión
// anterior o de quien quiere mover texto entre campos) se abre saneado, con
// aviso visible y con el detalle para la auditoría. Uno de versión
// desconocida sigue rechazado.
func TestUnprotect_V1AmbiguaSeAbreConAviso(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	engine := NuevoEnvelopeProtector()
	v1 := afpDeAtacante(t, 1, "antiguo.pdf", "application/pdf", &priv.PublicKey)
	if out, err := engine.Unprotect(context.Background(), v1, claveCompat(t, priv)); err != nil || out.Document.Name != "antiguo.pdf" || len(out.Avisos) != 0 {
		t.Fatalf("un sobre v1 normal debe abrirse sin avisos: %+v, %v", out, err)
	}
	ambiguo := afpDeAtacante(t, 1, "factura.pdf", "text/html\nname=factura.pdf", &priv.PublicKey)
	if _, err := engine.Unprotect(context.Background(), ambiguo, claveCompat(t, priv)); err == nil || excepciones.FromText(err.Error()) == nil {
		t.Fatalf("la lectura ambigua debe requerir ajuste: %v", err)
	}
	excepciones.SetForTest(t, "afp_aad_ambigua", true)
	machinepolicy.UnsetForTest(t, "permitir_afp_aad_ambigua")
	out, err := engine.Unprotect(context.Background(), ambiguo, claveCompat(t, priv))
	if err != nil {
		t.Fatal(err)
	}
	if out.Document.Name != "factura.pdf" || out.Document.MIMEType != "application/octet-stream" {
		t.Fatalf("el nombre y el MIME deben salir saneados: %q %q", out.Document.Name, out.Document.MIMEType)
	}
	if len(out.Avisos) != 1 || out.Avisos[0] != excepciones.Warning("afp_aad_ambigua") || len(out.Detalles) != 1 || out.Detalles[0] != detalleAFPAADV1Ambigua {
		t.Fatalf("falta el aviso o el detalle: %v %v", out.Avisos, out.Detalles)
	}
	machinepolicy.SetForTest(t, "permitir_afp_aad_ambigua", false)
	if _, err := engine.Unprotect(context.Background(), ambiguo, claveCompat(t, priv)); err == nil || excepciones.FromText(err.Error()) == nil {
		t.Fatalf("la directiva debe prevalecer: %v", err)
	}
	machinepolicy.ErrorForTest(t, "permitir_afp_aad_ambigua", errors.New("fixture ilegible"))
	if _, err := engine.Unprotect(context.Background(), ambiguo, claveCompat(t, priv)); err == nil || excepciones.FromText(err.Error()) == nil {
		t.Fatalf("la directiva ilegible debe bloquear: %v", err)
	}
	v9 := afpDeAtacante(t, 9, "futuro.pdf", "application/pdf", &priv.PublicKey)
	if _, err := engine.Unprotect(context.Background(), v9, claveCompat(t, priv)); err == nil {
		t.Fatal("se aceptó un sobre de versión desconocida")
	}
}
