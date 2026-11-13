// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package machinepolicy

// PolicyFile es la política de máquina en macOS. En macOS /etc es un enlace
// simbólico a /private/etc y la lectura no sigue enlaces en ningún componente
// de la ruta, así que se usa directamente el destino real. Debe ser propiedad
// de root y no escribible por usuarios.
const PolicyFile = "/private/etc/grxfirma/policy.json"
