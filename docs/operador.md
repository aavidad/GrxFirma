<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Guía de operador de GrxFirma

## Objetivo

Este documento resume cómo instalar, operar, diagnosticar y transicionar de AutoFirma V1 a GrxFirma en entornos reales. Está orientado a soporte, despliegue de escritorio y validación funcional antes del cambio de versión.

## Identidad reforzada v1

La identidad reforzada (`identidad-reforzada/v1`) es una función experimental y
no está activada en los paquetes publicados. Los endpoints legacy
`/auth/challenge` y `/auth/verify` solo protegen el REST local y no acreditan
una sesión, rol o permiso de la aplicación integradora.

## Requisitos por plataforma

### Linux

Mínimos recomendados:

- `libnss3`
- `opensc`
- `pcscd` si se usan tarjetas criptográficas
- `certutil` para escenarios NSS
- herramientas XDG (`xdg-open`, `xdg-mime`, `update-desktop-database`) para `afirma://`

### Windows

Mínimos esperados:

- almacén de certificados del sistema operativo
- permisos de usuario para registrar el protocolo `afirma://`
- navegador compatible con Native Messaging

### macOS

Mínimos esperados:

- acceso al Keychain del usuario
- registro del esquema `afirma://`
- navegador compatible con Native Messaging

### Android e iPhone

- Android tiene una aplicación nativa en Kotlin que usa el núcleo Go mediante
  `gomobile bind`.
- El código de iPhone/iPad está en el repositorio, pero no se ha validado.

Uso básico en Android:

1. Pulse **Seleccionar documento** y elija un fichero mediante el selector de
   Android.
2. Use un certificado disponible o pulse **Seleccionar archivo PKCS#12**,
   introduzca su contraseña e impórtelo. La identidad solo dura esa sesión.
3. Elija PAdES para PDF, CAdES para una firma separada o XAdES para XML y pulse
   **Firmar documento**.
4. Guarde el resultado en la ubicación que ofrezca Android.
5. Para verificar PAdES o XAdES, seleccione la firma y deje vacío el original.
   Para CAdES separada, seleccione además el fichero que se firmó en
   **Seleccionar original para firma separada**.

La validación distingue integridad y confianza. Un certificado sintético o
autofirmado puede tener una firma criptográficamente íntegra y, aun así, no
estar respaldado por una autoridad de confianza. No lo trate como certificado
de producción.

## Configuración

### Fichero de usuario

Ruta:

```text
~/.config/grxfirma/config.json
```

Ejemplo:

```json
{
  "websocket_habilitado": false,
  "websocket_permitido": true,
  "rest_habilitado": false,
  "tofu_habilitado": true,
  "directorio_p12": "~/.config/grxfirma/pkcs12",
  "dominios_de_confianza": [],
  "nivel_log": "info",
  "timeout_operacion_segundos": 30,
  "max_tamano_documento_bytes": 52428800
}
```

`websocket_habilitado: false` deja apagado el servicio residente, pero permite
que una sede abra un canal temporal con `afirma://`. Para desactivar también
ese canal, use `websocket_permitido: false`. El valor por defecto es `true`
para mantener la compatibilidad; una política de organización puede imponer
`false` aunque el usuario habilite WebSocket. Las sedes que requieran
AutoScript 1.9 por WebSocket dejarán de funcionar con el corte total.

### Política de organización

Ruta en Linux y macOS:

```text
/etc/grxfirma/policy.json
```

En Windows la política se lee **exclusivamente** de
`HKLM\SOFTWARE\Policies\GrxFirma` (desplegable por GPO); no se usa
`C:\etc`. Se aplica con mayor precedencia que el fichero de usuario. Valores y
ejemplos: [POLITICA_MAQUINA.md](POLITICA_MAQUINA.md).

### Allowlist del sistema y TOFU

Ruta de allowlist del sistema usada por `TrustPolicy`:

```text
/etc/grxfirma/allowed-domains.json
```

Regla operativa recomendada:

- en desktop no gestionado, `TOFU` puede quedar habilitado como fallback;
- en despliegues corporativos o de venta publica, el baseline recomendado es:
  - poblar `/etc/grxfirma/allowed-domains.json` con los dominios admitidos;
  - opcionalmente, declarar dominios adicionales en `/etc/grxfirma/policy.json`
    mediante `dominios_de_confianza`;
  - fijar `"tofu_habilitado": false` en `/etc/grxfirma/policy.json`.

### Confianza de usuario y TOFU

Rutas persistentes del perfil de usuario:

```text
~/.config/grxfirma/trusted-domains.json
~/.config/grxfirma/trusted-domains.seeded
```

- `trusted-domains.json` guarda decisiones persistidas del usuario sobre orígenes.
- `trusted-domains.seeded` marca que la semilla inicial de dominios públicos ya fue aplicada.
- una reinstalación normal del producto no borra esos ficheros.
- si `tofu_habilitado=false` por política, el motor pasa a modo allowlist:
  - deja de sembrar dominios públicos por defecto en perfiles nuevos;
  - ignora decisiones `allowed` persistidas del usuario;
  - sigue respetando la allowlist administrativa (`allowed-domains.json` y `dominios_de_confianza` de `policy.json`);
  - rechaza sin diálogo TOFU cualquier origen fuera de esa allowlist.

### Variables de entorno

Configuración base:

- `GRXFIRMA_WEBSOCKET_HABILITADO`
- `GRXFIRMA_WEBSOCKET_PERMITIDO`
- `GRXFIRMA_REST_HABILITADO`
- `GRXFIRMA_DIRECTORIO_P12`
- `GRXFIRMA_NIVEL_LOG`
- `GRXFIRMA_TIMEOUT_OPERACION_SEGUNDOS`
- `GRXFIRMA_MAX_TAMANO_DOCUMENTO_BYTES`

Logging:

- `GRXFIRMA_LOG_LEVEL`
- `GRXFIRMA_ENV`

Los paquetes oficiales se compilan con la etiqueta `production`. En ellos,
`GRXFIRMA_DEBUG` y `GRXFIRMA_LOG_LEVEL=DEBUG` no pueden elevar el nivel por
encima de `INFO`, y no se instalan lanzadores de depuración. `DEBUG` queda
reservado para builds de desarrollo sin esa etiqueta y con datos sintéticos.

Native host:

- `GRXFIRMA_PKCS12_DIR`
- `GRXFIRMA_PKCS12_PASSWORD` (compatibilidad para lanzadores existentes)
- `GRXFIRMA_REST_TOKEN` (compatibilidad para automatización REST; también se acepta el nombre heredado `AUTOFIRMAV2_REST_TOKEN`)
- `GRXFIRMA_PROTECTION_SECRET_B64` (compatibilidad para automatización CLI)

Límites:

