// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package auditlog implementa un EvidenceLogger rotativo en formato JSONL.
// Recibe exclusivamente evidencias ya saneadas por application.AuditUseCase.
// La política local conserva el fichero activo y una rotación de hasta 10 MiB
// cada uno durante un máximo de 90 días. Cada registro lleva la huella del
// anterior (campo "cadena"), de modo que VerificarCadena detecta
// modificaciones accidentales. La huella no lleva clave: no protege frente a
// quien puede escribir el fichero, que puede editar un registro y recalcular
// las huellas siguientes.
package auditlog
