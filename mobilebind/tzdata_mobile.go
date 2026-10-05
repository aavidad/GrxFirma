// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build android || ios

package mobilebind

// En Android Go arranca con time.Local = UTC y solo encuentra la base de
// zonas en rutas antiguas del sistema (/system/usr/share/zoneinfo); en las
// versiones con la base en APEX, o en iOS, time.LoadLocation puede fallar.
// La base embebida es el respaldo de SetRegion: añade unos 380 KiB por ABI
// a un AAR de unos 49 MB (menos del 1 %), a cambio de que el sello y el
// informe muestren siempre la hora local correcta. Solo entra en el binario
// móvil; el escritorio usa la zona del sistema.
import _ "time/tzdata"
