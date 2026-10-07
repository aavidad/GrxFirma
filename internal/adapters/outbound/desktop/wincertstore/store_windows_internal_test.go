// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package wincertstore

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// cryptKeyProvInfo refleja CRYPT_KEY_PROV_INFO de wincrypt.h.
type cryptKeyProvInfo struct {
	contenedor *uint16
	proveedor  *uint16
	tipo       uint32
	flags      uint32
	numParams  uint32
	params     uintptr
	keySpec    uint32
}

var procCertSetCertificateContextProperty = modCrypt32.NewProc("CertSetCertificateContextProperty")

// nuevoAlmacenMemoria crea un almacén volátil que solo existe en el proceso de
// la prueba: no toca el almacén MY del usuario.
func nuevoAlmacenMemoria(t *testing.T) windows.Handle {
	t.Helper()
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_MEMORY, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("CertOpenStore(memoria): %v", err)
	}
	t.Cleanup(func() { _ = windows.CertCloseStore(store, 0) })
	return store
}

func certificadoSintetico(t *testing.T, nombre string) []byte {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: nombre},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, clave.Public(), clave)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func anadirCertificado(t *testing.T, store windows.Handle, der []byte) *windows.CertContext {
	t.Helper()
	creado, err := windows.CertCreateCertificateContext(
		windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, &der[0], uint32(len(der)))
	if err != nil {
		t.Fatalf("CertCreateCertificateContext: %v", err)
	}
	defer windows.CertFreeCertificateContext(creado) //nolint:errcheck -- liberación en prueba
	var enAlmacen *windows.CertContext
	if err := windows.CertAddCertificateContextToStore(store, creado, windows.CERT_STORE_ADD_ALWAYS, &enAlmacen); err != nil {
		t.Fatalf("CertAddCertificateContextToStore: %v", err)
	}
	t.Cleanup(func() { _ = windows.CertFreeCertificateContext(enAlmacen) })
	return enAlmacen
}

// asociarClaveEnTarjeta declara que la clave está en el proveedor de tarjeta
// inteligente de Windows, con un contenedor que no existe. Abrirla exigiría la
// tarjeta; leer la propiedad no.
func asociarClaveEnTarjeta(t *testing.T, certCtx *windows.CertContext) {
	t.Helper()
	contenedor, _ := windows.UTF16PtrFromString("grxfirma-prueba-tarjeta-inexistente")
	proveedor, _ := windows.UTF16PtrFromString("Microsoft Smart Card Key Storage Provider")
	info := cryptKeyProvInfo{contenedor: contenedor, proveedor: proveedor, keySpec: windows.CERT_NCRYPT_KEY_SPEC}
	ret, _, err := procCertSetCertificateContextProperty.Call(
		uintptr(unsafe.Pointer(certCtx)), certKeyProvInfoPropID, 0, uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		t.Fatalf("CertSetCertificateContextProperty(KEY_PROV_INFO): %v", err)
	}
}

func huellaDER(der []byte) string {
	cert, _ := x509.ParseCertificate(der)
	return huellaCertificado(cert)
}

func TestListarAlmacen_ClaveEnTarjetaSeDetectaSinAbrirla(t *testing.T) {
	t.Parallel()
	store := nuevoAlmacenMemoria(t)

	derTarjeta := certificadoSintetico(t, "GrxFirma prueba clave en tarjeta")
	asociarClaveEnTarjeta(t, anadirCertificado(t, store, derTarjeta))
	derSinClave := certificadoSintetico(t, "GrxFirma prueba sin clave")
	anadirCertificado(t, store, derSinClave)

	inicio := time.Now()
	refs, err := listarAlmacen(context.Background(), store)
	if err != nil {
		t.Fatalf("listarAlmacen: %v", err)
	}
	// Abrir el proveedor de tarjeta sin lector ni tarjeta bloquea o falla;
	// leer la propiedad es inmediato.
	if transcurrido := time.Since(inicio); transcurrido > 5*time.Second {
		t.Fatalf("listar tardó %v; parece que se ha contactado con el proveedor", transcurrido)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %d; se esperaba solo el certificado con clave declarada", len(refs))
	}
	if refs[0].Fingerprint != huellaDER(derTarjeta) || !refs[0].HasSigningKey {
		t.Fatalf("ref inesperada: %+v", refs[0])
	}
}

func TestTieneClavePrivada_SoloLeeLaPropiedad(t *testing.T) {
	t.Parallel()
	store := nuevoAlmacenMemoria(t)
	sinClave := anadirCertificado(t, store, certificadoSintetico(t, "GrxFirma prueba propiedad"))
	if tieneClavePrivada(sinClave) {
		t.Fatal("un certificado sin CERT_KEY_PROV_INFO no debe anunciar clave")
	}
	asociarClaveEnTarjeta(t, sinClave)
	if !tieneClavePrivada(sinClave) {
		t.Fatal("un certificado con CERT_KEY_PROV_INFO debe anunciar clave")
	}
	if tieneClavePrivada(nil) {
		t.Fatal("un contexto nulo no tiene clave")
	}
}

func TestFirmanteWindows_NoInsisteTrasTarjetaAusente(t *testing.T) {
	t.Parallel()
	cert, err := x509.ParseCertificate(certificadoSintetico(t, "GrxFirma prueba firmante"))
	if err != nil {
		t.Fatal(err)
	}
	firmante, err := nuevoFirmanteWindows(cert)
	if err != nil {
		t.Fatal(err)
	}
	ausente := errors.Join(errors.New("wincertstore: adquiriendo la clave privada"), syscall.Errno(scardWCancelledByUser))
	firmante.accesoDenegado = ausente

	digest := sha256.Sum256([]byte("documento"))
	for range 3 {
		_, err := firmante.Sign(rand.Reader, digest[:], crypto.SHA256)
		if !errors.Is(err, ausente) {
			t.Fatalf("Sign = %v; se esperaba el fallo guardado sin volver a Windows", err)
		}
	}
}
