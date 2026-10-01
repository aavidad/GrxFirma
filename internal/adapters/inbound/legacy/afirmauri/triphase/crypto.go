// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package triphase implementa el protocolo trifásico de AutoFirma:
// Retrieve → Descifrado de sesión → PreSign → Firma local → PostSign → Upload.
package triphase

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des" // #nosec G502 -- DES is required for V1.9 payloads and is gated by explicit operator opt-in.
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"grxfirma/internal/adapters/inbound/legacy/legacycrypto"
)

const envHabilitarLegacyDES = legacycrypto.EnvEnableLegacyDES

// errLegacyDESDeshabilitado explica al operador como reactivar el fallback si
// su portal todavia lo necesita, en vez de dejarle un fallo opaco.
var errLegacyDESDeshabilitado = legacycrypto.ErrDESDisabled

// DecodeRetrievePayloadCompat aplica el mismo postprocesado tolerante que V1
// usa tras RetrieveService: intenta descifrado de sesión y una segunda pasada
// de base64 si el contenido recuperado aún viene encapsulado.
func DecodeRetrievePayloadCompat(datos []byte, claveRaw string, endpoints ...string) []byte {
	plano, _ := DecodeRetrievePayload(datos, claveRaw, endpoints...)
	return plano
}

// DecodeRetrievePayload es DecodeRetrievePayloadCompat, pero informa cuando
// los datos venían cifrados con el formato DES de V1.9 y la política lo
// bloquea. Sin ese error el flujo fallaba después con un mensaje que culpaba
// al portal de una decisión local.
func DecodeRetrievePayload(datos []byte, claveRaw string, endpoints ...string) ([]byte, error) {
	trimmed := bytes.TrimSpace(datos)
	if len(trimmed) == 0 {
		return datos, nil
	}

	var errDescifrado error
	candidatos := [][]byte{append([]byte(nil), trimmed...)}
	if claveRaw != "" && requiereDescifradoSesion(trimmed) {
		plano, err := descifrarAES128CBC(trimmed, claveRaw, endpoints...)
		if err == nil {
			plano = bytes.TrimSpace(plano)
			if len(plano) > 0 {
				candidatos = append(candidatos, plano)
			}
		} else if errors.Is(err, errLegacyDESDeshabilitado) {
			errDescifrado = err
		}
	}

	for _, candidato := range candidatos {
		if payloadParecePlano(candidato) {
			return candidato, nil
		}
		if decoded, ok := decodeBase64CompatPayload(candidato); ok {
			return decoded, nil
		}
	}

	return append([]byte(nil), trimmed...), errDescifrado
}

// descifrarAES128CBC descifra datos de sesión. Intenta primero el formato moderno
// AES-128-CBC y, si no encaja, cae al formato legacy compatible con V1:
// PADDING.BASE64URL + DES/ECB/NoPadding.
func descifrarAES128CBC(datos []byte, claveRaw string, endpoints ...string) ([]byte, error) {
	if len(datos) == 0 {
		return nil, errors.New("los datos a descifrar no pueden estar vacios")
	}
	if plano, err := descifrarAES128CBCInterno(datos, claveRaw); err == nil {
		return plano, nil
	}
	if err := legacycrypto.RequireDESEnIntercambio(endpoints...); err != nil {
		return nil, errLegacyDESDeshabilitado
	}
	return descifrarLegacyDES(datos, claveRaw, endpoints...)
}

// cifrarAES128CBC cifra datos de sesión. Usa AES-128-CBC cuando la clave resuelve
// 16 bytes y, en caso contrario, usa el formato legacy compatible con V1.
func cifrarAES128CBC(datos []byte, claveRaw string, endpoints ...string) ([]byte, error) {
	if len(datos) == 0 {
		return nil, errors.New("los datos a cifrar no pueden estar vacios")
	}
	if cifrado, err := cifrarAES128CBCInterno(datos, claveRaw); err == nil {
		return cifrado, nil
	}
	if err := legacycrypto.RequireDESEnIntercambio(endpoints...); err != nil {
		return nil, errLegacyDESDeshabilitado
	}
	return cifrarLegacyDES(datos, claveRaw, endpoints...)
}

