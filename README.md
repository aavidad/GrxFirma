<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma

GrxFirma es una aplicación Go-first para firma, verificación e integración local
de firma electrónica, con núcleo hexagonal y adaptadores separados para CLI, desktop,
REST local, `afirma://`, WebSocket legacy y Native Messaging.

La documentación pública describe las capacidades implementadas y cómo compilar,
instalar y utilizar el proyecto. Cada distribución debe validarse sobre los
artefactos concretos que se vayan a entregar.

## Capacidades

- Núcleo hexagonal operativo en `internal/domain`, `internal/application` e
  `internal/ports`.
- Firma y verificación en Go para:
  - `CAdES`
  - `XAdES`
  - `PAdES`
  - `XMLdSig`
  - `ODF`
  - `OOXML`
  - `FacturaE`
  - `ASiC-XAdES`
- Adaptadores de entrada activos:
  - CLI `grxfirma`
  - desktop Fyne `grxfirma-desktop`
  - bootstrap IPC/Qt `grxfirma-gui`
  - desktop nativo Windows `grxfirma-winui`
  - `afirma://` / WebSocket / `service` legacy `grxfirma-afirmauri`
  - Native Messaging `grxfirma-nativehost`
  - REST local con TLS, consola web local y firmador web local
- Utilidades hash de fichero y directorio ya operativas en CLI, REST, GUI QML
  y consola web local.
- Protección local ya operativa con contenedor nativo `.afp` y contenedores CMS
  DER estándar `.enveloped` (`EnvelopedData`), `.encrypted.p7m`
  (`EncryptedData`), `.signedenveloped.p7m` (`SignedAndEnvelopedData`) y
  `.authenveloped.p7m` (`AuthEnvelopedData` autenticado).
  La interoperabilidad CMS externa todavía es parcial y se detalla más abajo.
- Verificación y validación avanzada expuestas en CLI, REST y GUI QML con
  resultado rico estructurado. La utilidad dedicada `/validator`
  (`/validador`) añade validación de certificados contra raíces del sistema,
  OCSP/CRL, hash y tres informes JSON.
- La extensibilidad es deliberadamente de compilación: no se cargan plugins de
  usuario en proceso. Las utilidades oficiales de V1.9 se integran y un gate de
  CI/release impide introducir cargadores dinámicos.
- La GUI Qt/QML ya permite defaults automáticos por tipo documental cuando el
  formato por defecto está en `Auto`: `PDF`, `OOXML`, `FacturaE`, `ODF`,
  `XML` y binario/otros.
- La GUI Qt/QML ya expone `strictCompat` para fijar un baseline más compatible.
  Los PDF estructuralmente inválidos se rechazan siempre: la antigua opción
  `allowInvalidPDF` se conserva únicamente en los contratos legacy para
  devolver un error explícito si alguien intenta activarla.
- La GUI Qt/QML ya distingue entre:
  - último certificado recordado;
  - certificado predeterminado;
  - preferencia para arrancar directamente usando el certificado predeterminado;
  - autoselección configurable cuando solo hay un certificado disponible;
  - prioridad visual configurable para mostrar primero el certificado
    predeterminado;
  - prioridad visual configurable para mostrar primero certificados aptos y
    vigentes para firma.
- La GUI Qt/QML ya persiste defaults útiles de hash:
  - algoritmo por defecto;
  - copia automática al portapapeles para huellas simples;
  - formato por defecto de hash de fichero;
  - formato por defecto de hash de directorio;
  - modo recursivo por defecto;
  - guardado por defecto del `.hashreport`.
- La GUI Qt/QML ya expone defaults visibles de `PAdES`:
  - firma visible por defecto;
  - aplicación del sello visible en todas las páginas por defecto;
  - mantener texto sobre imagen por defecto.
