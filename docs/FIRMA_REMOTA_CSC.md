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
host y puerto que el servicio de firma. Si un prestador los separa, se autoriza
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

## Interfaces de escritorio

WinUI y Qt usan la misma sesión, que vive en el motor
(`internal/adapters/outbound/desktop/cscremota`) y se maneja por el IPC:

- `csc_status` dice si la firma remota está permitida (se vuelve a leer la
  política en cada operación) y devuelve la dirección y el client_id guardados.
  El botón «Firma remota» solo aparece si está permitida.
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
una reconexión. Los tokens nunca salen del motor. Un lote con un certificado
que pide OTP se rechaza, porque el código solo vale para una firma.

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
- La firma recibida se comprueba con la clave pública del certificado (en
  RSA-PSS, con la sal anunciada). La cadena que envía el servicio solo se
  incrusta si cada certificado está firmado por el siguiente.
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
- No hay integración con `afirma://` ni con el servicio REST. Las interfaces
  gráficas no se han probado aún en Windows con un prestador real.
- No se renueva el token: si caduca durante una orden larga, hay que repetirla.

## Pruebas

- `internal/adapters/outbound/common/csc`: flujo completo contra el simulador
  (`internal/testsupport/csctest`): descubrimiento, PKCE, listado, autorización
  SCAL1/SCAL2 en los tres modos, signHash con RSA, RSA-PSS y ECDSA. También los
  casos de error: `state` distinto (no se acepta y la espera caduca), servidor
  OAuth en otro host, redirección a otro host, respuesta enorme, `http` plano,
  TLS no confiable, servicio sin OAuth, firma que no corresponde al
  certificado, sal PSS distinta y cadena que no encadena.
- `internal/adapters/outbound/desktop/signer`: el motor firma PAdES, CAdES y
  XAdES con la clave remota y el verificador del propio motor las da por válidas.
- `internal/adapters/outbound/common/config`: activación por política (también
  la de máquina de Windows, con un lector simulado) y por `config.json`.
- `cmd/grxfirma`: opciones, activación y prohibición por política, pares OAuth,
  listado sin escribir el `state` y una firma CAdES completa con la CLI real,
  con PIN y OTP por stdin.

- `internal/adapters/outbound/desktop/cscremota` e IPC (`csc_remota_test.go`):
  política retirada a mitad de sesión, parámetros estrictos, hosts mostrados,
  PIN y OTP por petición, OTP de un solo uso, secretos para un certificado
  local rechazados y una firma CAdES completa por el IPC.
- Interfaces: contratos Python de Qt y WinUI y pruebas de `WinUI.Core` (sin
  compilar aquí: no hay SDK de .NET en este equipo).

## Qué falta para usarlo en producción

Un acuerdo con al menos un prestador cualificado (identificador de cliente y
condiciones) o la publicación del servicio de firma remota de la cartera española,
pruebas con ese servicio, una revisión de seguridad de la integración y la decisión
del responsable sobre con qué prestadores abrirlo.
