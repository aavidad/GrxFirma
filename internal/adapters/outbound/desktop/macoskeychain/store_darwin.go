// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin && cgo

// Package macoskeychain implementa ports.CertificateCatalog y ports.SigningKeyProvider
// usando CGo bridge a Security.framework y CoreFoundation de macOS.
//
// Criterio de seguridad: la clave privada NUNCA sale del Keychain. KeyFor() devuelve
// una referencia opaca (SecKeyRef) y la firma se realiza dentro de Security.framework.
package macoskeychain

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

// enumIdentities enumera las identidades (certificado + clave privada) del Keychain
// del usuario actual. Devuelve un array CF con referencias SecIdentityRef; el llamador
// debe liberar el array y cada identidad con CFRelease.
CFArrayRef enumIdentities(void) {
	CFMutableDictionaryRef query = CFDictionaryCreateMutable(
		kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	CFDictionarySetValue(query, kSecClass, kSecClassIdentity);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitAll);
	CFDictionarySetValue(query, kSecReturnRef, kCFBooleanTrue);

	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	CFRelease(query);

	if (status != errSecSuccess || result == NULL) {
		return NULL;
	}
	return (CFArrayRef)result;
}

// identityCount devuelve el numero de elementos de un CFArrayRef.
CFIndex identityCount(CFArrayRef arr) {
	if (arr == NULL) return 0;
	return CFArrayGetCount(arr);
}

// identityAt devuelve la SecIdentityRef en la posicion i (no incrementa retaincount).
SecIdentityRef identityAt(CFArrayRef arr, CFIndex i) {
	return (SecIdentityRef)CFArrayGetValueAtIndex(arr, i);
}

// copyCertFromIdentity copia la SecCertificateRef asociada a la identidad.
// El llamador debe liberar con CFRelease.
SecCertificateRef copyCertFromIdentity(SecIdentityRef ident) {
	SecCertificateRef cert = NULL;
	SecIdentityCopyCertificate(ident, &cert);
	return cert;
}

// copyKeyFromIdentity copia la SecKeyRef (clave privada) asociada a la identidad.
// La clave nunca sale del Keychain; esta referencia es opaca.
// El llamador debe liberar con CFRelease.
SecKeyRef copyKeyFromIdentity(SecIdentityRef ident) {
	SecKeyRef key = NULL;
	SecIdentityCopyPrivateKey(ident, &key);
	return key;
}

// certDERData devuelve los bytes DER del certificado como CFDataRef.
// El llamador debe liberar con CFRelease.
CFDataRef certDERData(SecCertificateRef cert) {
	return SecCertificateCopyData(cert);
}

// keyID devuelve una representacion hexadecimal del identificador de la clave.
// Internamente usa SecKeyCopyAttributes y kSecAttrApplicationLabel.
// Devuelve NULL si no se puede obtener. El llamador debe liberar con CFRelease.
CFStringRef keyIdentifier(SecKeyRef key) {
	if (key == NULL) return NULL;
	CFDictionaryRef attrs = SecKeyCopyAttributes(key);
	if (attrs == NULL) return NULL;
	// kSecAttrApplicationLabel contiene el hash SHA-1 de la clave publica como CFDataRef.
	CFDataRef label = (CFDataRef)CFDictionaryGetValue(attrs, kSecAttrApplicationLabel);
	CFStringRef result = NULL;
	if (label != NULL) {
		const UInt8 *bytes = CFDataGetBytePtr(label);
		CFIndex len = CFDataGetLength(label);
		// Convertir a hex string para devolver un ID legible.
		CFMutableStringRef hex = CFStringCreateMutable(kCFAllocatorDefault, len * 2);
		for (CFIndex i = 0; i < len; i++) {
			CFStringAppendFormat(hex, NULL, CFSTR("%02x"), bytes[i]);
		}
		result = hex;
	}
	CFRelease(attrs);
	return result;
}

// cfStringToUTF8 convierte un CFStringRef a una cadena C en UTF-8.
// El llamador debe liberar con free(). Devuelve NULL si falla.
char* cfStringToUTF8(CFStringRef s) {
	if (s == NULL) return NULL;
	CFIndex len = CFStringGetMaximumSizeForEncoding(
		CFStringGetLength(s), kCFStringEncodingUTF8) + 1;
	char *buf = (char*)malloc(len);
	if (!CFStringGetCString(s, buf, len, kCFStringEncodingUTF8)) {
		free(buf);
		return NULL;
	}
	return buf;
}

// cfDataBytes devuelve un puntero a los bytes de un CFDataRef.
const UInt8* cfDataBytes(CFDataRef d) {
	return CFDataGetBytePtr(d);
}

// cfDataLen devuelve la longitud de un CFDataRef.
CFIndex cfDataLen(CFDataRef d) {
	return CFDataGetLength(d);
}

// signWithKey firma digest usando la clave key con el algoritmo indicado.
// Usa SecKeyCreateSignature que opera dentro del Keychain/Secure Enclave.
// digestAlg debe ser uno de los kSecKeyAlgorithm* para el tipo de clave.
// Devuelve CFDataRef con la firma o NULL en caso de error.
// El llamador debe liberar con CFRelease.
CFDataRef signWithKey(SecKeyRef key, CFStringRef algorithm, const UInt8 *digest, CFIndex digestLen) {
	CFDataRef digestData = CFDataCreate(kCFAllocatorDefault, digest, digestLen);
	if (digestData == NULL) return NULL;

	CFErrorRef error = NULL;
	CFDataRef signature = SecKeyCreateSignature(key, algorithm, digestData, &error);
	CFRelease(digestData);
	if (error != NULL) CFRelease(error);
	return signature;
}
*/
import "C"

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"unsafe"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Almacen implementa ports.CertificateCatalog y ports.SigningKeyProvider
// usando el Keychain del sistema en macOS.
type Almacen struct{}