- La GUI nativa WinUI conecta el mismo backend Go para firma, verificación,
  huellas, protección, certificados, configuración, diagnóstico y ayuda. Su
  firma PAdES ofrece preview y sello visible por página/rangos/todas las
  páginas; la zona se mueve y redimensiona directamente sobre la página, con
  porcentajes X/Y/ancho/alto como alternativa precisa y accesible. Incluye
  verificación posterior al guardado y cofirma múltiple guiada para `PAdES`,
  `ODF` y `OOXML`. Protección admite también `EncryptedData`: pide la
  clave Base64 de 32 bytes mediante un diálogo Win32 nativo, la transporta en
  `secretB64` binario y borra las copias controladas. También permite validar
  online un certificado y establecer o quitar el predeterminado. La firma por
  lotes permite combinar selección múltiple y una carpeta no recursiva,
  escoger la carpeta de salida, cancelar y revisar el resultado confirmado de
  cada documento, incluidos resultados parciales. Certificados permite cargar
  P12/PFX solo para la sesión o importarlo de forma persistente en uno de los
  destinos tipados que publica el backend, además de retirar una credencial
  temporal o limpiar todas tras confirmación. El inventario temporal pertenece
  a la sesión IPC compartida, por lo que la marca y la retirada siguen
  disponibles al volver a la página. Configuración permite crear, rotar y
  retirar credenciales de proxy con entrada transitoria: la contraseña se
  captura en un diálogo nativo, viaja en un buffer borrable y Windows la
  entrega al almacén DPAPI del usuario sin persistirla en `settings.json`. El
  blob cifrado tiene tamaño limitado, DACL privada y escritura atómica; las
  lecturas y el borrado rechazan enlaces y puntos de reanálisis.
  Diagnóstico muestra fases locales con icono y texto y, solo en Windows,
  permite instalar o retirar de forma reversible la confianza TLS local
  inventariada por GrxFirma; no prueba ni atribuye un fallo a un portal o a
  `@firma` sin evidencia de una operación real. Como complemento opcional y
  oculto por defecto, puede generar Facturae 3.2.2, validar sus datos y guiar la
  validación, remisión, justificante y consulta de estado en los portales
  oficiales de FACe; no automatiza ni declara realizado el envío externo.
- La base de settings ya no parte de cero:
  - existe documento tipado parcial para `General`, `Firma`, `Hash`, `Proxy`
    y `Certificados`;
  - la compatibilidad legacy sigue abierta mediante claves planas fuera de esos
    bloques mientras la migración no se cierre.
- Las interfaces de escritorio Qt/QML y WinUI pueden avisar de una versión
  estable más reciente publicada en las Releases oficiales de GitHub. La
  comprobación automática se puede desactivar en Configuración y existe una
  comprobación manual en Acerca de. El cliente solo compara versiones: nunca
  descarga ni ejecuta una actualización y únicamente abre el destino HTTPS
  fijo del repositorio cuando lo decide el usuario.
- La trust policy ya puede quedar gobernada por despliegue gestionado:
  allowlist del sistema en `/etc/grxfirma/allowed-domains.json` y
  `tofu_habilitado=false` en `/etc/grxfirma/policy.json` (en Windows,
  `HKLM\SOFTWARE\Policies\GrxFirma`; véase
  [docs/POLITICA_MAQUINA.md](docs/POLITICA_MAQUINA.md)), dejando `TOFU`
  como fallback de desktop no gestionado.
- Base i18n embebida por JSON para `es`, `en`, `fr`, `de`, `it`, `pt`, `zh`,
  `eu`, `ca`, `gl` y `va`, con fallback a `es`. La auditoría exhaustiva sigue
  pendiente.

## Binarios principales

- `cmd/grxfirma`
  - CLI general, desktop Fyne y servidor REST local
- `cmd/grxfirma-gui`
  - bootstrap IPC para el frontend Qt/QML
- `cmd/grxfirmauri`
  - handler `afirma://`, WebSocket legacy y `service`
- `cmd/nativehost`
  - Native Messaging host para navegadores
- `cmd/gui-qml`
  - frontend Qt/QML opcional
- `cmd/gui-winui`
  - frontend nativo WinUI 3 para Windows x64

Los nombres generados por `make` son:

- `grxfirma`
- `grxfirma-desktop`
- `grxfirma-gui`
- `grxfirma-afirmauri`
- `grxfirma-nativehost`
- `grxfirma-winui` en la entrega nativa de Windows
- `grxfirma-gui-qml` si se compila el frontend Qt/QML

## Compilación rápida

```bash
go build ./cmd/grxfirma
go build ./cmd/grxfirma-gui
go build ./cmd/grxfirmauri
go build ./cmd/nativehost
```

Si necesitas el frontend Qt/QML:

```bash
(cd cmd/gui-qml && qmake6 grxfirma_qt.pro && make -j"$(nproc)")
```

