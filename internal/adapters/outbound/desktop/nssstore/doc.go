// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package nssstore implementa ports.CertificateCatalog leyendo los almacenes NSS
// de Firefox, Chromium y ~/.pki/nssdb en Linux mediante el subproceso certutil(1).
//
// Este paquete solo compila en Linux (build tag linux).
//
// # Rutas NSS exploradas
//
//   - ~/.pki/nssdb              (Chrome / Chromium)
//   - ~/.mozilla/firefox/*      (todos los perfiles Firefox encontrados)
//   - /etc/chromium/nssdb       (Chromium corporativo)
//
// # Dependencia de sistema
//
// Requiere el paquete libnss3-tools instalado (proporciona certutil).
// En Debian/Ubuntu: apt install libnss3-tools
//
// # Nota sobre CGo
//
// Una implementación alternativa directamente sobre libnss3 vía CGo requeriría
// el paquete de desarrollo libnss3-dev. La implementación con certutil es más
// portable y no requiere cabeceras C en tiempo de compilación.
package nssstore