func descifrarAES128CBCInterno(datos []byte, claveRaw string) ([]byte, error) {
	clave, err := decodificarClaveAES128(claveRaw)
	if err != nil {
		return nil, err
	}
	if len(datos) < aes.BlockSize {
		return nil, fmt.Errorf("los datos cifrados son demasiado cortos: %d bytes (minimo %d)", len(datos), aes.BlockSize)
	}
	if len(datos)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("la longitud de los datos cifrados (%d) no es multiplo del bloque AES (%d)", len(datos), aes.BlockSize)
	}

	iv := datos[:aes.BlockSize]
	ciphertext := datos[aes.BlockSize:]

	block, err := aes.NewCipher(clave)
	if err != nil {
		return nil, fmt.Errorf("error creando cifrador AES: %w", err)
	}

	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext)
}

func cifrarAES128CBCInterno(datos []byte, claveRaw string) ([]byte, error) {
	clave, err := decodificarClaveAES128(claveRaw)
	if err != nil {
		return nil, err
	}

	padded := pkcs7Pad(datos, aes.BlockSize)
	block, err := aes.NewCipher(clave)
	if err != nil {
		return nil, fmt.Errorf("error creando cifrador AES: %w", err)
	}

	iv, err := generarIVAleatorio()
	if err != nil {
		return nil, fmt.Errorf("error generando IV aleatorio: %w", err)
	}
	out := make([]byte, aes.BlockSize+len(padded))
	copy(out[:aes.BlockSize], iv[:])
	cipher.NewCBCEncrypter(block, iv[:]).CryptBlocks(out[aes.BlockSize:], padded)
	return out, nil
}

func generarIVAleatorio() ([aes.BlockSize]byte, error) {
	var iv [aes.BlockSize]byte
	if _, err := rand.Read(iv[:]); err != nil {
		return [aes.BlockSize]byte{}, err
	}
	return iv, nil
}

func decodificarClaveAES128(raw string) ([]byte, error) {
	candidata := strings.TrimSpace(raw)
	if candidata == "" {
		return nil, errors.New("la clave AES-128 no puede estar vacia")
	}

	if clave, err := hex.DecodeString(candidata); err == nil {
		if len(clave) == 16 {
			return clave, nil
		}
		return nil, fmt.Errorf("la clave AES-128 debe ser de 16 bytes (32 chars hex), se obtuvieron %d bytes", len(clave))
	}

	if clave, err := base64.StdEncoding.DecodeString(candidata); err == nil && len(clave) == 16 {
		return clave, nil
	}
	if clave, err := base64.RawStdEncoding.DecodeString(candidata); err == nil && len(clave) == 16 {
		return clave, nil
	}
	if clave, err := base64.URLEncoding.DecodeString(candidata); err == nil && len(clave) == 16 {
		return clave, nil
	}
	if clave, err := base64.RawURLEncoding.DecodeString(candidata); err == nil && len(clave) == 16 {
		return clave, nil
	}

	if len([]byte(candidata)) == 16 {
		return []byte(candidata), nil
	}

	return nil, fmt.Errorf("clave hex invalida o no compatible con AES-128: %q", candidata)
}

func descifrarLegacyDES(datos []byte, claveRaw string, endpoints ...string) ([]byte, error) {
	if err := legacycrypto.RequireDESEnIntercambio(endpoints...); err != nil {
		return nil, err
	}
	texto := strings.TrimSpace(string(datos))
	if texto == "" {
		return nil, errors.New("datos legacy vacios")
	}

	padding := 0
	payload := texto
	if dot := strings.IndexByte(texto, '.'); dot >= 0 {
		valorPadding, err := strconv.Atoi(texto[:dot])
		if err != nil {
			return nil, fmt.Errorf("padding legacy invalido: %w", err)
		}
		padding = valorPadding
		payload = texto[dot+1:]
	}

	payload = strings.ReplaceAll(payload, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")
	cifrado, err := decodificarPayloadLegacy(payload)
	if err != nil {
		return nil, fmt.Errorf("payload legacy base64 invalido: %w", err)
	}

	clave := normalizarClaveDES(claveRaw)
	block, err := des.NewCipher(clave) // #nosec G405 -- DES is required only for V1.9 interoperability and is gated by explicit operator opt-in above.
	if err != nil {
		return nil, fmt.Errorf("error creando cifrador DES legacy: %w", err)
	}
	if len(cifrado)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("ciphertext legacy no es multiplo de bloque DES")
	}

	plano := make([]byte, len(cifrado))
	for i := 0; i < len(cifrado); i += block.BlockSize() {
		block.Decrypt(plano[i:i+block.BlockSize()], cifrado[i:i+block.BlockSize()])
	}
	if padding > 0 && padding < len(plano) {
		plano = plano[:len(plano)-padding]
	}
	return plano, nil
}