- `GRXFIRMA_HTTP_TIMEOUT_SEC`
- `GRXFIRMA_MAX_PAYLOAD_BYTES`
- `GRXFIRMA_MAX_BATCH_DOCS`
- `GRXFIRMA_MAX_SESSION_SEC`

### Avisos de nuevas versiones

Las interfaces de escritorio consultan, como máximo una vez por proceso, la
última Release estable publicada en:

```text
https://api.github.com/repos/aavidad/GrxFirma/releases/latest
```

La preferencia tipada `General.checkForUpdates` controla la consulta
automática y queda habilitada cuando todavía no existe en el perfil. El usuario
puede desactivarla en **Configuración > Avisar de nuevas versiones** y siempre
puede lanzar una consulta explícita desde **Acerca de > Comprobar
actualizaciones**. La utilidad CLI heredada solo consulta si el operador define
`GRXFIRMA_CHECK_UPDATES=1`.

La operación envía una petición `GET` con el agente
`GrxFirma-UpdateCheck`; no adjunta documentos, certificados, firmas,
preferencias ni identificadores del usuario. GitHub y la infraestructura de
red observan, como en cualquier conexión HTTPS, los metadatos técnicos
necesarios para atenderla. GrxFirma no descarga ni instala la Release.

Requisitos de red para el aviso y la apertura voluntaria del navegador:

- permitir HTTPS a `api.github.com:443`;
- permitir HTTPS a `github.com:443`;
- configurar el proxy del sistema o de la aplicación sin desactivar la
  validación TLS.

El proyecto debe publicar una Release estable accesible sin autenticación,
cuya etiqueta coincida con la versión del paquete. Un repositorio privado, una
Release inexistente o una publicación solo en borrador/pre-release provocan un
`404` o no producen aviso. No se debe «resolver» insertando un token personal
en el binario: publique la Release prevista o mantenga desactivada la consulta
automática hasta que exista el canal público.

Ante un fallo manual, compruebe conexión, proxy, DNS, reloj y acceso HTTPS y
repita. La firma local sigue disponible. Si hay una nueva versión, el usuario
debe revisar las notas, descargarla desde la página oficial y ejecutar
manualmente su instalador; la aplicación nunca ejecuta el contenido remoto.

Compatibilidad heredada, solo cuando el portal la exija y **solo mediante la
política de máquina** (las variables de entorno antiguas ya no tienen efecto;
véase [POLITICA_MAQUINA.md](POLITICA_MAQUINA.md)):

- `permitir_des_legacy`: habilita el formato de sesión DES de V1.9.
- `permitir_sha1_legacy`: habilita generación de firmas SHA-1.
- `permitir_rutas_directas_protocolo`: permite rutas absolutas entregadas
  por el portal en `load`, `save` y `signandsave`.

Las tres opciones están desactivadas por defecto. No deben activarse
globalmente sin una necesidad de interoperabilidad documentada. SHA-1 no es
necesario para verificar firmas antiguas; las rutas directas sustituyen la
selección local del usuario y amplían expresamente la confianza concedida al
portal.

### Proxy autenticado y almacén seguro

En la GUI Qt, `Configuración > Proxy` permite activar el modo manual e indicar
host, puerto y exclusiones. En modo IPC aparece además el bloque de
autenticación para crear, rotar o quitar la credencial:

- Linux usa Secret Service mediante `secret-tool`;
- macOS usa el Keychain del usuario mediante Security.framework;
- Windows usa DPAPI ligado al usuario.

`settings.json` solo conserva `proxySecretId` y `proxyRealm`; usuario y
contraseña quedan dentro del almacén del SO. `REST /settings` e IPC
`save_settings` rechazan credenciales en claro y tampoco pueden sustituir la
referencia opaca. Si el almacén está bloqueado, falta el secreto o la
configuración es ambigua, el transporte falla cerrado: no intenta una conexión
directa ni recupera variables de entorno como degradación.

La disponibilidad puede diagnosticarse desde la GUI, `/signer` o:

```text
GET /settings/proxy/secret-store/status
```

Antes de desplegar, conviene probar alta, rotación, uso y borrado con una cuenta de
proxy de ensayo en cada SO y comprobar que ni settings ni logs contienen
usuario/password.

## Certificados y credenciales

### PKCS#12 por defecto

Directorio por defecto:

```text
~/.config/grxfirma/pkcs12
```

### Importación soportada

- `.p12`
- `.pfx`
- combinación `cert.pem` + `key.pem`

### Observaciones operativas

- el importador común está unificado detrás del adaptador `pkcs12importer`
- si la vía Go no reconoce un P12/PFX antiguo, el adaptador prueba OpenSSL y,
  solo si OpenSSL 3 rechaza el cifrado de transporte histórico, repite la
  lectura con `-legacy`; esto no cambia la política criptográfica de la firma
  ni convierte un certificado caducado en válido
- la CLI lee contraseñas con `-contrasena-stdin` o
  `-contrasena-p12-stdin`; desactiva el eco si stdin es una terminal
- `grxfirma-gui` usa `--p12-password-stdin`
- el lanzador `grxfirma -desktop -frontend qt` transfiere la contraseña al
  backend `grxfirma-gui` mediante un pipe anónimo; no la reenvía al frontend
  Qt en argumentos ni entorno
- en Native Messaging se usa `GRXFIRMA_PKCS12_PASSWORD`
- las variables con secretos son compatibilidad para procesos gestionados, no
  deben persistirse en scripts ni ficheros de entorno
- las opciones heredadas que incluían el valor en argv se rechazan por
  CWE-214
- cuando REST genera su Bearer, publica únicamente la ruta de una configuración
  efímera privada para `curl --config`; el fichero usa `0600`/DACL protegida y
  se elimina al terminar el servidor

## Logging, auditoría y métricas

### Logging estructurado

Se usa `log/slog` con estas reglas:

- `GRXFIRMA_LOG_LEVEL=DEBUG|INFO|WARN|ERROR`
- `GRXFIRMA_ENV=production|prod|produccion` emite JSON
- en otros entornos se emite texto legible

Binarios ya integrados:

- `cmd/grxfirma`
- `cmd/nativehost`

### Auditoría

Ruta por defecto:

```text
~/.local/share/grxfirma/audit.jsonl
```

Qué queda registrado:

- operación
- origen saneado
- huella del certificado
- nombre del documento
- hash SHA-256 del documento
- formato
- éxito o error

Qué no debe aparecer:

- documento en claro
- payloads completos

