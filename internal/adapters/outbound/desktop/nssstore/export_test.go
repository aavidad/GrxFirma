// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore

import "context"

// ParsarListaNSS expone parsearListaNSS para los tests externos del paquete.
var ParsarListaNSS = parsearListaNSS

// NewConOpenSSL expone la inyección del binario openssl para tests externos.
func NewConOpenSSL(opensslPath string) *Almacen {
	return &Almacen{openssl: opensslPath}
}

// ConvertirP12APEM expone convertirP12APEM para validar la invocación segura de openssl.
func ConvertirP12APEM(a *Almacen, p12Path, password string) ([]byte, error) {
	return a.convertirP12APEM(context.Background(), p12Path, password)
}

// ResolverPasswordExportacionNSS expone resolverPasswordExportacionNSS para tests.
var ResolverPasswordExportacionNSS = resolverPasswordExportacionNSS

// RutasEstandar expone rutasEstandar para validar discovery de perfiles Linux.
var RutasEstandar = rutasEstandar
