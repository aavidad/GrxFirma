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
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	hpHashval           = 0x00000002
	ncryptPadPKCS1Flag  = 0x00000002
	ncryptPadPSSFlag    = 0x00000008
	maximaLongitudFirma = 64 << 10
	mascaraEstadoNCrypt = uintptr(1<<32 - 1)
)

var (
	modAdvapi32           = windows.NewLazySystemDLL("advapi32.dll")
	procCryptCreateHash   = modAdvapi32.NewProc("CryptCreateHash")
	procCryptSetHashParam = modAdvapi32.NewProc("CryptSetHashParam")
	procCryptSignHashW    = modAdvapi32.NewProc("CryptSignHashW")
	procCryptDestroyHash  = modAdvapi32.NewProc("CryptDestroyHash")

	modNCrypt            = windows.NewLazySystemDLL("ncrypt.dll")
	procNCryptSignHash   = modNCrypt.NewProc("NCryptSignHash")
	procNCryptFreeObject = modNCrypt.NewProc("NCryptFreeObject")
)

type bcryptPKCS1PaddingInfo struct {
	algID *uint16
}

type bcryptPSSPaddingInfo struct {
	algID *uint16
	salt  uint32
}

// firmanteWindows implementa crypto.Signer sin retener manejadores nativos. El
// mutex serializa los diálogos y proveedores hardware de una misma identidad.
type firmanteWindows struct {
	mu sync.Mutex

	certificado *x509.Certificate
	huella      string
}

type claveWindowsAdquirida struct {
	manejador        windows.Handle
	especificacion   uint32
	liberarManejador bool
}

func nuevoFirmanteWindows(certificado *x509.Certificate) (*firmanteWindows, error) {
	if certificado == nil {
		return nil, errors.New("wincertstore: certificado nulo")
	}
	if err := validarClavePublica(certificado.PublicKey); err != nil {
		return nil, err
	}
	huella := huellaCertificado(certificado)
	if huella == "" {
		return nil, errors.New("wincertstore: certificado sin huella")
	}
	return &firmanteWindows{certificado: certificado, huella: huella}, nil
}

func (f *firmanteWindows) Public() crypto.PublicKey {
	if f == nil || f.certificado == nil {
		return nil
	}
	return f.certificado.PublicKey
}

