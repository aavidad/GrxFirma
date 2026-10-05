<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Firma remota CSC (prototipo)

**Estado:** prototipo en el motor y la línea de órdenes, desactivado por defecto.
Solo se ha probado contra un servidor CSC simulado. No se ha probado con ningún
prestador real ni con la firma remota de la cartera europea (EUDI).

## Qué es

Un cliente de la API del Cloud Signature Consortium (CSC API v2.x) que permite
firmar con un certificado que custodia un prestador de servicios de confianza en
lugar de estar en el equipo, una tarjeta o un fichero P12. Es la interfaz que usan
los prestadores cualificados de firma remota y la que prevé el marco técnico de la
cartera europea para firmar.

GrxFirma calcula en el equipo el resumen (hash) de lo que se firma. Al prestador
solo le llega ese resumen; el documento no sale del equipo. La firma que devuelve
el prestador se comprueba con la clave pública del certificado antes de usarla.
PAdES, CAdES y XAdES funcionan igual que con un certificado local (XAdES, como hoy
en el motor, solo con claves RSA).

El código está en `internal/adapters/outbound/common/csc`. La clave remota es un
`crypto.Signer` que se envuelve en la misma clave que usa el motor para los
certificados locales.

## Cómo activarlo

Hace falta una de estas dos cosas:

- `"firma_remota_csc": true` en el `config.json` del usuario, o
- la opción `-csc-activar` en cada ejecución.

Una organización puede prohibirlo con `"firma_remota_csc": false` en
`policy.json` (`/etc/grxfirma/policy.json`); entonces ninguna de las dos vías lo
activa. No hay variable de entorno para activarlo. En Windows la política aún no
se lee del registro (falta la plantilla ADMX).

El prestador debe dar de alta a GrxFirma como cliente OAuth público y facilitar un
identificador de cliente. No hay secreto de cliente: la autorización usa PKCE.

```sh
# Ver los certificados remotos de la cuenta
grxfirma -csc-activar -csc-url https://firma.prestador.example \
  -csc-client-id <id> -csc-listar-credenciales

# Firmar un PDF con uno de ellos
grxfirma -csc-activar -csc-url https://firma.prestador.example \
  -csc-client-id <id> -csc-credencial <credencial> \
  -entrada doc.pdf -salida doc_firmado.pdf -formato pades
```

La CLI muestra un enlace y abre el navegador del sistema para que la persona se
identifique ante el prestador. El navegador vuelve a `http://127.0.0.1` en un
puerto elegido al azar, donde GrxFirma recoge el código de autorización. Si la
credencial pide PIN o código de un solo uso (OTP), se piden por la entrada
estándar (sin eco en una terminal). Mientras dura la orden, la credencial remota
es el único certificado disponible, para que no se firme por error con otro.

## Seguridad

- Solo `https`, con la validación TLS del sistema (TLS 1.2 o superior). Se
  rechaza `http` en cualquier dirección, también la del servidor OAuth que
  anuncia el servicio.
- No se sigue ninguna redirección HTTP.
- Cada petición tiene un tiempo máximo de 30 segundos y la espera del navegador,
  de 5 minutos. Las respuestas se cortan a 1 MiB.
- El `state` OAuth es aleatorio y se compara en tiempo constante. Si no
  coincide, la autorización se descarta. El receptor local solo atiende
  `GET /callback` con el `Host` esperado y responde con cabeceras que impiden
  caché y referer.
- Los tokens, el SAD, el PIN y el OTP solo están en memoria. Se borran al
  terminar y el token de servicio se revoca. Go no permite borrar todas las
  copias que crean `net/http` y `encoding/json`, así que el borrado es lo mejor
  que se puede hacer, no una garantía.
- Ningún error ni registro incluye tokens, PIN, OTP ni SAD. Los datos que llegan
  del servidor se sanean antes de mostrarlos en la terminal.

## Límites

- Prototipo: no se ha probado con prestadores reales. Los nombres de algunos
  campos varían entre la versión 2.0 y las posteriores de la especificación; el
  cliente admite las dos formas conocidas, pero puede haber diferencias.
- Cada firma pide su propia autorización. Con SCAL2 o con PIN/OTP, una firma por
  lotes pide el dato una vez por documento. Falta autorizar varios resúmenes de
  una vez.
- Modos de autorización: `implicit`, `explicit` (PIN y OTP, también OTP enviado
  por el prestador) y `oauth2code`. No hay autenticación `basic` ni descubrimiento
  OAuth por `oauth2Issuer`, ni paginación del listado (máximo 100 credenciales).
- Algoritmos: RSA PKCS#1 v1.5, RSA-PSS y ECDSA con SHA-256, SHA-384 o SHA-512.
- Solo la CLI. No hay interfaz gráfica ni integración con `afirma://`.
- No se renueva el token: si caduca durante una orden larga, hay que repetirla.

## Pruebas

- `internal/adapters/outbound/common/csc`: flujo completo contra el simulador
  (`internal/testsupport/csctest`): descubrimiento, PKCE, listado, autorización
  SCAL1/SCAL2 en los tres modos, signHash con RSA, RSA-PSS y ECDSA. También los
  casos de error: `state` distinto, redirección a otro host, respuesta enorme,
  `http` plano, TLS no confiable, servicio sin OAuth y firma que no corresponde
  al certificado.
- `internal/adapters/outbound/desktop/signer`: el motor firma PAdES, CAdES y
  XAdES con la clave remota y el verificador del propio motor las da por válidas.
- `cmd/grxfirma`: opciones, activación y prohibición por política, listado y una
  firma CAdES completa con la CLI real, con PIN y OTP por stdin.

## Qué falta para usarlo en producción

Un acuerdo con al menos un prestador cualificado (identificador de cliente y
condiciones) o la publicación del servicio de firma remota de la cartera española,
pruebas con ese servicio, una revisión de seguridad de la integración y la decisión
del responsable sobre con qué prestadores abrirlo.
