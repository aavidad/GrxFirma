// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"bytes"
	"encoding/base64"
	"testing"

	"grxfirma/internal/domain"
)

func TestMergeTransientProtectionKeys_RejectsNonCanonicalPaddingBits(t *testing.T) {
	raw := bytes.Repeat([]byte{0x5a}, 32)
	encoded := []byte(base64.StdEncoding.EncodeToString(raw))
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	index := bytes.IndexByte([]byte(alphabet), encoded[len(encoded)-2])
	if index < 0 || index%4 != 0 {
		t.Fatalf("caracter Base64 final inesperado: %q", encoded[len(encoded)-2])
	}
	encoded[len(encoded)-2] = alphabet[index+1]
	legacyDecoded, legacyErr := base64.StdEncoding.DecodeString(string(encoded))
	if legacyErr != nil || !bytes.Equal(legacyDecoded, raw) {
		t.Fatal("el caso de prueba debe ser aceptado por el decoder Base64 no estricto")
	}
	clear(legacyDecoded)

	if _, err := mergeTransientProtectionKeys(
		nil, map[string]string{"secret_b64": string(encoded)},
		nil,
	); err == nil {
		t.Fatal("se esperaba rechazo de padding bits Base64 no canónicos")
	}
}

func TestZeroTransientProtectionKeys_OnlyClearsOwnedTransientCopy(t *testing.T) {
	providerKey := bytes.Repeat([]byte{0x11}, 32)
	raw := bytes.Repeat([]byte{0x5a}, 32)
	keys, err := mergeTransientProtectionKeys(
		[]domain.ProtectionKeyMaterial{{
			RecipientID:  "provider-key",
			SymmetricKey: providerKey,
		}},
		map[string]string{
			"secret_b64": base64.StdEncoding.EncodeToString(raw),
		},
		nil,
	)
	if err != nil {
		t.Fatalf("mergeTransientProtectionKeys() error = %v", err)
	}
	if len(keys) != 2 || !bytes.Equal(keys[1].SymmetricKey, raw) {
		t.Fatal("no se añadió la clave transitoria esperada")
	}

	ownedTransientBacking := keys[1].SymmetricKey
	zeroTransientProtectionKeys(keys)
	if keys[1].SymmetricKey != nil {
		t.Fatal("la referencia transitoria debe eliminarse tras el uso")
	}
	if !bytes.Equal(ownedTransientBacking, make([]byte, 32)) {
		t.Fatal("el backing array transitorio no quedó zeroizado")
	}
	if !bytes.Equal(providerKey, bytes.Repeat([]byte{0x11}, 32)) {
		t.Fatal("no se debe modificar material propiedad del proveedor")
	}
}

func TestZeroTransientProtectionKeys_ClearsPrivateDecryptionMaterial(t *testing.T) {
	rsaDER := bytes.Repeat([]byte{0x21}, 48)
	mlkemSeed := bytes.Repeat([]byte{0x32}, 64)
	x25519Private := bytes.Repeat([]byte{0x43}, 32)
	keys := []domain.ProtectionKeyMaterial{{
		RecipientID:               "provider-key",
		RSAOAEP256PrivateKeyPKCS8: rsaDER,
		MLKEM768Seed:              mlkemSeed,
		X25519PrivateKey:          x25519Private,
	}}

	zeroTransientProtectionKeys(keys)

	if keys[0].RSAOAEP256PrivateKeyPKCS8 != nil ||
		keys[0].MLKEM768Seed != nil ||
		keys[0].X25519PrivateKey != nil {
		t.Fatal("las referencias privadas deben retirarse tras desproteger")
	}
	for name, backing := range map[string][]byte{
		"RSA PKCS#8": rsaDER,
		"ML-KEM":     mlkemSeed,
		"X25519":     x25519Private,
	} {
		if !bytes.Equal(backing, make([]byte, len(backing))) {
			t.Fatalf("el backing array %s no quedó zeroizado", name)
		}
	}
}

func TestMergeTransientProtectionKeys_CopiesRawSymmetricKey(t *testing.T) {
	raw := bytes.Repeat([]byte{0x37}, 32)
	keys, err := mergeTransientProtectionKeys(nil, nil, raw)
	if err != nil {
		t.Fatalf("mergeTransientProtectionKeys() error = %v", err)
	}
	if len(keys) != 1 || !bytes.Equal(keys[0].SymmetricKey, raw) {
		t.Fatal("no se añadió la clave simétrica binaria")
	}
	if len(keys[0].SymmetricKey) > 0 &&
		&keys[0].SymmetricKey[0] == &raw[0] {
		t.Fatal("la aplicación no debe conservar el buffer del adaptador")
	}
	zeroTransientProtectionKeys(keys)
	if !bytes.Equal(raw, bytes.Repeat([]byte{0x37}, 32)) {
		t.Fatal("la aplicación no debe borrar el buffer propiedad del adaptador")
	}
}
