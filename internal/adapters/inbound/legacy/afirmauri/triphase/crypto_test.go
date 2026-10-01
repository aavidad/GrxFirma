// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/security/avisos"
	"grxfirma/internal/security/machinepolicy"
)

const claveTestHex = "0102030405060708090a0b0c0d0e0f10"

func TestCifrarDescifrarRoundtrip(t *testing.T) {
	original := []byte("datos de prueba para cifrado AES-128-CBC")
	cifrado, err := cifrarAES128CBC(original, claveTestHex)
	if err != nil {
		t.Fatalf("cifrarAES128CBC: error inesperado: %v", err)
	}
	descifrado, err := descifrarAES128CBC(cifrado, claveTestHex)
	if err != nil {
		t.Fatalf("descifrarAES128CBC: error inesperado: %v", err)
	}
	if !bytes.Equal(original, descifrado) {
		t.Errorf("roundtrip: esperado %q, obtenido %q", original, descifrado)
	}
}

func TestCifrarAES128CBC_UsaIVAleatorioAntepuesto(t *testing.T) {
	original := []byte("mismo contenido confidencial")
	primero, err := cifrarAES128CBCInterno(original, claveTestHex)
	if err != nil {
		t.Fatalf("primer cifrado: %v", err)
	}
	segundo, err := cifrarAES128CBCInterno(original, claveTestHex)
	if err != nil {
		t.Fatalf("segundo cifrado: %v", err)
	}
	if len(primero) < aes.BlockSize || len(segundo) < aes.BlockSize {
		t.Fatalf("los cifrados no contienen el IV antepuesto: %d y %d bytes", len(primero), len(segundo))
	}
	if bytes.Equal(primero[:aes.BlockSize], segundo[:aes.BlockSize]) {
		t.Fatal("dos cifrados independientes reutilizaron el mismo IV")
	}
	if bytes.Equal(primero, segundo) {
		t.Fatal("AES-CBC produjo el mismo resultado para dos IV independientes")
	}
	for i, cifrado := range [][]byte{primero, segundo} {
		plano, err := descifrarAES128CBCInterno(cifrado, claveTestHex)
		if err != nil {
			t.Fatalf("descifrado %d: %v", i, err)
		}
		if !bytes.Equal(plano, original) {
			t.Fatalf("descifrado %d=%q, want %q", i, plano, original)
		}
	}
}

func TestCifrarDescifrarDatosCortos(t *testing.T) {
	original := []byte("X")
	cifrado, err := cifrarAES128CBC(original, claveTestHex)
	if err != nil {
		t.Fatalf("cifrarAES128CBC datos cortos: %v", err)
	}
	descifrado, err := descifrarAES128CBC(cifrado, claveTestHex)
	if err != nil {
		t.Fatalf("descifrarAES128CBC datos cortos: %v", err)
	}
	if !bytes.Equal(original, descifrado) {
		t.Errorf("roundtrip datos cortos: esperado %q, obtenido %q", original, descifrado)
	}
}

func TestCifrarDescifrarDatosMultipleDeBloque(t *testing.T) {
	original := bytes.Repeat([]byte("A"), 32)
	cifrado, err := cifrarAES128CBC(original, claveTestHex)
	if err != nil {
		t.Fatalf("cifrarAES128CBC multiplo bloque: %v", err)
	}
	descifrado, err := descifrarAES128CBC(cifrado, claveTestHex)
	if err != nil {
		t.Fatalf("descifrarAES128CBC multiplo bloque: %v", err)
	}
	if !bytes.Equal(original, descifrado) {
		t.Errorf("roundtrip multiplo bloque: esperado %v, obtenido %v", original, descifrado)
	}
}

func TestCifrarClaveHexInvalidaActivaFallbackLegacy(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	cifrado, err := cifrarAES128CBC([]byte("datos"), "xyz_no_es_hex")
	if err != nil {
		t.Fatalf("cifrarAES128CBC con fallback legacy: %v", err)
	}
	if !strings.Contains(string(cifrado), ".") {
		t.Fatalf("se esperaba formato legacy padding.base64, obtenido %q", string(cifrado))
	}
}

func TestDescifrarClaveHexInvalida(t *testing.T) {
	_, err := descifrarAES128CBC(bytes.Repeat([]byte{0x01}, 32), "xyz_no_es_hex")
	if err == nil {
		t.Fatal("descifrarAES128CBC: deberia fallar con clave invalida")
	}
}

func TestCifrarDatosVacios(t *testing.T) {
	_, err := cifrarAES128CBC([]byte{}, claveTestHex)
	if err == nil {
		t.Fatal("cifrarAES128CBC: deberia fallar con datos vacios")
	}
}

func TestDescifrarDatosVacios(t *testing.T) {
	_, err := descifrarAES128CBC([]byte{}, claveTestHex)
	if err == nil {
		t.Fatal("descifrarAES128CBC: deberia fallar con datos vacios")
	}
}

