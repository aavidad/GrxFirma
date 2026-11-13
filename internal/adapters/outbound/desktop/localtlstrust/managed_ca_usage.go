// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import "encoding/asn1"

// oidExtKeyUsageServerAuth es id-kp-serverAuth (RFC 5280, 4.2.1.12).
var oidExtKeyUsageServerAuth = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1}

// serverAuthOnlyEnhancedKeyUsageDER codifica ExtKeyUsageSyntax con un único
// uso, autenticación de servidor TLS. Windows lo guarda como propiedad del
// certificado en el almacén (CERT_ENHKEY_USAGE_PROP_ID) para que la raíz solo
// se confíe para ese uso aunque alguien emita otra cosa con su clave (B-01).
func serverAuthOnlyEnhancedKeyUsageDER() ([]byte, error) {
	return asn1.Marshal([]asn1.ObjectIdentifier{oidExtKeyUsageServerAuth})
}