func (f *firmanteWindows) Sign(_ io.Reader, digest []byte, opciones crypto.SignerOpts) ([]byte, error) {
	if f == nil {
		return nil, errors.New("wincertstore: firmante nulo")
	}
	if opciones == nil {
		return nil, errors.New("wincertstore: opciones de firma nulas")
	}
	hash := opciones.HashFunc()
	parametros, err := resolverParametrosHash(hash)
	if err != nil {
		return nil, err
	}
	if len(digest) != hash.Size() {
		return nil, fmt.Errorf("wincertstore: digest de longitud invalida: %d (esperada %d para %v)", len(digest), hash.Size(), hash)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	return f.firmarConClaveAdquirida(digest, hash, parametros, opciones)
}

func (f *firmanteWindows) firmarConClaveAdquirida(digest []byte, hash crypto.Hash, parametros parametrosHash, opciones crypto.SignerOpts) ([]byte, error) {
	store, err := abrirAlmacenPersonal()
	if err != nil {
		return nil, err
	}
	defer windows.CertCloseStore(store, 0) //nolint:errcheck -- no quedan contextos al retornar

	var firma []byte
	encontrada := false
	err = recorrerCertificados(context.Background(), store, func(certCtx *windows.CertContext) (bool, error) {
		der, err := copiarDER(certCtx)
		if err != nil {
			return true, nil
		}
		_, certificado, err := construirRefDesdeDER(der)
		if err != nil || huellaCertificado(certificado) != f.huella {
			return true, nil
		}
		encontrada = true
		if !tieneClavePrivada(certCtx) {
			return false, fmt.Errorf("wincertstore: el certificado %s ya no tiene clave privada asociada", f.huella)
		}

		clave, err := adquirirClaveWindows(certCtx)
		if err != nil {
			return false, fmt.Errorf("wincertstore: adquiriendo la clave privada de %s: %w", f.huella, err)
		}
		if clave.especificacion == windows.CERT_NCRYPT_KEY_SPEC {
			firma, err = f.firmarCNG(clave, digest, hash, parametros, opciones)
		} else {
			firma, err = f.firmarCAPI(clave, digest, parametros, opciones)
		}
		return false, errors.Join(err, clave.liberar())
	})
	if err != nil {
		return nil, err
	}
	if !encontrada {
		return nil, fmt.Errorf("wincertstore: el certificado de firma ya no está en el almacén MY: %s", f.huella)
	}
	return firma, nil
}

func adquirirClaveWindows(certCtx *windows.CertContext) (claveWindowsAdquirida, error) {
	return adquirirClavePreferida(func(flags uint32) (claveWindowsAdquirida, error) {
		var clave claveWindowsAdquirida
		err := windows.CryptAcquireCertificatePrivateKey(
			certCtx, flags, nil,
			&clave.manejador, &clave.especificacion, &clave.liberarManejador,
		)
		return clave, err
	})
}

func adquirirClavePreferida(adquirir func(uint32) (claveWindowsAdquirida, error)) (claveWindowsAdquirida, error) {
	// ALLOW_NCRYPT intenta primero CAPI: una clave también accesible por CNG
	// puede acabar en un CSP antiguo sin SHA-256. PREFER conserva el fallback
	// CAPI para dispositivos que lo necesitan, pero elige CNG cuando existe.
	// No se exporta/reimporta la clave ni se rebaja el algoritmo solicitado.
	// https://learn.microsoft.com/windows/win32/api/wincrypt/nf-wincrypt-cryptacquirecertificateprivatekey
	clave, err := adquirir(windows.CRYPT_ACQUIRE_PREFER_NCRYPT_KEY_FLAG | windows.CRYPT_ACQUIRE_COMPARE_KEY_FLAG)
	if err != nil {
		return claveWindowsAdquirida{}, err
	}
	if clave.manejador == 0 {
		return claveWindowsAdquirida{}, errors.New("wincertstore: CryptoAPI devolvió un manejador de clave nulo")
	}
	return clave, nil
}

func (f *firmanteWindows) firmarCNG(clave claveWindowsAdquirida, digest []byte, hash crypto.Hash, parametros parametrosHash, opciones crypto.SignerOpts) ([]byte, error) {
	algID, err := windows.UTF16PtrFromString(parametros.nombreCNG)
	if err != nil {
		return nil, fmt.Errorf("wincertstore: preparando algoritmo CNG: %w", err)
	}

	var padding unsafe.Pointer
	var flags uint32
	var pkcs1 bcryptPKCS1PaddingInfo
	var pss bcryptPSSPaddingInfo

	switch publica := f.certificado.PublicKey.(type) {
	case *rsa.PublicKey:
		if opcionesPSS, ok := opciones.(*rsa.PSSOptions); ok {
			longitudSal, err := longitudSalPSS(publica, hash, opcionesPSS)
			if err != nil {
				return nil, err
			}
			pss = bcryptPSSPaddingInfo{algID: algID, salt: longitudSal}
			padding = unsafe.Pointer(&pss)
			flags = ncryptPadPSSFlag
		} else {
			pkcs1 = bcryptPKCS1PaddingInfo{algID: algID}
			padding = unsafe.Pointer(&pkcs1)
			flags = ncryptPadPKCS1Flag
		}
	case *ecdsa.PublicKey:
		if _, ok := opciones.(*rsa.PSSOptions); ok {
			return nil, errors.New("wincertstore: RSA-PSS no es aplicable a una clave ECDSA")
		}
	default:
		return nil, fmt.Errorf("wincertstore: clave CNG no soportada: %T", publica)
	}

	firma, err := ejecutarNCryptSignHash(clave.manejador, padding, digest, flags)
	runtime.KeepAlive(algID)
	runtime.KeepAlive(pkcs1)
	runtime.KeepAlive(pss)
	if err != nil {
		return nil, err
	}
	if publica, ok := f.certificado.PublicKey.(*ecdsa.PublicKey); ok {
		return firmaECDSADesdeCNG(firma, publica)
	}
	return firma, nil
}

func ejecutarNCryptSignHash(manejador windows.Handle, padding unsafe.Pointer, digest []byte, flags uint32) ([]byte, error) {
	var longitud uint32
	estado, _, _ := procNCryptSignHash.Call(
		uintptr(manejador),
		uintptr(padding),
		uintptr(unsafe.Pointer(&digest[0])),
		uintptr(len(digest)),
		0,
		0,
		uintptr(unsafe.Pointer(&longitud)),
		uintptr(flags),
	)
	estado = normalizarEstadoNCrypt(estado)
	if estado != 0 {
		return nil, errorEstadoNCrypt("NCryptSignHash(tamaño)", estado)
	}
	if longitud == 0 || longitud > maximaLongitudFirma {
		return nil, fmt.Errorf("wincertstore: NCryptSignHash devolvió una longitud de firma inválida: %d", longitud)
	}

	firma := make([]byte, int(longitud))
	estado, _, _ = procNCryptSignHash.Call(
		uintptr(manejador),
		uintptr(padding),
		uintptr(unsafe.Pointer(&digest[0])),
		uintptr(len(digest)),
		uintptr(unsafe.Pointer(&firma[0])),
		uintptr(longitud),
		uintptr(unsafe.Pointer(&longitud)),
		uintptr(flags),
	)
	estado = normalizarEstadoNCrypt(estado)
	if estado != 0 {
		return nil, errorEstadoNCrypt("NCryptSignHash", estado)
	}
	if longitud == 0 || int(longitud) > len(firma) {
		return nil, fmt.Errorf("wincertstore: NCryptSignHash escribió una longitud de firma inválida: %d", longitud)
	}
	return firma[:longitud], nil
}

func (f *firmanteWindows) firmarCAPI(clave claveWindowsAdquirida, digest []byte, parametros parametrosHash, opciones crypto.SignerOpts) ([]byte, error) {
	if _, ok := opciones.(*rsa.PSSOptions); ok {
		return nil, errors.New("wincertstore: el proveedor CryptoAPI heredado no soporta RSA-PSS")
	}
	if _, ok := f.certificado.PublicKey.(*rsa.PublicKey); !ok {
		return nil, fmt.Errorf("wincertstore: CryptoAPI heredada solo soporta RSA, recibida %T", f.certificado.PublicKey)
	}

	var hashHandle uintptr
	ret, _, callErr := procCryptCreateHash.Call(
		uintptr(clave.manejador),
		uintptr(parametros.algIDCAPI),
		0,
		0,
		uintptr(unsafe.Pointer(&hashHandle)),
	)
	if ret == 0 {
		return nil, errorLlamadaWindows("CryptCreateHash", callErr)
	}
	defer procCryptDestroyHash.Call(hashHandle) //nolint:errcheck -- el resultado criptográfico ya está materializado

	ret, _, callErr = procCryptSetHashParam.Call(
		hashHandle,
		hpHashval,
		uintptr(unsafe.Pointer(&digest[0])),
		0,
	)
	if ret == 0 {
		return nil, errorLlamadaWindows("CryptSetHashParam", callErr)
	}

	var longitud uint32
	ret, _, callErr = procCryptSignHashW.Call(
		hashHandle,
		uintptr(clave.especificacion),
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&longitud)),
	)
	if ret == 0 {
		return nil, errorLlamadaWindows("CryptSignHashW(tamaño)", callErr)
	}
	if longitud == 0 || longitud > maximaLongitudFirma {
		return nil, fmt.Errorf("wincertstore: CryptSignHashW devolvió una longitud de firma inválida: %d", longitud)
	}

	firmaLittleEndian := make([]byte, int(longitud))
	ret, _, callErr = procCryptSignHashW.Call(
		hashHandle,
		uintptr(clave.especificacion),
		0,
		0,
		uintptr(unsafe.Pointer(&firmaLittleEndian[0])),
		uintptr(unsafe.Pointer(&longitud)),
	)
	if ret == 0 {
		return nil, errorLlamadaWindows("CryptSignHashW", callErr)
	}
	if longitud == 0 || int(longitud) > len(firmaLittleEndian) {
		return nil, fmt.Errorf("wincertstore: CryptSignHashW escribió una longitud de firma inválida: %d", longitud)
	}
	// CryptoAPI documenta la firma RSA en little-endian; crypto.Signer y los
	// formatos interoperables esperan la representación big-endian.
	return invertirCopia(firmaLittleEndian[:longitud]), nil
}

