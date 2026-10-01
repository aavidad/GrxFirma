// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package desktopnotify implementa ports.DesktopNotification emitiendo notificaciones
// del sistema operativo al finalizar una operación de firma.
//
// Implementaciones por plataforma:
//   - Linux: subprocess notify-send (libnotify). No bloquea el hilo principal.
//   - Windows: build tag windows — usa golang.org/x/sys/windows (stub en otras plataformas).
//   - macOS: build tag darwin — NSUserNotification vía CGo (stub en otras plataformas).
//
// Si notify-send no está disponible en Linux, Notify retorna nil sin notificar
// (la operación de firma ya terminó — la notificación es decorativa).
package desktopnotify
