// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows && !darwin

package machinepolicy

// PolicyFile es la política de máquina en sistemas sin registro. Debe ser
// propiedad de root y no escribible por usuarios.
const PolicyFile = "/etc/grxfirma/policy.json"