// New crea un Almacen macOS Keychain.
func New() *Almacen {
	return &Almacen{}
}

// List implementa ports.CertificateCatalog enumerando los certificados con clave
// privada disponibles en el Keychain del usuario.
func (a *Almacen) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// CGo representa los *Ref de CoreFoundation como uintptr; su referencia nula es 0.
	arr := C.enumIdentities()
	if arr == 0 {
		// Sin identidades o acceso denegado — lista vacía sin error.
		return nil, nil
	}
	defer C.CFRelease(C.CFTypeRef(arr))

	count := int(C.identityCount(arr))
	refs := make([]domain.CertificateRef, 0, count)

	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return refs, err
		}

		ident := C.identityAt(arr, C.CFIndex(i))
		if ident == 0 {
			continue
		}

		certRef := C.copyCertFromIdentity(ident)
		if certRef == 0 {
			continue
		}

		ref, err := buildCertRef(certRef)
		C.CFRelease(C.CFTypeRef(certRef))
		if err != nil {
			continue
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

// KeyFor implementa ports.SigningKeyProvider devolviendo una referencia opaca
// (KeychainKey) que contiene la SecKeyRef. La clave privada nunca se extrae del
// Keychain; la firma se realiza dentro de Security.framework.
func (a *Almacen) KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	arr := C.enumIdentities()
	if arr == 0 {
		return nil, errors.New("macoskeychain: sin identidades disponibles en el Keychain")
	}
	defer C.CFRelease(C.CFTypeRef(arr))

	count := int(C.identityCount(arr))
	for i := 0; i < count; i++ {
		ident := C.identityAt(arr, C.CFIndex(i))
		if ident == 0 {
			continue
		}

		certRef := C.copyCertFromIdentity(ident)
		if certRef == 0 {
			continue
		}

		ref, err := buildCertRef(certRef)
		C.CFRelease(C.CFTypeRef(certRef))
		if err != nil {
			continue
		}

		if ref.Fingerprint != certificate.Fingerprint {
			continue
		}

		// Encontrado: copiar la SecKeyRef (no extrae bytes de clave privada).
		keyRef := C.copyKeyFromIdentity(ident)
		if keyRef == 0 {
			return nil, errors.New("macoskeychain: no se pudo obtener la referencia de clave privada")
		}

		keyID := extractKeyID(keyRef)
		if keyID == "" {
			keyID = certificate.Fingerprint
		}

		return &KeychainKey{
			id:     keyID,
			keyRef: keyRef,
		}, nil
	}

	return nil, fmt.Errorf("macoskeychain: certificado no encontrado en Keychain: %s", certificate.Fingerprint)
}

// buildCertRef construye un domain.CertificateRef a partir de una SecCertificateRef.
func buildCertRef(certRef C.SecCertificateRef) (domain.CertificateRef, error) {
	dataRef := C.certDERData(certRef)
	if dataRef == 0 {
		return domain.CertificateRef{}, errors.New("macoskeychain: no se pudieron obtener datos DER del certificado")
	}
	defer C.CFRelease(C.CFTypeRef(dataRef))

	length := int(C.cfDataLen(dataRef))
	if length == 0 {
		return domain.CertificateRef{}, errors.New("macoskeychain: datos DER vacíos")
	}

	derBytes := C.GoBytes(unsafe.Pointer(C.cfDataBytes(dataRef)), C.int(length))

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return domain.CertificateRef{}, fmt.Errorf("macoskeychain: ParseCertificate: %w", err)
	}

	huella := sha256.Sum256(cert.Raw)
	subject := cert.Subject.CommonName
	if subject == "" {
		subject = cert.Subject.String()
	}
	issuer := cert.Issuer.CommonName
	if issuer == "" {
		issuer = cert.Issuer.String()
	}

	fingerprint := hex.EncodeToString(huella[:])
	return domain.CertificateRef{
		ID:            fingerprint,
		Subject:       subject,
		Issuer:        issuer,
		NotAfter:      cert.NotAfter,
		Fingerprint:   fingerprint,
		DER:           cert.Raw,
		HasSigningKey: true,
	}, nil
}

// extractKeyID obtiene el identificador de la clave desde sus atributos.
func extractKeyID(keyRef C.SecKeyRef) string {
	cfID := C.keyIdentifier(keyRef)
	if cfID == 0 {
		return ""
	}
	defer C.CFRelease(C.CFTypeRef(cfID))

	cStr := C.cfStringToUTF8(cfID)
	if cStr == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cStr))

	return C.GoString(cStr)
}

// KeychainKey es una referencia opaca a una SecKeyRef del Keychain de macOS.
// La clave privada NUNCA se extrae; la firma se realiza dentro de Security.framework.
type KeychainKey struct {
	id     string
	keyRef C.SecKeyRef // SecKeyRef retenida; se libera en Close().
}

// KeyID implementa ports.SigningKey.
func (k *KeychainKey) KeyID() string { return k.id }

// CertificateChainDER devuelve nil por ahora en macOS hasta que implementemos
// el retorno de la cadena desde el Keychain.
func (k *KeychainKey) CertificateChainDER() [][]byte {
	return nil
}

// Close libera la SecKeyRef retenida. Debe llamarse cuando la referencia ya no se necesita.
func (k *KeychainKey) Close() {
	if k.keyRef != 0 {
		C.CFRelease(C.CFTypeRef(k.keyRef))
		k.keyRef = 0
	}
}

var _ ports.CertificateCatalog = (*Almacen)(nil)
var _ ports.SigningKeyProvider = (*Almacen)(nil)
var _ ports.SigningKey = (*KeychainKey)(nil)