func (c claveWindowsAdquirida) liberar() error {
	var erroresLiberacion []error
	if c.manejador != 0 && c.liberarManejador {
		if c.especificacion == windows.CERT_NCRYPT_KEY_SPEC {
			estado, _, _ := procNCryptFreeObject.Call(uintptr(c.manejador))
			estado = normalizarEstadoNCrypt(estado)
			if estado != 0 {
				erroresLiberacion = append(erroresLiberacion, errorEstadoNCrypt("NCryptFreeObject", estado))
			}
		} else if err := windows.CryptReleaseContext(c.manejador, 0); err != nil {
			erroresLiberacion = append(erroresLiberacion, fmt.Errorf("wincertstore: CryptReleaseContext: %w", err))
		}
	}
	return errors.Join(erroresLiberacion...)
}

func normalizarEstadoNCrypt(estado uintptr) uintptr {
	return estado & mascaraEstadoNCrypt
}

func errorEstadoNCrypt(operacion string, estado uintptr) error {
	estado = normalizarEstadoNCrypt(estado)
	errno := syscall.Errno(estado)
	return fmt.Errorf("wincertstore: %s: estado 0x%08x: %w", operacion, estado, errno)
}

func errorLlamadaWindows(operacion string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return fmt.Errorf("wincertstore: %s falló sin código de error", operacion)
	}
	return fmt.Errorf("wincertstore: %s: %w", operacion, err)
}

var _ crypto.Signer = (*firmanteWindows)(nil)
