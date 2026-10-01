// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && (!cgo || !nss_cgo)

package nssstore

import (
	"context"

	"grxfirma/internal/domain"
)

// listarEnRuta enumera vía subproceso certutil. La variante nativa con
// libnss3 (T032) se activa compilando con -tags nss_cgo (requiere
// libnss3-dev); ver list_cgo.go.
func (a *Almacen) listarEnRuta(ctx context.Context, ruta string) ([]domain.CertificateRef, error) {
	return a.listarEnRutaCertutil(ctx, ruta)
}
