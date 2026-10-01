// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package certcatalogagg implementa ports.CertificateCatalog agregando múltiples
// fuentes de certificados (NSS Linux, PKCS#11, Windows CertStore, macOS Keychain,
// importaciones P12) en una sola vista deduplicada.
//
// Si una fuente falla, la agregación continúa con las demás (degradación controlada).
// La deduplicación se realiza por fingerprint SHA-256.
package certcatalogagg
