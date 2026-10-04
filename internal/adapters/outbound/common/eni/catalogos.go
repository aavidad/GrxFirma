// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package eni

// Los catálogos son la fuente canónica de los códigos NTI. Las descripciones
// se resuelven con la clave eni.codigo.<código> del localizador.
//go:generate python3 ../../../../../scripts/generar_catalogos_eni.py

func EstadosElaboracion() []string {
	return []string{"EE01", "EE02", "EE03", "EE04", "EE99"}
}
func TiposDocumentales() []string {
	return []string{"TD01", "TD02", "TD03", "TD04", "TD05", "TD06", "TD07", "TD08", "TD09", "TD10", "TD11", "TD12", "TD13", "TD14", "TD15", "TD16", "TD17", "TD18", "TD19", "TD20", "TD99"}
}
func EstadosExpediente() []string {
	return []string{"E01", "E02", "E03"}
}
