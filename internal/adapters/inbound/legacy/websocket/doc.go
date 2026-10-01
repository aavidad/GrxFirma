// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package websocket implementa el adaptador heredado de mensajes WebSocket.
// Este paquete no levanta un servidor WSS: solo traduce mensajes de texto legacy
// al modelo interno, de forma que el transporte pueda cambiar sin tocar el nucleo.
package websocket