## Capacidades verificables hoy

### Firma y verificación

- `CLI`, `REST` y GUI QML aceptan hoy los formatos:
  `AUTO`, `PAdES`, `CAdES`, `XAdES`, `XMLdSig`, `ODF`, `OOXML`, `FacturaE`,
  `ASiC-XAdES`.
- `AUTO` ya detecta en verificación:
  - `PAdES`
  - `CAdES`
  - `XAdES`
  - `XMLdSig`
  - `ODF`
  - `OOXML`
  - `FacturaE`
  - `ASiC-XAdES`
- Hay regresión real frente a V1 en `test/regression/` para `CAdES`, `XAdES`
  y `PAdES`, y tests específicos para `XMLdSig`, `ODF`, `OOXML`, `FacturaE` y
  `ASiC-XAdES`. Las muestras oficiales V1.9 de FacturaE y ASiC-XAdES verifican
  firma y referencias con C14N inclusiva/exclusiva nativa; su certificado
  histórico caducado se muestra como un aspecto separado de la integridad.
- La cofirma múltiple guiada de documento único está cerrada en GUI QML,
  WinUI y `/signer` para `PAdES`, `ODF` y `OOXML`: exige un firmante adicional
  distinto del principal, encadena las cofirmas en una única salida y elimina
  las opciones de sello visible/QR de las firmas posteriores para no
  superponerlas. Las integraciones reales del motor verifican el resultado
  final de los tres formatos con tres certificados; la campaña instalada de
  WinUI sigue siendo un gate de release separado.
- `CAdES`, `XAdES`, `XMLdSig`, `FacturaE` y `ASiC-XAdES` no ofrecen esta
  función guiada porque sus motores actuales no soportan esa secuencia. La
  contrafirma y los lotes son flujos distintos y quedan fuera de su alcance.

### Hash de fichero y directorio

- CLI:
  - `createdigest` / `-operacion crear-hash`
  - `checkdigest` / `-operacion comprobar-hash`
- REST:
  - `POST /hash`
  - `POST /hash/check`
- consola web local:
  - validación de firma desde `/`
  - utilidad dedicada de firma, certificado y hash desde `/validator`
    (`/validador`)
  - creación y comprobación de hash desde `/`
  - resumen guiado del destinatario seleccionado al proteger documentos
  - resúmenes visibles del plan actual de verificación, hash,
    protección y desprotección
  - resumen visible de la referencia al original en validación
  - resumen visible del tipo de artefacto hash cargado y del catálogo de
    destinatarios de protección disponible
  - resúmenes visibles del artefacto firmado y del contenedor protegido
  - estado visible de filtros de certificados e inventario cargado
- GUI QML:
  - panel `Huellas e integridad`
  - creación y comprobación de huellas de fichero
  - creación y comprobación de manifiestos de directorio
  - guardado opcional de `.hashreport`
- Algoritmos soportados:
  - `SHA-1`
  - `SHA-256`
  - `SHA-384`
  - `SHA-512`
- Formatos de hash de fichero:
  - `.hexhash`
  - `.hashb64`
  - `.hash`
- Manifiestos de directorio:
  - generación: `xml`, `txt`, `csv`
  - comprobación: `xml`, `txt`
- Informe de comprobación de directorio:
  - `.hashreport` en XML compatible con la semántica observable de V1.9