La auditoría local conserva como máximo el fichero activo y una rotación de
10 MiB cada uno, y elimina ambos cuando superan 90 días antes de registrar una
evidencia nueva. Las incidencias y el log Qt se conservan como máximo 30 días;
además hay un máximo de 50 incidencias y una única rotación Qt de 2 MiB.
Consulte [POLITICA_RETENCION_Y_DIAGNOSTICO.md](POLITICA_RETENCION_Y_DIAGNOSTICO.md)
para el alcance, el borrado seguro y las obligaciones del despliegue.
- valor-pruebas o contraseñas

### Métricas

Ruta:

```text
/tmp/grxfirma-metrics.json
```

Rotación:

```text
/tmp/grxfirma-metrics.json.1
```

Métricas ya emitidas:

- `sign_total`
- `sign_duration_ms`
- `certificate_source`

## Consola web local

La consola REST local (`/`) ya muestra dos resúmenes visibles útiles para operación:

- estado TLS local:
  muestra si el certificado local existe, cuántos artefactos, certificados y
  claves TLS hay y recuerda que la confianza efectiva del sistema depende de
  la plataforma;
- diagnóstico local:
  muestra cuántos certificados se detectan, cuántos son utilizables para firmar
  y el estado resumido del almacén TLS local.

El contrato `tlsStore` de REST e IPC publica únicamente estados y recuentos:
no incluye la ruta del perfil ni nombres de certificados, claves o artefactos.
Por tanto, el resumen copiable para soporte no debe usarse para intentar
localizar ficheros concretos; esa inspección requiere acceso local autorizado.
El recuento de certificados utilizables exige además vigencia y una clave
resoluble, por lo que puede ser menor que el total del catálogo.

`Instalar confianza TLS local` gestiona únicamente la CA local inventariada de
GrxFirma. `Vaciar almacén TLS local` valida el directorio, retira primero esa
confianza del almacén del sistema y borra después solo sus artefactos conocidos.
No elimina certificados `.pem`/`.crt` ajenos; si no puede retirar la confianza,
conserva el material local para permitir un reintento seguro.

Acciones sensibles expuestas en esa consola:

- `Instalar confianza TLS local`
- `Vaciar almacén TLS local`

Ambas piden confirmación previa en la interfaz web antes de ejecutar el endpoint correspondiente.
- `protocol_requests_total`

## Consola y firmador web local

La API REST local publica dos superficies web útiles para operación y soporte:

- `https://127.0.0.1:63118/`
- `https://127.0.0.1:63118/signer`

Estado operativo actual:

- la consola `/` permite firmar uno o varios ficheros;
- la consola `/` permite también seleccionar una carpeta completa para tratarla como lote;
- si se seleccionan varios o una carpeta, usa `/sign-batch` automáticamente;
- el firmador `/signer` ofrece la misma capacidad de lote, con una UX más orientada a sello visible y metadatos `PAdES`, y admite tanto ficheros sueltos como carpetas completas;
- el firmador `/signer` además ejecuta una verificación automática del fichero recién firmado y muestra estado, razón, firmantes y detalles;
- tanto `/` como `/signer` ejecutan también verificación automática por elemento cuando la firma múltiple devuelve el contenido firmado en memoria;
- la consola `/` muestra además, antes de verificar manualmente, resúmenes visibles de validación:
  - tipo probable del documento firmado;
  - tipo probable del original de referencia si se aporta;
  - pareja prevista de validación cuando detecta firmado/original compatibles;
  - compatibilidad visible de validación cuando puede inferirla con ambos artefactos;
  - stack de firma probable cuando puede estimarlo;
  - y un plan visible de validación con:
  - documento firmado efectivo (`archivo` o `ruta local`);
  - original de referencia si se aporta;
  - y modo efectivo (`solo documento firmado` o `con original de referencia`);
- la consola `/` también expone metadatos `PAdES` básicos de motivo, ubicación y contacto;
- la consola `/` también expone utilidades de huella:
  - `POST /hash`
  - `POST /hash/check`
- la misma REST local expone además protección de ficheros:
  - `GET /protection/recipients`
  - `GET /protection/recipient/export?id=<id_alto>`
  - `POST /protection/recipient/import`
  - `POST /protect`
  - `POST /unprotect`
- la consola `/` ya permite también:
  - cargar destinatarios locales de protección;
  - ver el estado visible del destinatario seleccionado para proteger;
  - exportar un destinatario fuerte local o importado para compartirlo con otro equipo;
  - importar un destinatario fuerte previamente exportado;
  - proteger un fichero subido o una ruta local;
  - desproteger un fichero `.afp` subido o una ruta local;
  - descargar el contenido protegido o desprotegido devuelto por la API local;
- tanto `/` como `/signer` permiten añadir un `QR` opcional dentro del sello visible cuando no se usa imagen personalizada;
- tanto `/` como `/signer` muestran además una verificación automática del fichero recién firmado cuando la firma devuelve el contenido en memoria;
- ambos permiten sello visible PAdES en:
  - una página concreta;
  - rangos como `1,3-5`;
  - todas las páginas con `all`.

Observaciones de soporte:

- el sello visible solo se aplica a `PAdES` sobre PDF;
- en `/signer` la posición y tamaño del sello pueden ajustarse visualmente;
- en `/signer` también pueden definirse metadatos de firma como motivo, ubicación, contacto y QR del sello;
- en `/signer` se muestra además si esos metadatos configurados aplicarán o no
  al formato efectivo: fuera de `PAdES` quedan visibles como no aplicables;
- la GUI Qt6 y la CLI también permiten esos metadatos `PAdES` y el QR del sello;
- la extensión del navegador abre el firmador local `/signer`, no una web heredada aparte;
- si una instancia REST antigua sigue viva, conviene reciclarla para que la web publicada coincida con el binario recién instalado.

## GUI Qt6 de escritorio

Estado operativo actual:

- la GUI Qt6 firma por IPC tanto documento simple como lote;
- la misma pantalla `Firmar` permite seleccionar varios ficheros sueltos para tratarlos como lote;
- también permite seleccionar una carpeta completa y, opcionalmente, una carpeta de salida para el lote;
- el resultado del lote queda visible por fichero dentro de la propia interfaz, con apertura y validación directa de cada salida correcta;
- la GUI Qt6 verifica automáticamente el fichero recién firmado, tanto en firma simple como en los elementos correctos de un lote;
- la previsualización del sello visible se mantiene para firma simple PDF y no se fuerza artificialmente en modo lote;
- el bloque de sello visible incluye presets rápidos (`Compacto`, `Institucional`, `Solo logo`, `Logo + datos + QR`) y restauración a valores por defecto;
- el panel de certificados permite búsqueda local por titular, emisor, serie o huella;
- la GUI Qt6 distingue entre certificado predeterminado y último certificado recordado;
- la persistencia de preferencias en Qt6 es explícita: `Guardar preferencias`, `Descartar cambios` y aviso al cerrar si hay cambios sin guardar.

Observaciones de soporte:

