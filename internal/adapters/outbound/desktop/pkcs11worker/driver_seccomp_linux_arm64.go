// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import "golang.org/x/sys/unix"

const driverAuditArch = unix.AUDIT_ARCH_AARCH64

var driverForkSyscalls = []uint32{}