- Diferencia importante respecto a V1.9:
  - V2 ya acepta manifiestos legacy con `\` y `/` al comprobar y filtra los
    temporales/sistema más típicos de V1; también admite el Base64 estándar y
    URL-safe heredado, con validación estricta de algoritmo, tamaño y rutas;
  - V2 genera en modo estable con `/` y orden determinista;
  - no existe un modo byte a byte: V1.9 construía y serializaba esos
    manifiestos con orden dependiente de concurrencia/runtime, por lo que su
    salida no era un contrato reproducible;
  - `csv` es solo salida; la comprobación interoperable usa `xml` o `txt`.

### Protección local y CMS

- Perfil `alto`:
  - contenedor nativo `.afp`
  - identidad local fuerte autogenerada cuando falta
- Perfil `compat`:
  - contenedor nativo `.afp`
  - contenedor CMS `.enveloped` con `EnvelopedData`
  - contenedor CMS `.encrypted.p7m` con `EncryptedData`
  - contenedor CMS `.signedenveloped.p7m` con `SignedAndEnvelopedData`
  - contenedor CMS `.authenveloped.p7m` con `AuthEnvelopedData`
  - solo publica destinatarios RSA con `KeyEncipherment` y clave privada
    realmente descifrable por el proceso. Los certificados opacos del almacén
    del sistema continúan disponibles para firma, pero no se anuncian para
    cifrado compatible; use un P12/PFX autorizado o el perfil `alto`
- Los algoritmos no son iguales en ambos tipos de contenedor:
  - `.afp` usa RSA-OAEP-SHA256 y AES-256-GCM;
  - `EnvelopedData` usa transporte RSA PKCS#1 v1.5 y AES-256-GCM;
  - `AuthEnvelopedData` usa RSA-OAEP-SHA256/MGF1-SHA256 y AES-256-GCM;
  - `SignedAndEnvelopedData` añade SHA-256/RSA-SHA256 y su apertura exige,
    antes de entregar texto claro, integridad criptográfica, vigencia/uso de
    firma del certificado y cadena X.509 hasta una ancla del sistema o
    configurada; si no hay almacén de confianza disponible, falla cerrado.
- Interoperabilidad CMS comprobada:
  - OpenSSL parsea el DER emitido por V2;
  - V2 descifra `EnvelopedData` AES-256-CBC generado por OpenSSL;
  - V2 y OpenSSL 3.5 intercambian `AuthEnvelopedData` en ambos sentidos con
    RSA-OAEP-SHA256 y AES-256-GCM;
  - la muestra oficial V1.9 usa AES-ECB/PKCS5 y una codificación ASN.1 legacy:
    V2 no la genera por seguridad. Su lector de migración requiere
    `GRXFIRMA_ENABLE_LEGACY_CMS_AES_ECB=1` y además exige integridad, vigencia
    actual y confianza X.509. La muestra histórica oficial tiene un firmante
    caducado/no confiable en el corte actual, por lo que se reconoce y valida
    criptográficamente, pero no se entrega su texto claro.
- Alias aceptados hoy para `EncryptedData`:
  - `cms-encrypted`
  - `cms-encrypted-data`
  - `encrypted`
  - `encrypteddata`
- Operaciones expuestas:
  - CLI `proteger`, `proteger-firmando`, `desproteger`, listado/export/import de
    destinatarios
  - REST `/protect`, `/protect-sign`, `/unprotect`,
    `/protection/recipients`, `/protection/recipient/export`,
    `/protection/recipient/import`
  - consola web, `/signer`, IPC y Qt/QML para `AuthEnvelopedData`, con
    destinatarios compatibles filtrados por el backend y algoritmos visibles
  - Qt/QML para `EncryptedData`, con entrada y confirmación de una clave
    Base64 canónica de 32 bytes, sin destinatarios ni firma, salida inequívoca
    `.encrypted.p7m` y borrado inmediato best-effort de las copias controladas
    al enviar, finalizar, cancelar o abandonar el flujo
  - WinUI para `EncryptedData`, con captura y confirmación en diálogos Win32
    nativos que mantienen el texto fuera de `string` administrados, conversión
    a buffers borrables, transporte IPC binario `secretB64` y zeroing de las
    copias controladas al finalizar o cancelar
- CMS heredado fuera del perfil conservado:
  - `AuthenticatedData`, porque autentica pero no cifra el contenido;
  - `CompressedData`, porque no aporta confidencialidad ni integridad.
  Ambos OID se reconocen y se rechazan de forma explícita.

### Integración legacy web

- El borde `afirma://` / WebSocket legacy ya cubre:
  - `selectcert`
  - `save`
  - `load`
  - `signandsave`
- `selectcert` ya autoselecciona el único certificado compatible cuando solo
  hay uno y persiste preferencia `sticky` cuando la operación lo solicita.
- La cancelación del lote local legacy ya devuelve `CANCEL` en vez de un error
  genérico.
- La cancelación del lote remoto legacy también devuelve `CANCEL` cuando el
  ejecutor remoto informa cancelación del usuario.
- El borde WebSocket legacy ya preserva códigos explícitos `SAF_xx` y `ERR-xx`
  en vez de degradarlos a un error genérico.