func cifrarLegacyDES(datos []byte, claveRaw string, endpoints ...string) ([]byte, error) {
	if err := legacycrypto.RequireDESEnIntercambio(endpoints...); err != nil {
		return nil, err
	}
	clave := normalizarClaveDES(claveRaw)
	block, err := des.NewCipher(clave) // #nosec G405 -- DES is required only for V1.9 interoperability and is gated by explicit operator opt-in above.
	if err != nil {
		return nil, fmt.Errorf("error creando cifrador DES legacy: %w", err)
	}

	padding := (block.BlockSize() - len(datos)%block.BlockSize()) % block.BlockSize()
	padded := make([]byte, len(datos)+padding)
	copy(padded, datos)

	cifrado := make([]byte, len(padded))
	for i := 0; i < len(padded); i += block.BlockSize() {
		block.Encrypt(cifrado[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
	}

	payload := base64.StdEncoding.EncodeToString(cifrado)
	payload = strings.ReplaceAll(payload, "+", "-")
	payload = strings.ReplaceAll(payload, "/", "_")
	return []byte(fmt.Sprintf("%d.%s", padding, payload)), nil
}

func normalizarClaveDES(raw string) []byte {
	clave := []byte(strings.TrimSpace(raw))
	if len(clave) < 8 {
		padded := make([]byte, 8)
		copy(padded, clave)
		return padded
	}
	if len(clave) > 8 {
		return append([]byte(nil), clave[:8]...)
	}
	return append([]byte(nil), clave...)
}

func decodificarPayloadLegacy(payload string) ([]byte, error) {
	if cifrado, err := base64.StdEncoding.DecodeString(payload); err == nil {
		return cifrado, nil
	}
	if cifrado, err := base64.RawStdEncoding.DecodeString(payload); err == nil {
		return cifrado, nil
	}
	if cifrado, err := base64.URLEncoding.DecodeString(payload); err == nil {
		return cifrado, nil
	}
	if cifrado, err := base64.RawURLEncoding.DecodeString(payload); err == nil {
		return cifrado, nil
	}
	return nil, fmt.Errorf("ningun decoder base64 compatible acepto el payload")
}

func payloadParecePlano(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "{") ||
		strings.HasPrefix(trimmed, "[") ||
		strings.HasPrefix(trimmed, "<") ||
		strings.HasPrefix(trimmed, "%PDF")
}

func decodeBase64CompatPayload(data []byte) ([]byte, bool) {
	inputs := []string{strings.TrimSpace(string(data))}
	if dot := strings.IndexByte(inputs[0], '.'); dot >= 0 && dot+1 < len(inputs[0]) {
		inputs = append(inputs, strings.TrimSpace(inputs[0][dot+1:]))
	}

	decoders := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}

	for _, input := range inputs {
		if input == "" {
			continue
		}
		for _, enc := range decoders {
			decoded, err := enc.DecodeString(input)
			if err != nil {
				continue
			}
			decoded = bytes.TrimSpace(decoded)
			if payloadParecePlano(decoded) {
				return decoded, true
			}
		}
	}
	return nil, false
}

// pkcs7Pad añade padding PKCS#7 para que la longitud sea múltiplo de blockSize.
func pkcs7Pad(data []byte, blockSize int) []byte {
	if blockSize <= 0 || blockSize > 255 {
		panic("pkcs7: invalid block size")
	}
	padding := blockSize - (len(data) % blockSize)
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	padByte, err := pkcs7PaddingByte(padding)
	if err != nil {
		panic(err)
	}
	for i := len(data); i < len(padded); i++ {
		padded[i] = padByte
	}
	return padded
}

func pkcs7PaddingByte(padding int) (byte, error) {
	if padding < 1 || padding > math.MaxUint8 {
		return 0, fmt.Errorf("pkcs7: invalid padding length: %d", padding)
	}
	return byte(padding), nil
}

// pkcs7Unpad elimina el padding PKCS#7 de los datos.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("datos vacios, no se puede eliminar padding")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > aes.BlockSize {
		return nil, fmt.Errorf("valor de padding PKCS7 invalido: %d", padLen)
	}
	if padLen > len(data) {
		return nil, fmt.Errorf("padding (%d) mayor que los datos (%d)", padLen, len(data))
	}
	expected := bytes.Repeat([]byte{byte(padLen)}, padLen)
	if !bytes.Equal(data[len(data)-padLen:], expected) {
		return nil, errors.New("padding PKCS7 invalido: bytes de padding no coinciden")
	}
	return data[:len(data)-padLen], nil
}