- en modo lote, si no se indica carpeta de salida, cada resultado se guarda junto al documento original;
- la selección de lote en Qt6 es no recursiva cuando se elige una carpeta: procesa los ficheros regulares contenidos directamente en ella;
- el caso de uso reutilizado es el mismo `ProcessBatchUseCase` del núcleo, no una implementación paralela específica de la UI.
- si se instala la GUI Qt/QML local en Linux, el flujo operativo vigente es:
  - base de usuario: `make install-user`
  - frontend Qt/QML: `packaging/linux/install-user.sh --with-qt`
- esa instalación deja:
  - `~/.local/bin/grxfirma-gui`
  - `~/.local/bin/grxfirma-gui-qml`
  - `~/.local/lib/grxfirma/gui-qml/{qml,assets}`
- si la GUI devuelve `accion no soportada sign_batch` o `accion no soportada sign_multicosign`, la causa operativa más probable no es el caso de uso sino un backend IPC viejo todavía escuchando en el socket por defecto;
- en ese caso, conviene reciclar la instalación local y relanzar la GUI para que frontend y backend IPC usen la misma build.

## GUI WinUI de escritorio

Estado operativo en el árbol actual:

- firma, cofirma y contrafirma simples usan el mismo backend Go por IPC;
- la cofirma múltiple guiada usa `sign_multicosign` para `PAdES`, `ODF` y
  `OOXML`, exige un certificado adicional distinto y no se ofrece en modo
  contrafirma;
- la protección y desprotección incluyen `EncryptedData` con una clave
  AES-256 aleatoria expresada en Base64 canónico;
- la firma por lotes permite combinar varios ficheros con una carpeta de
  primer nivel, elegir una carpeta de salida y revisar el estado confirmado de
  cada documento, incluidos resultados parciales;
- el lote admite como máximo 128 documentos, 100 MB por documento y 256 MB
  en conjunto; la carpeta no se recorre de forma recursiva;
- al cancelar, los documentos pendientes se marcan como cancelados y solo se
  conservan como correctas las salidas que el backend ya haya confirmado;
- para PAdES, la previsualización representa la página PDF real y superpone la
  zona exacta del sello; se puede mover arrastrándola y redimensionar desde el
  tirador inferior derecho, sin permitir que salga de la página;
- en Windows instalado, WinUI renderiza esa página con
  `Windows.Data.Pdf.PdfDocument`; no requiere `pdfinfo`, `pdftoppm` ni una
  instalación separada de Poppler. La vista sigue siendo local y aplica
  límites de 100 MiB, tamaño de imagen y puntos de reanálisis;
- los porcentajes X/Y/ancho/alto siguen disponibles para teclado, lector de
  pantalla y ajuste preciso; X parte del borde izquierdo e Y del inferior;
- si cambia el documento, la selección de páginas o la identidad física del
  fichero desde la previsualización, WinUI exige cargarla de nuevo antes de
  firmar;
- Certificados permite elegir P12/PFX y usarlo solo durante la sesión o
  importarlo en uno de los destinos persistentes que publica el backend;
- la credencial se limita a 2 MiB y la contraseña a 4 KiB; el diálogo Win32
  evita `string` administrados y ambos valores viajan como buffers
  `credentialB64`/`passwordB64` que se sobrescriben tras la operación;
- si Windows no puede crear el diálogo Win32 propio, se usa CredUI como
  respaldo de solo contraseña, siempre visible y no persistente; esta ruta del
  sistema admite como máximo 256 caracteres y entrega el secreto directamente
  a un buffer nativo borrable;
- una credencial temporal se puede retirar individualmente o limpiar junto con
  las demás, siempre tras una confirmación explícita;
- la clave se captura y confirma mediante diálogos Win32 nativos, se convierte
  a buffers borrables sin crear un `string` administrado y viaja por IPC en el
  campo binario `secretB64`;
- el secreto no se persiste ni aparece en logs; las copias controladas se
  sobrescriben al finalizar o cancelar.

Las acciones de servicio Windows no se anuncian desde el backend porque no
están implementadas en esa plataforma. No deben esperarse controles para
consultar, instalar, iniciar, detener o desinstalar un servicio desde WinUI.

## Protección local de ficheros

Estado actual:

- perfiles operativos:
  - `alto`: `ML-KEM-768 + X25519 + AES-256-GCM`
  - `compat`: `RSA-OAEP-SHA256 + AES-256-GCM`
- en `compat` ya se puede elegir:
  - sobre nativo `json` (`.afp`)
  - contenedor CMS DER estándar `cms` de tipo `EnvelopedData` (`.enveloped`)
  - contenedor CMS DER estándar `cms-encrypted` de tipo `EncryptedData` (`.encrypted.p7m`) con `secret_b64` transitorio de 32 bytes para AES-256-GCM
  - contenedor CMS DER estándar `signedandenvelopeddata` de tipo `SignedAndEnvelopedData` (`.signedenveloped.p7m`) mediante la operación separada `proteger-firmando`
- el perfil `alto` genera automáticamente una identidad local fuerte del usuario si todavía no existe
- los destinatarios se resuelven desde el catálogo local ya disponible
- en `compat` solo se ofrecen certificados RSA que autorizan
  `KeyEncipherment` y cuya clave privada es realmente recuperable por el motor
  para desproteger; no basta con que el certificado tenga una clave pública RSA
- un certificado opaco de Windows CertStore, PKCS#11 o hardware puede seguir
  sirviendo para firmar, pero se omite del catálogo `compat` si el adaptador no
  implementa descifrado. Para cifrar en ese perfil hay que cargar un P12/PFX
  RSA autorizado y apto, o usar el perfil `alto`
- en `alto` se expone además la identidad fuerte local del usuario

Operaciones útiles:

- GUI Qt/QML:
  - en `Cifrar`, elegir perfil `compat` y `CMS EncryptedData`;
  - introducir y confirmar una clave AES-256 en Base64 canónico de 44
    caracteres; es una clave criptográfica aleatoria de 32 bytes, no una
    contraseña y no se le aplica KDF;
  - conservarla fuera de GrxFirma por un canal seguro: la aplicación no la
    guarda ni ofrece recuperación y sin ella el documento no puede abrirse;
  - al proteger se fuerza el sufijo `.encrypted.p7m`; al desproteger ese sufijo
    vuelve a solicitar la clave. Un `.p7m` genérico exige marcar expresamente
    que contiene `EncryptedData`;
  - los campos se limpian al enviar, finalizar, cancelar, cambiar de flujo o
    cerrar. La sobrescritura en Qt es best-effort por sus copias implícitas;
    el motor Go conserva una sola copia binaria decodificada y la zeroiza.