- El WebSocket legacy rechaza por defecto handshakes sin `Origin` o con `Origin:
  null`. Para integraciones V1 estrictamente locales que no envían cabecera,
  existe la política de máquina `permitir_origen_vacio`.
- `load` ya admite selector interactivo con multiselección y filtros por
  extensión cuando no llega `filePath`.
- La capacidad base de `service`, la fragmentación, la cancelación y los
  códigos terminales están cerrados en código y regresión de laboratorio.
  Sigue abierta la evidencia externa de `service`/`batch` y `selectcert` contra
  Portafirmas u otra sede prioritaria, ejecutada desde el paquete final.

### Validación visible

- La consola web local también consume esa verificación rica mediante
  `POST /verify`.
- `/validator` y `/validador` ofrecen una utilidad dedicada que:
  - verifica firmas con documento original opcional;
  - valida vigencia, `DigitalSignature` y cadena X.509 contra raíces del
    sistema;
  - consulta OCSP/CRL solo cuando el usuario lo solicita;
  - crea o comprueba hashes;
  - exporta informes JSON de firma, certificado y hash.
- El firmador web local ya muestra además un resumen de seguridad de la
  verificación posterior a la firma.
- El firmador web local `/signer` ya muestra además un resumen visible del
  tipo de selección, del estado del certificado seleccionado, de sus detalles
  visibles, de la aplicabilidad de metadatos según formato, del plan de firma
  y del inventario de certificados antes de ejecutar la operación.
- `CLI` y `REST` devuelven un `VerificationResult` rico con:
  - `valid`
  - `reason`
  - `details`
  - `integrity`
  - `certificate`
  - `trust`
  - `signerSummaries`
  - `warnings`
  - `errors`
  - `evidence`
- La GUI QML ya muestra estado, motivo, firmantes y bloques de
  `Integridad / Certificado / Confianza`.
- CAdES, PAdES, XAdES, XMLDSig, FacturaE, ASiC-XAdES, ODF y OOXML evalúan la
  cadena X.509. Las superficies desktop usan las raíces del sistema; una raíz
  ajena invalida y la ausencia real de anclas se presenta como confianza no
  evaluada, nunca como verde.
- La utilidad dedicada limita los ficheros antes de `FileReader`, conserva el
  token solo en memoria y evita que respuestas obsoletas pisen una selección
  más reciente.

### Extensibilidad segura

GrxFirma no replica el `URLClassLoader` de plugins de V1.9. Ejecutar módulos
de usuario en el proceso que maneja certificados, documentos y claves anularía
la separación de seguridad del núcleo. Las capacidades nuevas se revisan,
compilan, prueban y distribuyen dentro de la release oficial.

El gate `scripts/ci/check_no_runtime_plugins.py` bloquea plugins Go, frameworks
RPC e intérpretes Go/WASM/Lua, sin confundir los módulos Qt/QML ni PKCS#11. La
decisión y el modelo operativo están en
[ADR-004](docs/ADR-004-extensibilidad-sin-plugins-runtime.md) y
[Extensibilidad y utilidades integradas](docs/EXTENSIBILIDAD_Y_UTILIDADES_INTEGRADAS.md).

## Instalación local de usuario

Flujo real soportado desde el checkout:

```bash
packaging/linux/install-user.sh
```

Si quieres que además compile e instale el frontend Qt/QML con sus recursos
`qml/` y `assets/`:

```bash
packaging/linux/install-user.sh --with-qt
```

Esa ruta deja instalado en el usuario:

- `~/.local/bin/grxfirma`
- `~/.local/bin/grxfirma-gui`
- `~/.local/bin/grxfirma-desktop`
- `~/.local/bin/grxfirma-afirmauri`
- `~/.local/lib/grxfirma/bin/grxfirma-nativehost`
- y, si se usa `--with-qt`:
  - `~/.local/bin/grxfirma-gui-qml`
  - `~/.local/lib/grxfirma/gui-qml/qml`
  - `~/.local/lib/grxfirma/gui-qml/assets`
- preferencias Qt persistidas en:
  - `~/.config/grxfirma/settings.json`

Si una reinstalación de Linux no incluye Qt, el instalador limpia el frontend
Qt/QML anterior y sus recursos para que `grxfirma-gui` no siga resolviendo
una build obsoleta.

