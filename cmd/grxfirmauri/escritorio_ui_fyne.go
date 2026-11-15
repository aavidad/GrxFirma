// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import "grxfirma/internal/adapters/outbound/desktop/escritorio"

// comprobarEscritorioUI decide si esta compilación con ventanas puede abrir
// un diálogo. Sin escritorio visible devuelve el error tipado de escritorio y
// la operación sale denegada: nadie podría responder. Las pruebas que
// simulan los diálogos lo sustituyen.
var comprobarEscritorioUI = escritorio.Comprobar