- GUI WinUI:
  - seleccionar `CMS EncryptedData` y aportar dos veces la misma clave Base64
    canónica de 44 caracteres mediante los diálogos nativos;
  - la interfaz valida que represente exactamente 32 bytes, no usa
    `PasswordBox` ni materializa el secreto como `string` administrado;
  - el backend recibe los 32 bytes por `secretB64`, no por el mapa de opciones,
    y las copias controladas se borran al terminar;
  - conservar la clave fuera de GrxFirma por un canal seguro: no existe
    recuperación si se pierde.
- listar destinatarios locales:
  - `grxfirma -modo-cli -operacion listar-destinatarios-proteccion -salida-json`
  - si no aparece ninguno con perfil `compat`, cargar temporalmente un P12/PFX
    RSA autorizado o cambiar a `alto`; importar el certificado en el almacén
    de Windows no convierte una clave opaca en clave descifrable
- exportar un destinatario fuerte para otro equipo:
  - `grxfirma -modo-cli -operacion exportar-destinatario-proteccion -destinatario <id_alto> -salida /tmp/destinatario.afpr.json`
- importar un destinatario fuerte previamente compartido:
  - `grxfirma -modo-cli -operacion importar-destinatario-proteccion -entrada /tmp/destinatario.afpr.json`
- proteger un fichero:
  - `grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -destinatario <id_alto> -perfil-proteccion alto`
  - `grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -destinatario <id_compat> -perfil-proteccion compat`
  - `grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -destinatario <id_compat> -perfil-proteccion compat -contenedor-proteccion cms`
  - `grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -perfil-proteccion compat -contenedor-proteccion cms-encrypted -clave-proteccion-stdin`
- proteger firmando en CMS heredado:
  - `grxfirma -modo-cli -operacion proteger-firmando -entrada /ruta/secreto.pdf -destinatario <id_compat> -id-certificado <id_cert_firma> -perfil-proteccion compat`
- proteger con cifrado autenticado CMS:
  - `grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -destinatario <id_compat> -perfil-proteccion compat -contenedor-proteccion auth-enveloped-data`
- desproteger un fichero:
  - `grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.afp`
  - `grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.enveloped`
  - `grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.encrypted.p7m -clave-proteccion-stdin`
  - `grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.signedenveloped.p7m`
  - `grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.authenveloped.p7m`

Estado del frente CMS heredado de 1.9:
- operativo hoy: `EnvelopedData`, `EncryptedData`, `SignedAndEnvelopedData` y
  `AuthEnvelopedData`
- algoritmos CMS reales: `EnvelopedData` usa RSA PKCS#1 v1.5 + AES-256-GCM;
  `AuthEnvelopedData` usa RSA-OAEP-SHA256/MGF1-SHA256 + AES-256-GCM
- OpenSSL parsea el DER V2, V2 descifra AES-256-CBC de OpenSSL y el
  intercambio `AuthEnvelopedData` con OpenSSL 3.5 está probado en ambos sentidos
- el fixture V1.9 usa AES-ECB/PKCS5 y ASN.1 legacy; V2 no lo escribe. Para
  migrar sobres conocidos, su lector de solo lectura se activa con
  `GRXFIRMA_ENABLE_LEGACY_CMS_AES_ECB=1`, valida límites/padding, firma,
  vigencia y confianza X.509 antes de devolver texto claro. La muestra oficial
  histórica se rechaza porque su firmante está caducado/no confiable
- `SignedAndEnvelopedData` solo entrega contenido tras validar integridad,
  vigencia/uso de firma y cadena X.509 hasta las anclas del sistema o
  configuradas; si el truststore no está disponible, falla cerrado
- detectado y rechazado con error explícito: `AuthenticatedData` y
  `CompressedData`; no cifran y quedan fuera del perfil seguro conservado
- compatibilidad observable ya cubierta:
  - aliases legacy de contenedor como `authenvelopeddata`,
    `authenticateddata` y `compresseddata`;
  - detección por OID ASN.1 en desprotección;
  - soporte de `AuthEnvelopedData` en `protect` y `unprotect`, con límites,
    validación certificado-clave/`KeyUsage` y fallo cerrado sin distinguir
    entre clave incorrecta, OAEP inválido o autenticación GCM fallida;
  - rechazo explícito de `AuthenticatedData` y `CompressedData`.
  - `proteger-firmando` ya normaliza aliases de `SignedAndEnvelopedData`
    y rechaza de forma explícita cualquier otro contenedor CMS para no
    aceptar silenciosamente un modo incompatible.
  - `CLI` y `REST /protect-sign` ya reflejan esa misma semántica de entrada:
    aliases válidos de `SignedAndEnvelopedData` sí; ese flujo separado rechaza
    correctamente el resto de modos CMS, incluido `AuthEnvelopedData`.
  - ejemplo: `-contenedor-proteccion signed-and-enveloped-data` se acepta en
    `proteger-firmando`, mientras que `authenvelopeddata` se usa con
    `proteger`, porque no incorpora firma del remitente.

## Huellas de integridad

Estado actual:

 - utilidad de huellas de fichero y directorio ya operativa en CLI y REST local;
- compatible con la semántica de `createdigest` / `checkdigest` de AutoFirma 1.9;
- algoritmos soportados:
  - `SHA-1`
  - `SHA-256`
  - `SHA-384`
  - `SHA-512`
- formatos de artefacto soportados:
  - `.hexhash`
  - `.hashb64`
  - `.hash`
  - `.hashfiles`
  - `.txthashfiles`
  - `.csv`
  - `.hashreport`

Operaciones útiles:

- crear huella:
  - `grxfirma -modo-cli -operacion crear-hash -entrada /ruta/documento.pdf -algoritmo-hash sha256 -formato-hash hex`
  - `grxfirma -modo-cli -createdigest -entrada /ruta/documento.pdf -algoritmo-hash sha256 -formato-hash hex`
- comprobar huella:
  - `grxfirma -modo-cli -operacion comprobar-hash -entrada /ruta/documento.pdf -fichero-hash /ruta/documento.pdf.hexhash`
  - `grxfirma -modo-cli -checkdigest -entrada /ruta/documento.pdf -fichero-hash /ruta/documento.pdf.hexhash`
- crear manifiesto de directorio:
  - `grxfirma -modo-cli -operacion crear-hash -entrada /ruta/directorio -formato-hash xml -recursive`
- comprobar manifiesto de directorio:
  - `grxfirma -modo-cli -operacion comprobar-hash -entrada /ruta/directorio -fichero-hash /ruta/directorio.hashfiles -salida /ruta/directorio.hashreport`
