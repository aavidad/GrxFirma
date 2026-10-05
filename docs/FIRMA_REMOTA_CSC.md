<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Firma remota CSC (prototipo)

**Estado:** prototipo en el motor, la línea de órdenes y las interfaces de escritorio
(WinUI y Qt), desactivado por defecto.
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

Decide primero la política de la organización:

- `"firma_remota_csc": true` en `policy.json` (`/etc/grxfirma/policy.json`) o,
  en Windows, el valor `firma_remota_csc` = `1` (`REG_DWORD`) en
  `HKLM\SOFTWARE\Policies\GrxFirma` la permite;
- `false` o `0` la prohíbe, diga lo que diga el usuario.

Si la política no fija el valor, la activa `"firma_remota_csc": true` en el
`config.json` del usuario. No hay opción de la línea de órdenes ni variable de
entorno que la active. La plantilla ADMX incluye las dos directivas
(`FirmaRemotaCSC` y `FirmaRemotaCSCOAuth`); véase `POLITICA_MAQUINA.md`.

El prestador debe dar de alta a GrxFirma como cliente OAuth público y facilitar un
identificador de cliente. No hay secreto de cliente: la autorización usa PKCE.

Por seguridad, el servidor OAuth que anuncia el servicio debe estar en el mismo
host y puerto que el servicio de firma. Si `/info` anuncia `oauth2Issuer`
(CSC 2.1), GrxFirma lee sus metadatos (RFC 8414, en
`/.well-known/oauth-authorization-server`) y usa los extremos que publican; el
emisor declarado debe coincidir con el anunciado, los métodos PKCE publicados
deben incluir S256 y el emisor y cada extremo (autorización, token y revocación)
deben cumplir la misma regla de host. Si un prestador los separa, se autoriza
el par en `firma_remota_csc_oauth_permitidos`, con una entrada
`servicio=autorizacion` por prestador (por ejemplo
`"firma.prestador.es=auth.prestador.es:8443"`). La lista de la política
sustituye a la del usuario y una entrada mal escrita impide usar la firma remota.

```sh
# Ver los certificados remotos de la cuenta
grxfirma -csc-url https://firma.prestador.example \
  -csc-client-id <id> -csc-listar-credenciales

# Firmar un PDF con uno de ellos
grxfirma -csc-url https://firma.prestador.example \
  -csc-client-id <id> -csc-credencial <credencial> \
  -entrada doc.pdf -salida doc_firmado.pdf -formato pades
```

La CLI escribe el host del servicio y el del servidor de autorización y abre el
navegador del sistema para que la persona se identifique ante el prestador. No
escribe el enlace, porque lleva el `state` de la petición; si no hay navegador en
el equipo, la orden falla. El navegador vuelve a `http://127.0.0.1` en un puerto
elegido al azar, donde GrxFirma recoge el código de autorización. Si la
credencial pide PIN o código de un solo uso (OTP), se piden por la entrada
estándar (sin eco en una terminal). Mientras dura la orden, la credencial remota
es el único certificado disponible, para que no se firme por error con otro.

Con `-lote <carpeta|manifiesto>`, si el prestador admite varias firmas por
autorización (`multisign` en `credentials/info`), GrxFirma calcula a la vez los
resúmenes de un grupo de documentos, pide el PIN y el OTP una vez y los autoriza
juntos. Los grupos tienen como mucho el tamaño que anuncia el prestador (y 128
documentos); si el lote es mayor, se pide el dato una vez por grupo. Si el
prestador no admite varias firmas, cada documento pide la suya, como antes.

## Interfaces de escritorio

WinUI y Qt usan la misma sesión, que vive en el motor
(`internal/adapters/outbound/desktop/cscremota`) y se maneja por el IPC:

- `csc_status` dice si la firma remota está permitida (se vuelve a leer la
  política en cada operación) y devuelve la dirección y el client_id guardados.
  Si la política de la organización la prohíbe, añade `prohibitedByPolicy`: el
  botón «Firma remota» se muestra y el cuadro solo explica que la política no
  la permite. Si solo está desactivada en `config.json`, el botón no aparece.
  La CLI distingue igual los dos casos.
- `csc_configure` valida la dirección y el client_id con las reglas de la CLI,
  consulta `/info` sin abrir el navegador y devuelve el host del servicio y el
  del servidor de autorización (los nombres internacionalizados, en punycode).
  La persona los ve antes de pulsar «Conectar». La dirección y el client_id se
  guardan en `firma_remota_csc.json`, sin secretos.