La instalación comprueba antes de tocar `~/.local` que todos los ELF resuelven
sus bibliotecas y que están disponibles los módulos QML declarados. El `.deb`
genera `Depends` desde `dpkg-shlibdeps` y desde los paquetes propietarios de
los `qmldir`; el `tar.gz` conserva un manifiesto y falla pronto si el runtime
Qt de la distribución está incompleto.

## Variables de entorno relevantes

### Entrada segura de secretos CLI

GrxFirma no acepta contraseñas, tokens ni claves simétricas como valores de
`argv`, porque quedarían visibles en la tabla de procesos:

```bash
grxfirma -contenedor-p12 identidad.p12 -contrasena-stdin -entrada documento.pdf
grxfirma -importar-p12 identidad.p12 -contrasena-p12-stdin
grxfirma -modo-cli -operacion desproteger -entrada documento.encrypted.p7m -clave-proteccion-stdin
```

En una terminal se solicita una línea sin eco. En automatización, esas mismas
opciones leen una línea canalizada por stdin desde el gestor de secretos. Como
compatibilidad temporal se admiten `GRXFIRMA_PKCS12_PASSWORD`,
`GRXFIRMA_REST_TOKEN` y `GRXFIRMA_PROTECTION_SECRET_B64`; deben
inyectarse solo en el proceso y no guardarse en scripts, unidades o ficheros de
entorno. El servidor REST genera un token efímero si no se configura
autenticación y entrega una configuración para `curl` en un fichero efímero
privado (`0600` en POSIX y DACL protegida en Windows). La salida muestra solo
la ruta; `curl --config <ruta>` evita copiar el Bearer a `argv` o al historial,
y el fichero se elimina al cerrar el servidor. Las antiguas opciones con valor (`-password`,
`-contrasena-p12`, `-token-rest` y `-clave-proteccion-b64`) se rechazan.

Los P12/PFX antiguos que OpenSSL 3 no abre con su proveedor predeterminado se
reintentan localmente con el modo de lectura `-legacy`. Esa compatibilidad solo
descifra el contenedor de transporte: no habilita SHA-1, DES ni algoritmos
antiguos para crear la firma. La vigencia y la confianza del certificado se
siguen evaluando por separado.

Configuración general:

- `GRXFIRMA_DIRECTORIO_P12`
- `GRXFIRMA_PKCS12_DIR`
- `GRXFIRMA_NIVEL_LOG`
- `GRXFIRMA_TIMEOUT_OPERACION_SEGUNDOS`
- `GRXFIRMA_MAX_TAMANO_DOCUMENTO_BYTES`

Compatibilidad criptográfica heredada, desactivada por defecto y solo
habilitable por un administrador mediante la política de máquina
([docs/POLITICA_MAQUINA.md](docs/POLITICA_MAQUINA.md)); las antiguas variables
`GRXFIRMA_ENABLE_LEGACY_*` ya no tienen efecto:

- `permitir_des_legacy`: permite el formato de sesión DES de V1.9.
- `permitir_sha1_legacy`: permite generar firmas SHA-1 pedidas por
  portales heredados. La verificación SHA-1 sigue disponible (con advertencia).
- `permitir_rutas_directas_protocolo`: permite que un portal legacy
  indique rutas locales absolutas en `load`, `save` o `signandsave`. Por
  defecto se exige selección explícita del usuario para no exponer ni
  sobrescribir ficheros de su perfil.

REST local:

- `GRXFIRMA_REST_TOKEN` (compatibilidad para automatización)
- `GRXFIRMA_REST_HABILITADO`
- `GRXFIRMA_HTTP_TIMEOUT_SEC`
- `GRXFIRMA_MAX_PAYLOAD_BYTES`
- `GRXFIRMA_MAX_BATCH_DOCS`
- `GRXFIRMA_MAX_SESSION_SEC`

Native Messaging:

- Nombre del host propio: `com.grxfirma.native`.
- `GRXFIRMA_PKCS12_DIR`
- `GRXFIRMA_PKCS12_PASSWORD` (compatibilidad; preferir stdin en CLI)
- Política de máquina `aprobacion_automatica_nativehost`
  - Solo para despliegues controlados; la variable
    `GRXFIRMA_NATIVEHOST_AUTO_APPROVE` ya no tiene efecto. Por defecto el host nativo solicita
    confirmación gráfica del usuario antes de firmar.
