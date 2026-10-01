// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package pkcs12importer centraliza la importacion de credenciales locales
// para V2. Aunque el nombre historico menciona PKCS#12, el adaptador tambien
// soporta certificados y claves en PEM para evitar logica duplicada en cmd/.
package pkcs12importer