- `csc_connect` abre el navegador del sistema desde el motor, espera la vuelta
  (como mucho cuatro minutos) y carga los certificados remotos. Aparecen en la
  lista de certificados junto a los locales, con la marca «Remoto».
- `csc_disconnect` revoca los tokens y olvida los certificados remotos.
- `csc_send_otp` pide al prestador el código de un solo uso cuando la
  credencial lo envía en línea; la interfaz lo ofrece en el cuadro del PIN.

Al firmar con un certificado remoto que pide PIN u OTP, la interfaz los pide en
campos de contraseña (`PasswordBox` en WinUI, `TextField` con `echoMode`
`Password` en Qt). Viajan en Base64 dentro de la petición de firma (`remotePin`,
`remoteOtp`), el motor los decodifica en `[]byte`, solo los acepta para un
certificado remoto y los borra al terminar. Las acciones de firma pasan a ser
sensibles: el motor borra sus parámetros y las interfaces no las repiten tras
una reconexión. Los tokens nunca salen del motor.

En `sign_batch`, el motor usa la autorización conjunta si el prestador admite
varias firmas: un solo PIN u OTP para todo el lote. `csc_connect` devuelve
`multiSign` en cada credencial y `certificates`, `remoteMultiSign`. Un lote con
un certificado que pide OTP solo se admite si cabe en una autorización: sin
`multisign` se rechaza con `csc_otp_lote`, y si tiene más documentos de los que
admite el prestador, con `csc_otp_lote_excede` antes de firmar nada. Las
interfaces lo comprueban antes de pedir el código. La multifirma por lotes no
usa la autorización conjunta y sigue rechazando el OTP.

## Seguridad

- Solo `https`, con la validación TLS del sistema (TLS 1.2 o superior). Se
  rechaza `http` en cualquier dirección, también la del servidor OAuth que
  anuncia el servicio, y ese servidor debe estar en el host del servicio o en
  un par autorizado.
- No se sigue ninguna redirección HTTP. Si el cliente HTTP de base no se puede
  endurecer (no es un `http.Transport`), no se conecta.
- Cada petición tiene un tiempo máximo de 30 segundos y la espera del navegador,
  de 5 minutos. Las respuestas se cortan a 1 MiB y la vida de los tokens se
  acota a 24 horas.
- El `state` OAuth es aleatorio y se compara en tiempo constante. Una petición
  con otro `state`, otra ruta, otro método u otro `Host` recibe un error y no
  cuenta: el receptor sigue esperando la buena hasta el tiempo máximo, de modo
  que otra web u otro proceso local no puede ni colar un código ni cortar la
  espera. Las respuestas impiden caché y referer.
- Tokens, SAD, PIN y OTP solo están en memoria, en `[]byte`. Los cuerpos que los
  llevan se componen como bytes y se borran tras enviarse. Al terminar se
  revocan el token de servicio y el de credencial (modo `oauth2code`). Quedan
  copias que no se pueden borrar: la cabecera `Authorization` es un `string` en
  `net/http` y la CLI lee el PIN como `string`. El borrado reduce la exposición,
  no la elimina.
- El token de servicio se renueva con `refresh_token` si el servidor lo da
  (también tras un 401 en las consultas que se pueden repetir) y el
  `refresh_token` vive en la misma memoria protegida y se revoca al cerrar. Si
  no hay `refresh_token` o el servidor lo rechaza, la operación falla con
  `sesion_caducada` y hay que volver a conectar. La autorización de la
  credencial y la firma no se repiten nunca tras un 401, porque llevarían otra
  vez el PIN o un OTP ya gastado.
- En un lote, el SAD solo autoriza los resúmenes enviados en
  `credentials/authorize` (`numSignatures` igual a su número), se usa en una
  sola llamada a `signatures/signHash` con esos mismos resúmenes y se borra al
  terminar. Si el motor pidiera una segunda firma del mismo documento, falla con
  `lote_repetido` en lugar de pedir otra autorización. Para que eso no ocurra
  con PAdES, la reserva de espacio de la firma en el PDF se calcula por el
  tamaño de la clave del firmante más un margen para los atributos firmados.
- El listado de credenciales recorre las páginas (`pageToken`), con un máximo
  de 1000 credenciales y 20 páginas; un token de página repetido o mal formado
  invalida el listado. Las interfaces muestran hasta 200 certificados remotos.