- El host solo arranca si el navegador le entrega fuera del mensaje JSON una
  extensión autorizada. Una ejecución manual sin identidad, un ID distinto al
  registrado o un manifiesto que apunte a otro binario se rechazan.
- La extensión valida su propio ID, el origen de la página y la acción antes de
  llamar al host. Los portales declarados en `host_permissions` solo pueden
  abrir el firmador; las operaciones criptográficas proceden de páginas
  internas de la extensión. El puente de loopback solo puede consumir una vez
  el token del documento pendiente.
- Cada solicitud estricta necesita un `requestId` nuevo. El host rechaza
  identificadores ausentes, mal formados o repetidos y el consentimiento de
  firma muestra la aplicación y el origen acreditados.
- Todo rechazo visible explica la condición incumplida y la recuperación
  segura: repetir la operación, reiniciar el navegador o reinstalar el
  conector oficial. Nunca se indica desactivar controles o ampliar allowlists
  con comodines.
- `GRXFIRMA_NATIVEHOST_ALLOW_DEVELOPMENT_CALLER=1` solo sirve en una build de
  desarrollo con depuración permitida. Los binarios `production` lo ignoran.

Integración web por navegador:

- Chrome, Chromium, Edge, Brave, Vivaldi y Opera usan manifiestos
  `NativeMessagingHosts` y artefacto Chromium.
- Firefox Release/ESR necesita XPI firmado para instalación estable.
- Safari requiere una Safari Web Extension con app contenedora Apple,
  mensajería nativa propia, firma y notarización. No consume los manifiestos
  `NativeMessagingHosts` de Chrome/Firefox.

## Calidad

```bash
make test
go vet ./...
```

Para trabajo focalizado:

```bash
GOCACHE=/tmp/grxfirma-gocache go test ./internal/application ./cmd/grxfirma ./cmd/nativehost
```

Para la suite DSS:

```bash
go build -o /tmp/grxfirma-dssrunner ./cmd/dssrunner
GRXFIRMA_DSS_RUNNER=/tmp/grxfirma-dssrunner \
GRXFIRMA_DSS_EXPECTED_RESULT=TOTAL_PASSED \
make test-conformance-dss
```

La suite de conformidad exige `TOTAL_PASSED`. Para un diagnóstico que solo
confirme integridad criptográfica y reconocimiento del formato, el runner puede
invocarse directamente con
`--expect INTEGRITY_FORMAT_RECOGNIZED`; ese resultado no constituye evidencia
de conformidad ETSI.

## Empaquetado de suites

- Linux:
  - `./packaging/linux/build-suite.sh`
  - `./packaging/linux/build-suite.sh --deb`
  - la build Qt es fuera del árbol, el `.deb` declara dependencias ELF/QML
    calculadas y el bundle valida su runtime antes de instalar
- Windows:
  - `./packaging/windows/build-nativehost.sh`
  - `./packaging/windows/build-suite.sh`
  - `./packaging/windows/build-desktop-winui.sh`
  - `./packaging/windows/build-suite.sh --with-winui`
  - `./packaging/windows/build-suite.sh --with-winui --nsis`
  - `./packaging/windows/build-suite.sh --with-qt`
  - `./packaging/windows/build-suite.sh --with-winui --with-qt --nsis`
  - `./packaging/windows/build-suite.sh --with-qt --nsis`
- macOS:
  - `./packaging/macos/build-suite.sh`
  - `./packaging/macos/build-suite.sh --pkg`

Los scripts de empaquetado generan artefactos técnicos. Consulta
[publicación](docs/RELEASE.md) y
[firma de distribuciones](docs/RELEASE_SIGNING.md) antes de distribuirlos.

## Documentación útil

- [Índice de documentación](docs/INDICE.md)
- [Arquitectura](docs/ARCHITECTURE.md)
- [Puertos y adaptadores](docs/PORTS_AND_ADAPTERS.md)
- [Guía de operador](docs/operador.md)
- [Construcción y publicación](docs/RELEASE.md)
- [Seguridad](SECURITY.md)

## Licencia

El código propio de GrxFirma se distribuye bajo la
[EUPL 1.2 o posterior](LICENSE). Los componentes de terceros conservan sus
licencias, inventariadas en [legal/README.md](legal/README.md).
