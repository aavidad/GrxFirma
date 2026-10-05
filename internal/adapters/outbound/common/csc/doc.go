// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package csc es un prototipo de cliente de la API del Cloud Signature
// Consortium (CSC API v2.x) para firmar con certificados custodiados por un
// prestador remoto.
//
// Alcance del prototipo:
//
//   - descubrimiento del servicio (POST /csc/v2/info);
//   - autorización OAuth 2.0 «Authorization Code» con PKCE (S256) para un
//     cliente público, abriendo el navegador del sistema y recibiendo el
//     código en http://127.0.0.1:<puerto efímero>/callback (RFC 8252);
//   - listado e información de credenciales (credentials/list e info);
//   - autorización de la credencial (credentials/authorize, SCAL1 y SCAL2,
//     modos implicit, explicit y oauth2code);
//   - firma de resúmenes (signatures/signHash) mediante [FirmanteRemoto], un
//     crypto.Signer que encaja donde el motor usa una clave local.
//
// Solo el resumen sale del equipo: el documento y los atributos firmados se
// calculan en local. El firmante comprueba cada firma recibida con la clave
// pública del certificado antes de devolverla.
//
// Seguridad: solo https con la validación TLS del sistema, tiempos máximos,
// respuestas acotadas, ninguna redirección HTTP y servidor OAuth en el mismo
// host que el servicio (salvo pares configurados). Tokens, SAD, PIN y OTP
// solo están en memoria: el paquete los guarda en []byte, compone con bytes
// los cuerpos que los llevan y los borra al terminar o con Close. El borrado
// no alcanza todas las copias: la cabecera Authorization tiene que ser un
// string para net/http y quien llama puede haber leído el PIN como string.
// Nunca se registran tokens, PIN ni OTP. El paquete no contiene textos
// visibles: los errores llevan un [Codigo] que la capa de presentación
// traduce con el catálogo.
//
// El prototipo está desactivado por defecto y no se ha probado con
// prestadores reales; véase docs/FIRMA_REMOTA_CSC.md.
package csc
