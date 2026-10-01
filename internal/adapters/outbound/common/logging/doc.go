// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package logging centraliza la construccion de loggers estructurados con slog.
//
// Reglas:
//   - GRXFIRMA_LOG_LEVEL controla el nivel: DEBUG, INFO, WARN, ERROR.
//   - GRXFIRMA_DEBUG=1 fuerza nivel DEBUG en builds de desarrollo.
//   - La etiqueta de build production ignora cualquier petición de DEBUG.
//   - GRXFIRMA_LOG_FILE=/ruta/fichero duplica la salida al fichero indicado.
//   - GRXFIRMA_ENV=production|prod fuerza salida JSON.
//   - En otros entornos se usa salida de texto para facilitar el desarrollo.
package logging
