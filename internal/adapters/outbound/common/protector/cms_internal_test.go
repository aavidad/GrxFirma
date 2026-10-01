// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"encoding/asn1"
	"encoding/base64"
	"testing"
)

func TestRequestedCMSContentType_AuthEnvelopedAliases(t *testing.T) {
	t.Parallel()
	cases := []string{
		"cms-authenveloped",
		"cms-auth-enveloped",
		"authenveloped",
		"auth-enveloped",
		"authenvelopeddata",
		"auth-enveloped-data",
		"authenticatedenvelopeddata",
		"authenticated-enveloped-data",
		"CMS_AUTH_ENVELOPED",
	}
	for _, raw := range cases {
		got, err := requestedCMSContentType(map[string]string{protectionContainerOptionKey: raw})
		if err != nil {
			t.Fatalf("requestedCMSContentType(%q) error = %v", raw, err)
		}
		if got != cmsContentTypeAuthEnvelopedData {
			t.Fatalf("requestedCMSContentType(%q) = %q, want %q", raw, got, cmsContentTypeAuthEnvelopedData)
		}
	}
}

func TestRequestedCMSContentType_AuthenticatedAliases(t *testing.T) {
	t.Parallel()
	cases := []string{
		"cms-authenticated",
		"cms-authenticated-data",
		"authenticated",
		"authenticateddata",
		"authenticated-data",
		"AUTHENTICATED_DATA",
	}
	for _, raw := range cases {
		got, err := requestedCMSContentType(map[string]string{protectionContainerOptionKey: raw})
		if err != nil {
			t.Fatalf("requestedCMSContentType(%q) error = %v", raw, err)
		}
		if got != cmsContentTypeAuthenticatedData {
			t.Fatalf("requestedCMSContentType(%q) = %q, want %q", raw, got, cmsContentTypeAuthenticatedData)
		}
	}
}

func TestRequestedCMSContentType_CompressedAliases(t *testing.T) {
	t.Parallel()
	cases := []string{
		"cms-compressed",
		"cms-compressed-data",
		"compressed",
		"compresseddata",
		"compressed-data",
		"COMPRESSED_DATA",
	}
	for _, raw := range cases {
		got, err := requestedCMSContentType(map[string]string{protectionContainerOptionKey: raw})
		if err != nil {
			t.Fatalf("requestedCMSContentType(%q) error = %v", raw, err)
		}
		if got != cmsContentTypeCompressedData {
			t.Fatalf("requestedCMSContentType(%q) = %q, want %q", raw, got, cmsContentTypeCompressedData)
		}
	}
}

func TestRequestedCMSContentType_SignedAndEnvelopedAliases(t *testing.T) {
	t.Parallel()
	cases := []string{
		"cms-signedandenveloped",
		"cms-signed-and-enveloped",
		"signedandenveloped",
		"signed-and-enveloped",
		"signedandenvelopeddata",
		"signed-and-enveloped-data",
		"SIGNED_AND_ENVELOPED_DATA",
	}
	for _, raw := range cases {
		got, err := requestedCMSContentType(map[string]string{protectionContainerOptionKey: raw})
		if err != nil {
			t.Fatalf("requestedCMSContentType(%q) error = %v", raw, err)
		}
		if got != cmsContentTypeSignedEnvelopedData {
			t.Fatalf("requestedCMSContentType(%q) = %q, want %q", raw, got, cmsContentTypeSignedEnvelopedData)
		}
	}
}

func TestRequestedCMSContentType_EnvelopedAndEncryptedAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want cmsContentType
	}{
		{raw: "cms-enveloped-data", want: cmsContentTypeEnvelopedData},
		{raw: "ENVELOPED_DATA", want: cmsContentTypeEnvelopedData},
		{raw: "cms-encrypted-data", want: cmsContentTypeEncryptedData},
		{raw: "ENCRYPTED_DATA", want: cmsContentTypeEncryptedData},
	}
	for _, tc := range cases {
		got, err := requestedCMSContentType(map[string]string{protectionContainerOptionKey: tc.raw})
		if err != nil {
			t.Fatalf("requestedCMSContentType(%q) error = %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("requestedCMSContentType(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestDetectCMSContentType_AuthenticatedModesAndCompressed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		oid  asn1.ObjectIdentifier
		want cmsContentType
	}{
		{name: "authEnveloped", oid: oidCMSAuthEnvelopedData, want: cmsContentTypeAuthEnvelopedData},
		{name: "authenticated", oid: oidCMSAuthenticatedData, want: cmsContentTypeAuthenticatedData},
		{name: "compressed", oid: oidCMSCompressedData, want: cmsContentTypeCompressedData},
	}
	for _, tc := range cases {
		raw, err := asn1.Marshal(cmsContentInfo{ContentType: tc.oid})
		if err != nil {
			t.Fatalf("%s: asn1.Marshal() error = %v", tc.name, err)
		}
		got, err := detectCMSContentType(raw)
		if err != nil {
			t.Fatalf("%s: detectCMSContentType() error = %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: detectCMSContentType() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDecodeProtectionSecret_RequiresStrictCanonicalBase64(t *testing.T) {
	raw := bytes.Repeat([]byte{0x5a}, protectionAES256KeyBytes)
	canonical := base64.StdEncoding.EncodeToString(raw)

	decoded, err := decodeProtectionSecret(canonical)
	if err != nil {
		t.Fatalf("decodeProtectionSecret(canonical) error = %v", err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatal("decodeProtectionSecret(canonical) no conserva la clave")
	}
	zeroBytes(decoded)
	if !bytes.Equal(decoded, make([]byte, protectionAES256KeyBytes)) {
		t.Fatal("la copia controlada devuelta no se pudo zeroizar")
	}

	nonCanonical := []byte(canonical)
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	index := bytes.IndexByte([]byte(alphabet), nonCanonical[len(nonCanonical)-2])
	if index < 0 || index%4 != 0 {
		t.Fatalf("caracter Base64 final inesperado: %q", nonCanonical[len(nonCanonical)-2])
	}
	nonCanonical[len(nonCanonical)-2] = alphabet[index+1]
	legacyDecoded, legacyErr := base64.StdEncoding.DecodeString(string(nonCanonical))
	if legacyErr != nil || !bytes.Equal(legacyDecoded, raw) {
		t.Fatal("el caso de prueba debe ser aceptado por el decoder Base64 no estricto")
	}
	clear(legacyDecoded)
	if _, err := decodeProtectionSecret(string(nonCanonical)); err == nil {
		t.Fatal("se esperaba rechazo de padding bits Base64 no canónicos")
	}
}

func TestDecodeProtectionSecret_RejectsWrongLength(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x5a}, protectionAES256KeyBytes-1))
	if _, err := decodeProtectionSecret(encoded); err == nil {
		t.Fatal("se esperaba rechazo de una clave que no tiene 32 bytes")
	}
}
