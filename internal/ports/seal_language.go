// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import "strings"

// ClaveIdiomaSello es la preferencia «idioma del sello»: vacía o
// IdiomaSelloInterfaz para que el sello siga el idioma de la interfaz que
// firma, o un idioma fijo (p. ej. "es" para documentos de la Administración).
const (
	ClaveIdiomaSello    = "signSealLanguage"
	IdiomaSelloInterfaz = "interface"
)

// IdiomaSelloFijo devuelve el idioma fijo configurado para el sello, o "" si
// el sello debe seguir a la interfaz. Solo admite etiquetas cortas de
// idioma; cualquier otro valor se trata como «seguir a la interfaz».
func IdiomaSelloFijo(doc DocumentoConfiguracionUsuario) string {
	valor, _ := doc.Extras[ClaveIdiomaSello].(string)
	valor = strings.TrimSpace(valor)
	if valor == "" || strings.EqualFold(valor, IdiomaSelloInterfaz) || len(valor) > 16 {
		return ""
	}
	for _, r := range valor {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_') {
			return ""
		}
	}
	return valor
}