func TestDescifrarClaveLongitudIncorrecta(t *testing.T) {
	claveCorta := hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	_, err := descifrarAES128CBC(bytes.Repeat([]byte{0x01}, 32), claveCorta)
	if err == nil {
		t.Fatal("descifrarAES128CBC: deberia fallar con clave de longitud incorrecta")
	}
}

func TestCifrarClaveLongitudIncorrectaActivaFallbackLegacy(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	claveCorta := hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	cifrado, err := cifrarAES128CBC([]byte("datos"), claveCorta)
	if err != nil {
		t.Fatalf("cifrarAES128CBC con fallback legacy: %v", err)
	}
	if !strings.Contains(string(cifrado), ".") {
		t.Fatalf("se esperaba formato legacy padding.base64, obtenido %q", string(cifrado))
	}
}

func TestDescifrarDatosNoMultiploDeBloque(t *testing.T) {
	_, err := descifrarAES128CBC(bytes.Repeat([]byte{0x01}, 17), claveTestHex)
	if err == nil {
		t.Fatal("descifrarAES128CBC: deberia fallar con datos que no son multiplo de bloque")
	}
}

func TestDescifrarDatosDemasiadorCortos(t *testing.T) {
	_, err := descifrarAES128CBC([]byte{0x01, 0x02}, claveTestHex)
	if err == nil {
		t.Fatal("descifrarAES128CBC: deberia fallar con datos mas cortos que el IV")
	}
}

func TestDescifrarClaveBase64Valida(t *testing.T) {
	original := []byte("datos base64")
	cifrado, err := cifrarAES128CBC(original, claveTestHex)
	if err != nil {
		t.Fatalf("cifrarAES128CBC: %v", err)
	}
	clave, _ := hex.DecodeString(claveTestHex)
	claveB64 := base64.StdEncoding.EncodeToString(clave)
	descifrado, err := descifrarAES128CBC(cifrado, claveB64)
	if err != nil {
		t.Fatalf("descifrarAES128CBC base64: %v", err)
	}
	if !bytes.Equal(original, descifrado) {
		t.Fatalf("descifrado inesperado: %q", descifrado)
	}
}

func TestPKCS7PaddingByte_ValidaLimitesDeOcteto(t *testing.T) {
	for _, tt := range []struct {
		padding int
		want    byte
	}{
		{padding: 1, want: 1},
		{padding: 255, want: 255},
	} {
		got, err := pkcs7PaddingByte(tt.padding)
		if err != nil {
			t.Fatalf("pkcs7PaddingByte(%d): %v", tt.padding, err)
		}
		if got != tt.want {
			t.Fatalf("pkcs7PaddingByte(%d)=%d, want %d", tt.padding, got, tt.want)
		}
	}
	for _, padding := range []int{-1, 0, 256} {
		if _, err := pkcs7PaddingByte(padding); err == nil {
			t.Errorf("pkcs7PaddingByte(%d) debería fallar", padding)
		}
	}
}

func TestPKCS7Pad_SoportaTamanoMaximoRepresentable(t *testing.T) {
	padded := pkcs7Pad(nil, 255)
	if len(padded) != 255 {
		t.Fatalf("len(pkcs7Pad)=%d, want 255", len(padded))
	}
	if !bytes.Equal(padded, bytes.Repeat([]byte{255}, 255)) {
		t.Fatal("el padding máximo no está representado por 255 octetos 0xff")
	}
}

func TestLegacyDESConClaveCorta(t *testing.T) {
	// El fallback DES ya no esta activo por defecto: hay que pedirlo.
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	original := []byte("<xml>dato legacy</xml>")
	claveLegacy := "abc"
	cifrado, err := cifrarAES128CBC(original, claveLegacy)
	if err != nil {
		t.Fatalf("cifrar legacy: %v", err)
	}
	descifrado, err := descifrarAES128CBC(cifrado, claveLegacy)
	if err != nil {
		t.Fatalf("descifrar legacy: %v", err)
	}
	if !bytes.Equal(original, descifrado) {
		t.Fatalf("legacy roundtrip inesperado: %q", descifrado)
	}
}

// El fallback DES/ECB heredado esta desactivado salvo peticion expresa. Es una
// postura de seguridad, no una preferencia: DES esta roto y este cliente lo
// usan administraciones publicas.
func TestLegacyDESDesactivadoPorDefecto(t *testing.T) {
	if _, err := cifrarAES128CBC([]byte("datos"), "abc"); !errors.Is(err, errLegacyDESDeshabilitado) {
		t.Fatalf("cifrar debio rechazarse por DES desactivado, y fue: %v", err)
	}
	if _, err := descifrarAES128CBC([]byte("1.YWJjZA"), "abc"); !errors.Is(err, errLegacyDESDeshabilitado) {
		t.Fatalf("descifrar debio rechazarse por DES desactivado, y fue: %v", err)
	}
}

