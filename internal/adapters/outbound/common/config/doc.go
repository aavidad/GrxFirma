// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package config implementa el sistema de configuración de GrxFirma.
//
// Jerarquía de precedencia (de mayor a menor):
//  1. /etc/grxfirma/policy.json — política de organización (solo admins)
//  2. Variables de entorno GRXFIRMA_* — override por proceso
//  3. ~/.config/grxfirma/config.json — configuración del usuario
//  4. Valores por defecto compilados en Go
//
// Los valores por defecto garantizan el modelo Zero Server por defecto:
// sin puertos abiertos al arrancar (websocket_habilitado=false, rest_habilitado=false).
//
// policy.json puede fijar tofu_habilitado=false y dominios_de_confianza para
// forzar un modo allowlist en el perímetro de confianza. Esa allowlist de
// organización no es una segunda fuente operativa: debe inyectarse en el
// truststore como ExtraAllowed y evaluarse siempre mediante TrustPolicy.
package config
