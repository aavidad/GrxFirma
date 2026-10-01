// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package mobilebind expone una fachada publica y pequena para integracion
// con clientes mobile mediante gomobile bind. La fachada usa solo tipos
// sencillos y JSON en sus metodos publicos para no filtrar el nucleo interno.
//
// Cobertura actual de la fachada:
//   - importacion PKCS#12 en una identidad de sesion no persistente
//   - catalogo y seleccion de esa identidad
//   - firma y verificacion criptografica local
//   - resolucion de perfil de capacidades por plataforma
//   - traduccion bind-friendly de Android Intent a solicitud mobile JSON
//
// El contrato de cada instancia declara de forma explicita las capacidades no
// configuradas, incluidos lotes, intercambio remoto, confianza del sistema y
// persistencia segura de claves.
//
// La capa nativa Android/iOS sigue siendo responsable de:
//   - intents, deep links y share sheets
//   - permisos de ficheros y SAF / sandbox
//   - ciclo de vida visual y callbacks del sistema
package mobilebind