- REST local:
  - `POST /hash`
  - `POST /hash/check`
  - al crear huellas, la consola `/` muestra también el artefacto esperado de salida
    y avisa si la ruta no termina en la extensión coherente con el formato elegido;
  - al comprobar huellas, la consola `/` infiere si el artefacto cargado parece
    de fichero o de directorio y avisa cuando no coincide con el modo actual;
  - `/hash/check` devuelve `report_base64` para directorios y puede guardar `report_output_path`
  - la consola web `/` ya muestra también un detalle guiado de diferencias del manifiesto:
    - entradas que coinciden;
    - entradas con hash distinto;
    - hashes sin fichero;
    - ficheros nuevos fuera del manifiesto.
- la consola web `/` muestra además un resumen guiado del destinatario de protección:
  - perfil (`alto` o `compat`);
  - algoritmo;
  - id;
  - y contenedor efectivo (`json/.afp` o `cms/.enveloped` cuando aplica).
- la consola `/` muestra además un resumen del catálogo de destinatarios disponible
  para el perfil seleccionado y del artefacto protegido esperado según la ruta de salida;
- antes de proteger, la consola `/` muestra también un plan visible de protección:
  - origen;
  - número de destinatarios;
  - perfil;
  - contenedor efectivo;
  - salida prevista;
  - y modo de entrega (`disco`, `memoria`, `base64`).
- antes de desproteger, la consola `/` detecta también el contenedor protegido por
  extensión cuando puede inferirlo y muestra un plan visible de desprotección.

## Diagnóstico rápido

### Si falla la carga de certificados

Comprobar:

- que el directorio P12 existe
- que el fichero `.p12` o `.pfx` es legible
- que la contraseña es correcta
- que OpenSSL está disponible si el contenedor usa cifrado de transporte
  antiguo no soportado por la biblioteca Go
- que la fecha actual está dentro de la vigencia del certificado; una firma
  puede ser criptográficamente íntegra y, a la vez, presentar el certificado
  como caducado
- que la política o el entorno no redirigen el directorio a otra ruta

Si el catálogo está vacío en la GUI Qt/QML:

- `Usar certificado sin instalar` acepta P12/PFX o un único bundle PEM sin
  cifrar que contenga certificado y clave privada; la identidad solo vive en
  la memoria del proceso y se puede retirar desde el selector;
- `Importar o abrir gestor` separa la instalación persistente: hay que escoger
  el almacén de destino antes de importar un P12/PFX;
- `Actualizar certificados` vuelve a consultar el catálogo tras una
  importación externa o un cambio en el gestor;
- el navegador mostrado es una recomendación de mejor esfuerzo basada en
  `BROWSER` y gestores instalados, no una detección garantizada de la ventana
  activa;
- si no aparece un destino NSS, comprobar `cert9.db`/`cert8.db` y la
  disponibilidad de `pk12util`; Firefox Snap y Flatpak mantienen perfiles
  separados.

Límites de seguridad del diálogo:

- archivo regular, sin enlace simbólico, no vacío y de hasta `2 MiB`;
- credenciales y contraseñas se redactan de logs IPC y no se guardan para
  reintentar una conexión;
- el backend conserva un timeout inicial de 60 segundos antes de la primera
  trama, pero el canal autenticado admite 30 minutos de inactividad. El usuario
  puede mantener abierto el selector, revisar el PDF o colocar el sello sin
  perder el motor al minuto;
- si el canal se corta mientras el backend iniciado por la GUI continúa vivo,
  Qt vuelve a conectarse. Una operación pendiente falla de forma explícita y
  cualquier contraseña o clave transitoria debe introducirse de nuevo: nunca
  se conserva para un reintento silencioso;
- en WinUI, la ruta CredUI de respaldo no consulta ni escribe el Administrador
  de credenciales y no ofrece guardar la contraseña;
- el uso temporal no escribe la credencial en disco; la importación persistente
  sí usa un temporal privado `0600` que se elimina al acabar;
- retirar o cerrar elimina la identidad del catálogo temporal, aunque el
  runtime de Go no permite garantizar el borrado físico inmediato de todas las
  copias de la clave ya parseada.

En la consola web local `/`, el bloque `Certificados` muestra además un resumen visible del certificado seleccionado:

- asunto;
- emisor;
- validez;
- huella;
- y si el backend lo considera apto o no para firma.
- también permite filtrar visualmente la tabla por asunto, emisor y vigencia, mostrando cuántos certificados quedan visibles respecto al total cargado.
- además resume si existen certificados aptos para firma y si alguno de ellos sigue
  visible tras aplicar los filtros actuales.

### Si falla Native Messaging

Comprobar:

- que `make bridge-user` dejó los manifests en el navegador correcto
- que la ruta del script `browser-bridge.sh` apunta al binario real
- que el ID del paquete Chromium coincide con
  `extensions/chromium/dipgra-extension-chromium.id` cuando exista
- que `allowed_origins` o `allowed_extensions` contienen únicamente las
  extensiones publicadas o gestionadas esperadas
- que los errores salen por `stderr`, nunca por `stdout`

`grxfirma-nativehost` no es una herramienta de línea de órdenes. Si se
ejecuta a mano, sin los argumentos que añade el navegador, termina de forma
deliberada con `validar_caller`. Firefox entrega además la ruta del manifiesto:
el host comprueba que sea un JSON regular, que autorice el ID y que su `path`
apunte al mismo binario o a su `browser-bridge.sh`. Chromium entrega el origen
`chrome-extension://…`; se contrasta con los IDs oficiales, el ID del paquete
instalado y los manifiestos ligados al ejecutable.

La extensión distribuida requiere este canal para las operaciones
criptográficas. El fallback REST está deshabilitado por defecto y no debe
considerarse disponible solo porque responda `/health`: necesita un Bearer
aprovisionado y una petición autenticada correcta a `/certificates`.

Rutas esperadas tras `make bridge-user`:

- `~/.local/lib/grxfirma/browser-bridge.sh`
- `~/.local/lib/grxfirma/bin/grxfirma-nativehost`
- `~/.mozilla/native-messaging-hosts/com.grxfirma.native.json`
- `~/.mozilla/native-messaging-hosts/com.dipgra.grxfirma.json`

En Chrome, Chromium y Edge los manifests se instalan solo si existe el directorio
de Native Messaging del navegador en el perfil del usuario.

Para una comprobación de release sin tocar perfiles habituales se deben usar
los arneses de `packaging/browser-extensions/tests/`. Crean perfiles
temporales, validan la identidad del host y restauran manifiestos/permisos. Chrome estable no se usa para
cargar una extensión unpacked si su política de sideload lo impide; la
distribución real sigue requiriendo tienda o política empresarial. Firefox
Snap necesita el portal WebExtensions, `geckodriver` y el cliente `flatpak`
durante esa prueba temporal.

Los gates focalizados del borde de confianza son:

```bash
go test -tags production ./cmd/nativehost \
  ./internal/adapters/inbound/desktop/nativehost ./internal/application
node --test packaging/browser-extensions/tests/background.test.mjs
python3 packaging/browser-extensions/tests/chromium/test_native_messaging_e2e.py
python3 packaging/browser-extensions/tests/firefox/test_native_messaging_e2e.py
```

Las pruebas negativas cubren otra extensión, origen no permitido, acción no
autorizada, campos de control falseados y replay. No se debe solucionar un
rechazo ampliando el manifiesto a comodines: se reinstala el conector correcto
o se despliega por política un ID exacto.

Toda restricción que pueda alcanzar al usuario debe devolver tres elementos:
la condición incumplida, el motivo por el que se detuvo la operación y una
acción segura y concreta. Si no existe una recuperación local, debe decirlo y
remitir al administrador o a la sede; nunca se recomienda desactivar TLS,
protecciones del navegador, validaciones criptográficas o controles del
sistema.

### Prueba local de identidad reforzada

La acción `proveIdentity` está reservada a portales HTTPS incluidos de forma exacta en
la configuración gestionada de la extensión. El portal entrega únicamente el reto
canónico Base64 y una correlación UUID; no puede consultar certificados ni indicar
cuál debe usarse. GrxFirma muestra en el equipo el origen, la finalidad y la
operación, solicita la elección del certificado y conserva el consentimiento local.

La política de máquina `aprobacion_automatica_nativehost` no omite estos dos pasos. Si no aparece el
selector, compruebe la instalación del host, el ID de extensión y los
`host_permissions`; no habilite comodines ni envíe el certificado desde la página.
Los gates automáticos focalizados son:

```bash
go test -race ./internal/application \
  ./internal/adapters/inbound/desktop/nativehost ./cmd/nativehost
node --test packaging/browser-extensions/tests/*.test.mjs
python3 packaging/browser-extensions/tests/test_build.py
```

Estos gates no sustituyen la instalación real del paquete y una operación completa en
cada navegador/plataforma autorizados.

### Si falla el protocolo `afirma://`

Comprobar:

- que el desktop entry está instalado
- que `xdg-mime` registró `x-scheme-handler/afirma`
- que el usuario tiene `grxfirma.desktop` en `~/.local/share/applications/`
- que el desktop entry apunta a `Exec=grxfirma-afirmauri %u`
- que existe `~/.local/bin/grxfirma-afirmauri`

Comandos útiles:

```bash
xdg-mime query default x-scheme-handler/afirma
grep '^Exec=' ~/.local/share/applications/grxfirma.desktop
ls -l ~/.local/bin/grxfirma-afirmauri
```

Solo para un checkout de desarrollo, nunca sobre el paquete distribuido:

```bash
make desktop-user-debug
xdg-mime query default x-scheme-handler/afirma
tail -f /tmp/grxfirma-debug.log
```

Para volver al modo normal:

```bash
make desktop-user
```

El objetivo `desktop-user-debug` compila una edición de laboratorio sin la
etiqueta `production`. Debe usarse con datos sintéticos, retirar sus trazas al
terminar y reinstalar después el paquete oficial.

### Si el portal no conecta por HTTPS/WSS local

Los navegadores modernos separan tres controles que no deben confundirse:

- permiso del portal para acceder a aplicaciones del mismo equipo
  (`loopback-network`);
- permiso del portal para acceder a dispositivos de la red privada
  (`local-network`);
- confianza TLS en la CA local administrada por GrxFirma.

Firefox 153 y versiones posteriores piden estos permisos por sitio. En la
interfaz pueden aparecer como `Aplicaciones y servicios del dispositivo` y
`Dispositivos de la red local`; el texto exacto depende del idioma y de la
versión. Chrome/Chromium también aplica permisos de acceso local en versiones
recientes.

Procedimiento seguro:

1. Confirmar que el portal remoto usa HTTPS y que su dominio es el esperado.
2. Iniciar una sola operación y aceptar únicamente el permiso solicitado para
   ese portal. El canal de GrxFirma en `localhost`/`127.0.0.1` necesita
   `loopback-network`; conceder también `local-network` solo si el portal lo
   solicita porque prueba ambos espacios de direcciones.
3. Si se rechazó el aviso o se guardó una decisión incorrecta, abrir la
   información/permisos del sitio desde la barra de direcciones y restablecer
   solo esos permisos para el dominio afectado. No crear una excepción global.
4. Comprobar el material y la confianza local:

   ```bash
   grxfirma -estado-almacen-tls
   grxfirma -estado-confianza-tls
   ```

5. Si se acaba de instalar, regenerar o rotar la CA local, cerrar por completo
   todas las ventanas y procesos de Firefox o Chrome/Chromium y abrir de nuevo
   el navegador antes de repetir la prueba. Una pestaña nueva no garantiza que
   el proceso haya recargado el almacén de confianza.

Para acotar la causa, revisar la consola web del portal sin copiar payloads:

- un mensaje `Local Network Access` o permiso denegado apunta al permiso por
  sitio;
- `unknown issuer`, `certificate unknown` o un error equivalente apunta a la
  confianza TLS local;
- `connection refused` o ausencia de listener apunta a que el handler no llegó
  a iniciar el servicio;
- un error funcional posterior a establecer WSS apunta a la operación, no al
  permiso ni a la CA.

No se debe continuar aceptando una advertencia de certificado, importar una
raíz obtenida de una web, desactivar la comprobación TLS ni modificar
`network.lna.enabled`, `network.lna.blocking` o listas globales de exclusión en
`about:config`. Tampoco se debe desactivar la seguridad equivalente en Chrome.
En equipos gestionados, las excepciones por origen deben desplegarse mediante
la política corporativa del navegador y limitarse a los portales aprobados.

### Si una firma no progresa

Mirar en este orden:

1. `audit.jsonl`
2. `/tmp/grxfirma-metrics.json`
3. exportación de incidencia saneada desde la aplicación
4. límites efectivos y timeouts

Si el mensaje indica que se cerró la conexión IPC:

- comprobar que frontend y backend proceden de la misma instalación;
- esperar la reconexión automática cuando el backend siga vivo;
- repetir la operación y volver a introducir únicamente la credencial
  transitoria solicitada;
- no aumentar globalmente los límites ni guardar la contraseña en una variable
  de entorno. El canal autenticado ya contempla 30 minutos de interacción
  humana; un cierre más temprano debe investigarse como terminación del
  backend, cambio de versión o fallo del socket.

Un paquete de producción no admite nivel `DEBUG`. Si soporte necesita una
reproducción más detallada, debe usar una build de desarrollo separada, datos
sintéticos y la disciplina temporal indicada en la política de retención; no
debe modificar el paquete instalado ni pedir al usuario el log crudo.

## Tabla de formatos y estado de conformidad