// El mensaje debe decirle al operador como reactivarlo, no fallar en opaco.
func TestLegacyDESDesactivadoExplicaComoReactivarlo(t *testing.T) {
	_, err := cifrarAES128CBC([]byte("datos"), "abc")
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if !strings.Contains(err.Error(), machinepolicy.PermitirDESLegacy) {
		t.Fatalf("el error debe nombrar la política que lo reactiva, y fue: %v", err)
	}
}

func TestLegacyDESSeReactivaPorConfiguracion(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	original := []byte("datos heredados")
	cifrado, err := cifrarAES128CBC(original, "abc")
	if err != nil {
		t.Fatalf("con %s=1 debe funcionar: %v", envHabilitarLegacyDES, err)
	}
	descifrado, err := descifrarAES128CBC(cifrado, "abc")
	if err != nil {
		t.Fatalf("descifrar con DES reactivado: %v", err)
	}
	if !bytes.Equal(original, descifrado) {
		t.Fatalf("ida y vuelta inesperada: %q", descifrado)
	}
}

func TestDescifrarLegacyDESAceptaBase64SinPadding(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	original := []byte("<xml>dato legacy sin padding</xml>")
	claveLegacy := "abcd"
	cifrado, err := cifrarLegacyDES(original, claveLegacy)
	if err != nil {
		t.Fatalf("cifrar legacy: %v", err)
	}
	partes := strings.SplitN(string(cifrado), ".", 2)
	if len(partes) != 2 {
		t.Fatalf("formato legacy inesperado: %q", cifrado)
	}
	sinPadding := strings.TrimRight(partes[1], "=")
	descifrado, err := descifrarLegacyDES([]byte(partes[0]+"."+sinPadding), claveLegacy)
	if err != nil {
		t.Fatalf("descifrar legacy sin padding: %v", err)
	}
	if !bytes.Equal(original, descifrado) {
		t.Fatalf("legacy sin padding inesperado: %q", descifrado)
	}
}

func TestDecodeRetrievePayloadCompat_DescifraLegacyYDecodificaBase64(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	original := []byte(`{"signbatch":"ok"}`)
	envuelto := base64.StdEncoding.EncodeToString(original)
	cifrado, err := cifrarLegacyDES([]byte(envuelto), "abcd")
	if err != nil {
		t.Fatalf("cifrar legacy: %v", err)
	}

	decodificado := DecodeRetrievePayloadCompat(cifrado, "abcd")
	if !bytes.Equal(original, decodificado) {
		t.Fatalf("payload inesperado: %q", decodificado)
	}
}

func TestDecodeRetrievePayload_InformaDeDESBloqueado(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)
	// Formato de sesión V1.9: relleno "." y Base64 URL-safe de un bloque DES.
	cifrado := []byte("0.QUJDREVGR0hBQkNERUZHSA")
	_, err := DecodeRetrievePayload(cifrado, "12345678")
	if !errors.Is(err, errLegacyDESDeshabilitado) {
		t.Fatalf("err = %v, want DES deshabilitado", err)
	}
	if got := DecodeRetrievePayloadCompat(cifrado, "12345678"); len(got) == 0 {
		t.Fatal("la variante compatible debe seguir devolviendo los datos")
	}
}

// Sin política de máquina, DES solo se admite para el paquete del servidor
// intermedio cuando todos sus servidores son HTTPS, y se avisa al usuario.
func TestDESSoloEnIntercambioHTTPS(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)
	avisos.Reiniciar()
	original := []byte(`{"signbatch":"ok"}`)
	envuelto := base64.StdEncoding.EncodeToString(original)
	https := []string{"https://firesda.example.es/afirma/storage", "https://firesda.example.es/afirma/retrieve"}

	cifrado, err := cifrarLegacyDES([]byte(envuelto), "12345678", https...)
	if err != nil {
		t.Fatalf("DES con servidores HTTPS: %v", err)
	}
	plano, err := DecodeRetrievePayload(cifrado, "12345678", https...)
	if err != nil || !bytes.Equal(plano, original) {
		t.Fatalf("descifrado HTTPS = %q, %v", plano, err)
	}
	if !strings.Contains(avisos.Texto(), "Compatibilidad con AutoFirma 1.x") {
		t.Fatalf("el uso de DES debe explicarse al usuario: %q", avisos.Texto())
	}

	for _, endpoints := range [][]string{
		nil,
		{"http://firesda.example.es/afirma/storage"},
		{"https://firesda.example.es/ok", "http://127.0.0.1/x"},
		{"https://usuario@firesda.example.es/x"},
	} {
		if _, err := cifrarLegacyDES([]byte(envuelto), "12345678", endpoints...); !errors.Is(err, errLegacyDESDeshabilitado) {
			t.Fatalf("endpoints %v: err = %v, want DES bloqueado", endpoints, err)
		}
	}
}
