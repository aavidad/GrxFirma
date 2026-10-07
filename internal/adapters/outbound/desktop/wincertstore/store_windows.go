// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

// Package wincertstore integra el almacén personal MY de Windows con los
// contratos de catálogo y clave de firma de la aplicación.
package wincertstore

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"

	desksigner "grxfirma/internal/adapters/outbound/desktop/signer"
)

const (
	nombreAlmacenPersonal                        = "MY"
	maximoDERCertificado                         = 4 << 20
	maximoCertificadosCadena                     = 64
	certChainCacheOnlyURLRetrieval               = 0x00000004
	certChainDisableAIA                          = 0x00002000
	certKeyProvInfoPropID                        = 2
	cryptENotFound                 syscall.Errno = 0x80092004
)

var (
	modCrypt32                            = windows.NewLazySystemDLL("crypt32.dll")
	procCertGetCertificateContextProperty = modCrypt32.NewProc("CertGetCertificateContextProperty")
)

// Almacen implementa el catálogo y la resolución de claves privadas del
// almacén personal del usuario actual. El material privado nunca sale del CSP,
// KSP, tarjeta criptográfica o TPM que lo custodia.
type Almacen struct{}

// New crea un almacén Windows CertStore.
func New() *Almacen {
	return &Almacen{}
}

// List enumera los certificados de firma de entidad final del almacén MY que
// tienen una clave privada asociada. Los certificados CA, los que restringen
// su clave a usos distintos de firma y los certificados malformados se omiten
// sin invalidar el resto del catálogo.
//
// Listar no abre claves privadas ni contacta con su proveedor: la interfaz y
// el host de Native Messaging consultan el catálogo sin acción del usuario, y
// con una clave en tarjeta Windows pediría insertarla o buscaría el lector.
func (a *Almacen) List(ctx context.Context) ([]domain.CertificateRef, error) {
	store, err := abrirAlmacenPersonal()
	if err != nil {
		return nil, err
	}
	defer windows.CertCloseStore(store, 0) //nolint:errcheck -- no invalida un catálogo ya leído
	return listarAlmacen(ctx, store)
}

func listarAlmacen(ctx context.Context, store windows.Handle) ([]domain.CertificateRef, error) {
	refs := make([]domain.CertificateRef, 0)
	err := recorrerCertificados(ctx, store, func(certCtx *windows.CertContext) (bool, error) {
		if !tieneClavePrivada(certCtx) {
			return true, nil
		}
		der, err := copiarDER(certCtx)
		if err != nil {
			return true, nil
		}
		ref, cert, err := construirRefDesdeDER(der)
		if err != nil ||
			validarClavePublica(cert.PublicKey) != nil ||
			!certificadoAptoParaCatalogo(cert) {
			return true, nil
		}
		ref.HasSigningKey = true
		refs = append(refs, ref)
		return true, nil
	})
	if err != nil {
		return refs, err
	}
	return refs, nil
}