| Formato | Estado en V2 | Uso previsto | Conformidad ETSI |
|---|---|---|---|
| CAdES-BES | Implementado | Firma general detached | Operativa |
| CAdES-T | Implementado | Sello de tiempo | Operativa |
| XAdES-BES | Implementado | XML firmado detached | Operativa con validación DSS |
| XAdES-T | Implementado | XML con sello de tiempo | Operativa con validación DSS |
| PAdES-B-B | Implementado | PDF firmado | Operativa local; variante Adobe-compatible disponible; recomendable validacion externa adicional |
| PAdES-B-T | Implementado | PDF con sello de tiempo | Operativa con validación DSS |
| CAdES-LT/LTA | Implementado | Larga duración CMS | Operativa local; recomendable validacion externa adicional |

## Validación manual PAdES

Para cerrar la validación externa final de PAdES en visores PDF conviene comprobar,
como mínimo, estos escenarios:

1. PDF `ETSI.CAdES.detached` generado por V2.
2. PDF `adbe.pkcs7.detached` generado por V2 con `subfilter=adobe`.
3. PDF propio de prueba (`test/regression/fixtures/v1/samples/2.pdf`), firmado localmente con un certificado de pruebas autorizado.

Checklist mínima:

- `pdfsig <pdf>` debe detectar la firma y marcarla como válida.
- `qpdf --check <pdf>` debe pasar sin errores estructurales.
- Okular debe mostrar la firma como presente y no corrupta.
- Acrobat Reader debe abrir el documento sin advertencias de corrupción del PDF.
- Si el visor no confía en la cadena, debe seguir reconociendo la firma como estructura
  PAdES válida aunque la confianza del certificado no sea completa.

La verificación estructural de una firma y la confianza en su certificado son
resultados distintos. Usa solo certificados de ensayo autorizados y conserva
las muestras firmadas fuera del repositorio hasta que se revisen para publicar.

Ayuda local:

```bash
bash scripts/validar_pades_manual.sh <pdf-firmado> [<pdf-firmado>...]
```

Para generar muestras locales desde V2:

```bash
GOCACHE=/tmp/grxfirma-gocache go run ./scripts/generar_muestras_pades.go -out /tmp/grxfirma-pades-samples
bash scripts/validar_pades_manual.sh /tmp/grxfirma-pades-samples/pades-etsi.pdf /tmp/grxfirma-pades-samples/pades-adobe.pdf /tmp/grxfirma-pades-samples/pades-adobe-t.pdf
```

Para ejecutar el cierre local disponible en este entorno:

```bash
make pades-cierre-local
```

El informe queda en:

```text
/tmp/grxfirma-pades-cierre/reporte.txt
```

Para preparar el paquete externo listo para Acrobat/Okular:

```bash
make pades-validacion-externa
```

El paquete queda en:

```text
/tmp/grxfirma-pades-validacion-externa
```

Y contiene:

- `muestras/` con los PDF generados localmente
- `evidencias/` con validación local y hashes
- un registro de validación local

## Guía de transición V1→V2

### Objetivo del switchover

Sustituir el uso operativo de V1 en escritorio manteniendo rollback controlado mientras se valida compatibilidad real con webs y navegadores.

### Preparación

1. Instalar la base de usuario:

```bash
make install-user
```

Si además se quiere dejar instalada la GUI Qt/QML local en Linux:

```bash
packaging/linux/install-user.sh --with-qt
```

2. Instalar los manifests de Native Messaging:

```bash
make bridge-user
```

3. Registrar V2 como handler de `afirma://`:

```bash
make desktop-user
```

4. Cargar al menos un `.p12` o `.pfx` válido en:

```text
~/.config/grxfirma/pkcs12
```

5. Verificar que la configuración base mantiene el modelo Zero Server:

```bash
grep -E '"websocket_habilitado"|"rest_habilitado"' ~/.config/grxfirma/config.json
```

### Validación previa

1. Probar firma CLI directa con un certificado local.
2. Probar Native Messaging con navegador soportado y extensión compatible.
3. Probar `afirma://` desde un portal controlado o con una URI de prueba.
4. Verificar que se generan auditoría y métricas.
5. Revisar que no hay puertos abiertos si no se habilitan explícitamente.

Comprobaciones mínimas:

```bash
xdg-mime query default x-scheme-handler/afirma
test -x ~/.local/bin/grxfirma
test -x ~/.local/bin/grxfirma-afirmauri
test -x ~/.local/lib/grxfirma/bin/grxfirma-nativehost
```

La salida esperada del handler es:

```text
grxfirma.desktop
```

### Switchover

1. Mantener V1 instalado como binario si se necesita rollback rápido.
2. Ejecutar:

```bash
make bridge-user
make desktop-user
```

3. Confirmar que `afirma://` ya apunta a V2:

```bash
xdg-mime query default x-scheme-handler/afirma
```

4. Confirmar que el desktop entry del usuario referencia el binario correcto:

```bash
grep '^Exec=' ~/.local/share/applications/grxfirma.desktop
```

Resultado esperado:

```text
Exec=grxfirma-afirmauri %u
```

5. Repetir la batería mínima de validación:
   - firma CLI
   - Native Messaging
   - portal con `afirma://`
   - presencia de `audit.jsonl` y `/tmp/grxfirma-metrics.json`

### Rollback

Si aparece regresión funcional:

1. Retirar el handler y binarios de usuario de V2:

```bash
make uninstall-user
```

2. Restaurar el handler y los manifests de V1 con el mecanismo de instalación de V1.
3. Si se quiere limpiar del todo el bridge de V2, eliminar manualmente:
   - `~/.local/lib/grxfirma/`
   - `~/.mozilla/native-messaging-hosts/com.grxfirma.native.json`
   - `~/.mozilla/native-messaging-hosts/com.dipgra.grxfirma.json`
   - los manifests equivalentes de Chrome, Chromium y Edge si existen
4. Conservar `audit.jsonl` y `/tmp/grxfirma-metrics.json` para análisis.
5. Documentar el caso y no eliminar V2 definitivamente hasta aislar la causa.

## Evidencia de identidad reforzada

El servicio que componga `identityevidence` debe recibir desde el gestor de
secretos una clave aleatoria de 32 bytes y una versión no vacía. La clave no se
guarda junto al registro. El directorio configurado debe ser absoluto, privado
(`0700`) y no compartirse entre procesos escritores; el fichero se crea con
`0600`.

En cada arranque se verifica la cadena HMAC completa. Un error
`registro de evidencia de identidad corrupto` exige aislar el fichero, conservarlo
para análisis y arrancar únicamente con un destino nuevo aprobado: no se debe
truncar, reparar ni ignorar automáticamente. La rotación de clave requiere un
registro nuevo y custodia de la clave anterior durante el periodo legal de
retención.
