// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && cgo && nss_cgo

// Listado nativo de almacenes NSS vía libnss3 (T032). Se activa compilando
// con -tags nss_cgo y requiere libnss3-dev (pkg-config nss). Evita el
// subproceso certutil para el catálogo; si la apertura nativa falla en una
// ruta, se degrada al fallback certutil de esa ruta.
//
// La exportación de clave privada (KeyFor) sigue vía pk12util: el material
// de clave no se maneja por CGo.
package nssstore

/*
#cgo pkg-config: nss
#include <stdlib.h>
#include <nss.h>
#include <cert.h>
#include <pk11pub.h>

// Wrappers para las macros de lista (CGo no expande macros con argumentos).
static CERTCertListNode* grxfirma_list_head(CERTCertList *l) { return CERT_LIST_HEAD(l); }
static int grxfirma_list_end(CERTCertListNode *n, CERTCertList *l) { return CERT_LIST_END(n, l); }
static CERTCertListNode* grxfirma_list_next(CERTCertListNode *n) { return CERT_LIST_NEXT(n); }
static CERTCertificate* grxfirma_node_cert(CERTCertListNode *n) { return n->cert; }

// Callback de password que se rinde: nunca bloquear esperando un PIN
// interactivo durante un listado. Los slots protegidos se listan igualmente
// en su parte pública.
static char* grxfirma_no_password(PK11SlotInfo *slot, PRBool retry, void *arg) {
	(void)slot; (void)retry; (void)arg;
	return NULL;
}

static void grxfirma_set_no_password(void) {
	PK11_SetPasswordFunc(grxfirma_no_password);
}
*/
import "C"

import (
	"context"
	"crypto/x509"
	"fmt"
	"sync"
	"unsafe"

	"grxfirma/internal/domain"
)

// nssMu serializa el acceso a libnss3: la inicialización de contextos NSS
// comparte estado de proceso (softoken, secmod).
var nssMu sync.Mutex

var configurarPasswordUnaVez sync.Once

// listarEnRuta enumera nativamente con libnss3 y cae al subproceso certutil
// si la apertura de la base falla (formato antiguo, permisos, corrupción).
func (a *Almacen) listarEnRuta(ctx context.Context, ruta string) ([]domain.CertificateRef, error) {
	refs, err := listarEnRutaNativo(ctx, ruta)
	if err == nil {
		return refs, nil
	}
	return a.listarEnRutaCertutil(ctx, ruta)
}

func listarEnRutaNativo(ctx context.Context, ruta string) ([]domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	nssMu.Lock()
	defer nssMu.Unlock()

	configurarPasswordUnaVez.Do(func() { C.grxfirma_set_no_password() })

	configdir := C.CString(rutaCertutil(ruta))
	defer C.free(unsafe.Pointer(configdir))
	vacio := C.CString("")
	defer C.free(unsafe.Pointer(vacio))

	nssCtx := C.NSS_InitContext(configdir, vacio, vacio, vacio, nil,
		C.NSS_INIT_READONLY|C.NSS_INIT_NOROOTINIT)
	if nssCtx == nil {
		return nil, fmt.Errorf("nssstore: NSS_InitContext falló para %s", ruta)
	}
	defer C.NSS_ShutdownContext(nssCtx)

	// PK11CertListUser: solo certificados con clave privada en el almacén,
	// el mismo criterio que el trust "u,u,u" de certutil -L.
	lista := C.PK11_ListCerts(C.PK11CertListUser, nil)
	if lista == nil {
		return nil, nil
	}
	defer C.CERT_DestroyCertList(lista)

	var refs []domain.CertificateRef
	for nodo := C.grxfirma_list_head(lista); C.grxfirma_list_end(nodo, lista) == 0; nodo = C.grxfirma_list_next(nodo) {
		c := C.grxfirma_node_cert(nodo)
		if c == nil || c.derCert.data == nil || c.derCert.len == 0 {
			continue
		}
		der := C.GoBytes(unsafe.Pointer(c.derCert.data), C.int(c.derCert.len))
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			continue // entradas corruptas no bloquean el resto
		}
		refs = append(refs, construirRef(cert))
	}
	return refs, nil
}