// KeyFor adquiere la clave privada vinculada al certificado y la adapta a la
// ClaveLocal que consumen los motores CAdES, XAdES, PAdES y trifásicos. La
// firma se ejecuta dentro del proveedor criptográfico de Windows.
func (a *Almacen) KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	huella, err := huellaBuscada(certificate)
	if err != nil {
		return nil, err
	}

	store, err := abrirAlmacenPersonal()
	if err != nil {
		return nil, err
	}
	defer windows.CertCloseStore(store, 0) //nolint:errcheck -- no quedan contextos al retornar

	var clave ports.SigningKey
	err = recorrerCertificados(ctx, store, func(certCtx *windows.CertContext) (bool, error) {
		der, err := copiarDER(certCtx)
		if err != nil {
			return true, nil
		}
		ref, cert, err := construirRefDesdeDER(der)
		if err != nil || ref.Fingerprint != huella {
			return true, nil
		}
		if err := validarClavePublica(cert.PublicKey); err != nil {
			return false, err
		}
		if !tieneClavePrivada(certCtx) {
			return false, fmt.Errorf("wincertstore: el certificado %s no tiene clave privada asociada", huella)
		}

		firmante, err := nuevoFirmanteWindows(cert)
		if err != nil {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		cadena := obtenerCadena(certCtx, cert)
		clave = desksigner.NuevaClaveLocalConCadena(firmante, cert, cadena)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	if clave == nil {
		return nil, fmt.Errorf("wincertstore: certificado no encontrado en el almacén MY: %s", huella)
	}
	return clave, nil
}

func abrirAlmacenPersonal() (windows.Handle, error) {
	nombre, err := windows.UTF16PtrFromString(nombreAlmacenPersonal)
	if err != nil {
		return 0, fmt.Errorf("wincertstore: codificando nombre del almacén: %w", err)
	}
	store, err := windows.CertOpenSystemStore(0, nombre)
	if err != nil {
		return 0, fmt.Errorf("wincertstore: abriendo almacén MY: %w", err)
	}
	return store, nil
}

func recorrerCertificados(ctx context.Context, store windows.Handle, visitar func(*windows.CertContext) (bool, error)) error {
	var actual *windows.CertContext
	defer func() {
		if actual != nil {
			_ = windows.CertFreeCertificateContext(actual)
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		siguiente, err := windows.CertEnumCertificatesInStore(store, actual)
		// CryptoAPI libera actual al usarlo como pPrevCertContext, incluso al
		// alcanzar el final del almacén.
		actual = nil
		if err != nil {
			if errors.Is(err, cryptENotFound) {
				return nil
			}
			return fmt.Errorf("wincertstore: enumerando certificados: %w", err)
		}
		actual = siguiente
		continuar, err := visitar(actual)
		if err != nil {
			return err
		}
		if !continuar {
			return nil
		}
	}
}

// tieneClavePrivada indica si el certificado declara una clave privada
// asociada leyendo solo su propiedad CERT_KEY_PROV_INFO_PROP_ID, guardada en
// el propio almacén. No abre la clave ni el proveedor (CSP, KSP, tarjeta o
// TPM), así que nunca muestra diálogos ni busca lectores.
//
// No se usa CryptFindCertificateKeyProvInfo: recorre todos los proveedores y
// sus contenedores, incluido el de tarjeta inteligente, para buscar la clave,
// y sin CRYPT_FIND_SILENT_KEYSET_FLAG puede pedir que se inserte la tarjeta
// por cada certificado y en cada listado.
func tieneClavePrivada(certCtx *windows.CertContext) bool {
	if certCtx == nil {
		return false
	}
	var tamano uint32
	ret, _, _ := procCertGetCertificateContextProperty.Call(
		uintptr(unsafe.Pointer(certCtx)),
		certKeyProvInfoPropID,
		0,
		uintptr(unsafe.Pointer(&tamano)),
	)
	return ret != 0 && tamano > 0
}

func copiarDER(certCtx *windows.CertContext) ([]byte, error) {
	if certCtx == nil || certCtx.EncodedCert == nil || certCtx.Length == 0 {
		return nil, errors.New("wincertstore: contexto de certificado sin datos DER")
	}
	if certCtx.Length > maximoDERCertificado {
		return nil, fmt.Errorf("wincertstore: certificado DER demasiado grande: %d bytes", certCtx.Length)
	}
	der := unsafe.Slice(certCtx.EncodedCert, int(certCtx.Length))
	return append([]byte(nil), der...), nil
}

func obtenerCadena(certCtx *windows.CertContext, hoja *x509.Certificate) []*x509.Certificate {
	parametros := windows.CertChainPara{Size: uint32(unsafe.Sizeof(windows.CertChainPara{}))}
	var contextoCadena *windows.CertChainContext
	if err := windows.CertGetCertificateChain(
		0,
		certCtx,
		nil,
		certCtx.Store,
		&parametros,
		certChainCacheOnlyURLRetrieval|certChainDisableAIA,
		0,
		&contextoCadena,
	); err != nil || contextoCadena == nil {
		return nil
	}
	defer windows.CertFreeCertificateChain(contextoCadena)

	if contextoCadena.ChainCount == 0 || contextoCadena.Chains == nil || contextoCadena.ChainCount > maximoCertificadosCadena {
		return nil
	}
	cadenas := unsafe.Slice(contextoCadena.Chains, int(contextoCadena.ChainCount))
	if len(cadenas) == 0 || cadenas[len(cadenas)-1] == nil {
		return nil
	}
	// Windows ordena las cadenas simples desde el certificado inicial hasta la
	// cadena final construida; crypto/x509 usa igualmente la última.
	cadena := cadenas[len(cadenas)-1]
	if cadena.NumElements == 0 || cadena.Elements == nil || cadena.NumElements > maximoCertificadosCadena {
		return nil
	}

	elementos := unsafe.Slice(cadena.Elements, int(cadena.NumElements))
	certificados := make([]*x509.Certificate, 0, len(elementos))
	for _, elemento := range elementos {
		if elemento == nil || elemento.CertContext == nil {
			continue
		}
		der, err := copiarDER(elemento.CertContext)
		if err != nil {
			continue
		}
		cert, err := x509.ParseCertificate(der)
		if err == nil {
			certificados = append(certificados, cert)
		}
	}
	return filtrarCadena(hoja, certificados)
}

var _ ports.CertificateCatalog = (*Almacen)(nil)
var _ ports.SigningKeyProvider = (*Almacen)(nil)