- La firma recibida se comprueba con la clave pública del certificado (en
  RSA-PSS, con la sal anunciada). La cadena que envía el servicio solo se
  incrusta si cada certificado está firmado por el siguiente.
- Ningún error ni registro incluye tokens, PIN, OTP ni SAD. Los datos que llegan
  del servidor se sanean antes de mostrarlos en la terminal.

## Límites

- Prototipo: no se ha probado con prestadores reales. Los nombres de algunos
  campos varían entre la versión 2.0 y las posteriores de la especificación; el
  cliente admite las dos formas conocidas, pero puede haber diferencias.
- Un lote solo se autoriza de una vez si el prestador anuncia `multisign`. Si
  no, cada documento pide su autorización (y su PIN) y el lote con OTP no se
  admite en las interfaces. Los documentos de un grupo se firman a la vez
  porque la autorización necesita todos sus resúmenes; con `StopOnError`, los
  documentos del grupo posteriores al primer fallo ya están firmados, pero no
  se devuelven.
- Todos los resúmenes de un grupo deben usar el mismo algoritmo; si no, el
  documento discordante falla con `lote_mixto`.
- Modos de autorización: `implicit`, `explicit` (PIN y OTP, también OTP enviado
  por el prestador) y `oauth2code`. No hay autenticación `basic`.
- Algoritmos: RSA PKCS#1 v1.5, RSA-PSS y ECDSA con SHA-256, SHA-384 o SHA-512.
- No hay integración con `afirma://` ni con el servicio REST. Las interfaces
  gráficas no se han probado aún en Windows con un prestador real.
- Sin `refresh_token`, un token que caduca durante una orden larga obliga a
  repetirla o a volver a conectar.

## Pruebas

- `internal/adapters/outbound/common/csc`: flujo completo contra el simulador
  (`internal/testsupport/csctest`): descubrimiento, PKCE, listado, autorización
  SCAL1/SCAL2 en los tres modos, signHash con RSA, RSA-PSS y ECDSA. Lotes con
  `multisign` (un PIN y un OTP, documentos que fallan antes de firmar, firma
  repetida, algoritmos mezclados, cierre y cancelación), capacidad según
  `multisign`, listado paginado (más de 100, cíclico y demasiado largo),
  renovación por `refresh_token` (con y sin rotación, tras un 401, rechazada y
  sin `refresh_token`) y metadatos de `oauth2Issuer` (emisor distinto, sin S256,
  `http` y extremos en otro host). También los
  casos de error: `state` distinto (no se acepta y la espera caduca), servidor
  OAuth en otro host, redirección a otro host, respuesta enorme, `http` plano,
  TLS no confiable, servicio sin OAuth, firma que no corresponde al
  certificado, sal PSS distinta y cadena que no encadena.
- `internal/adapters/outbound/desktop/signer`: el motor firma PAdES, CAdES y
  XAdES con la clave remota y el verificador del propio motor las da por válidas.
- `internal/adapters/outbound/common/config`: activación por política (también
  la de máquina de Windows, con un lector simulado) y por `config.json`.
- `cmd/grxfirma`: opciones, activación y prohibición por política, pares OAuth,
  listado sin escribir el `state`, una firma CAdES completa con la CLI real,
  con PIN y OTP por stdin, y un lote PAdES de una carpeta con un solo PIN y OTP
  y una sola autorización.
- `internal/application`: firma por grupos con una clave que autoriza lotes
  (orden de resultados, fallos, `StopOnError` y grupo final suelto).
- `third_party/pdfsign`: con claves RSA-4096 y P-521 el PDF se firma una sola
  vez.

- `internal/adapters/outbound/desktop/cscremota` e IPC (`csc_remota_test.go`):
  política retirada a mitad de sesión, parámetros estrictos, hosts mostrados,
  PIN y OTP por petición, OTP de un solo uso, secretos para un certificado
  local rechazados, una firma CAdES completa por el IPC y lotes PAdES por
  `sign_batch` (un OTP y una autorización, lote que no cabe, sin `multisign`,
  agrupación por `multisign`).
- Interfaces: contratos Python de Qt y WinUI y pruebas de `WinUI.Core` (sin
  compilar aquí: no hay SDK de .NET en este equipo).

## Qué falta para usarlo en producción

Un acuerdo con al menos un prestador cualificado (identificador de cliente y
condiciones) o la publicación del servicio de firma remota de la cartera española,
pruebas con ese servicio, una revisión de seguridad de la integración y la decisión
del responsable sobre con qué prestadores abrirlo.
