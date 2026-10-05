<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma Suite para Windows

Este paquete unifica los componentes principales de GrxFirma para Windows:

- CLI
- `nativehost` para navegadores
- handler `afirma://`
- interfaz nativa WinUI 3, recomendada cuando su stage se incluye en la suite
- interfaz Qt/QML, alternativa opcional que puede coexistir con WinUI

La entrega actual de Windows es `x64` y requiere Windows 10 1809
(`10.0.17763`) o posterior. La validación física se realiza sobre Windows 10
22H2 x64.

## Qué incluye

- `grxfirma.exe`
- `grxfirma-gui.exe` (launcher/backend IPC canónico y compartido)
- `grxfirma-nativehost.exe`
- `grxfirma-afirmauri.exe`
- `install-suite.ps1`
- `install-nativehost.ps1`
- `install-afirmauri.ps1`
- `afirmauri-registration.ps1`
- `uninstall-suite.ps1`
- `uninstall-nativehost.ps1`
- `uninstall-afirmauri.ps1`
- `install-desktop-qml.ps1`
- `uninstall-desktop-qml.ps1`
- `install-desktop-winui.ps1`
- `uninstall-desktop-winui.ps1`
- `remove-unselected-desktop.ps1`
- `install-path-safety.ps1`
- `invoke-uninstall-silent.ps1`
- `extensions/` con artefactos Firefox y Chromium
- `README_WINDOWS_SUITE.md`
- `VERSION.txt`

Si la suite se construye sobre una salida completa de `desktop-winui`, añade un
payload aislado `desktop-winui/` con:

- `app/grxfirma-winui.exe`;
- el runtime autocontenido de .NET y Windows App SDK;
- `app/grxfirma-gui.exe`, copia de validación del backend canónico;
- `PUBLISH-MANIFEST.sha256`.

Si se construye también sobre una salida completa de `desktop-qml`, añade un
payload aislado `desktop-qt/` con:

- `grxfirma-gui-qml.exe`
- `grxfirma-gui.exe` (copia de validación del backend canónico)
- `install-desktop-qml.ps1`
- `README_DESKTOP_QML_WINDOWS.md`
- recursos `qml/`, `assets/`, `help/` y runtime Qt redistribuible

Y, cuando se compila con NSIS:

- `GrxFirma-0.0.90-windows-amd64-setup.exe`

El paquete portable es `GrxFirma-0.0.90-windows-amd64.zip` y contiene la
carpeta raíz `GrxFirma-0.0.90-windows-amd64/`. Los nombres reflejan la versión
de `VERSION.txt` en cada compilación.

## Qué instala

La suite instala y registra:

- la CLI en una ruta local del usuario;
- el `nativehost` y sus manifiestos de `Native Messaging` para Chrome, Chromium, Edge, Brave, Vivaldi, Opera y Firefox;
- los artefactos de extension Firefox y Chromium;
- la extension de Firefox en perfiles existentes y, si hay permisos, en la
  distribucion global, solo cuando el XPI firmado coincide con su SHA-256;
- el registro de la ficha oficial de Chrome y Edge solo si están configurados
  los IDs publicados `GRXFIRMA_CHROMIUM_EXTENSION_ID` y
  `GRXFIRMA_EDGE_EXTENSION_ID`. El instalador usa las URL oficiales de tienda;
  cada navegador pide confirmación al usuario. No se genera ni instala un CRX
  propio. Al reinstalar, recupera los IDs válidos de su inventario;
- el protocolo `afirma://` para el usuario actual;
- la interfaz WinUI si va incluida y se selecciona;
- la interfaz Qt/QML si va incluida y se selecciona;
- scripts auxiliares de depuración.
- al desinstalar desde NSIS, también limpia los programas por usuario en
  `%LOCALAPPDATA%\Programs\GrxFirma`, las claves `HKCU` de `afirma://` y
  `Native Messaging`, y el registro externo de la extensión Chromium.

## Instalación manual

Desde PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-suite.ps1
```

## Instalación con NSIS

Si la entrega contiene el instalador:

```powershell
.\GrxFirma-0.0.90-windows-amd64-setup.exe
```

El instalador NSIS y los archivos de GrxFirma son por usuario: registra su
entrada de desinstalación en `HKCU` y deja la copia de mantenimiento en
`%LOCALAPPDATA%\Programs\GrxFirma\Suite`. Los componentes operativos viven
en las subcarpetas hermanas `CLI`, `AfirmaURI`, `NativeHost`,
`DesktopLauncher`, `DesktopWinUI` y `DesktopQML` de
`%LOCALAPPDATA%\Programs\GrxFirma`. La desinstalación conserva los datos del
usuario en `%LOCALAPPDATA%\GrxFirma` y la configuración en
`%APPDATA%\GrxFirma`.

Cuando los dos payloads están presentes, la página de componentes ofrece:

- motor, navegador y CLI, siempre obligatorio;
- WinUI 3, preseleccionada y recomendada para Windows 10;
- Qt/QML, opcional y desmarcada por defecto.

WinUI y Qt pueden instalarse simultáneamente. Cada una crea su propio acceso
directo, pero ambas arrancan el mismo backend Go instalado una sola vez en
`%LOCALAPPDATA%\Programs\GrxFirma\DesktopLauncher`. Las copias
`desktop-winui/app/grxfirma-gui.exe` y
`desktop-qt/grxfirma-gui.exe` existen en el paquete para comprobar
integridad; no se duplican en los directorios finales de las interfaces.

El modo silencioso (`setup.exe /S`) no muestra cuadros de diálogo invisibles:
instala los componentes seleccionados por defecto —incluida WinUI si está
presente, pero no Qt— y, si falla un proceso auxiliar, termina con código
`1603`. La entrada de
desinstalación publica además `QuietUninstallString` para gestores de software.
Ese comando copia temporalmente el desinstalador con una ACL exclusiva, propaga
su código de salida y limpia la copia sin borrados recursivos. La
desinstalación se detiene si el árbol de mantenimiento contiene un junction,
un enlace u otro reparse point.

Con un desktop Qt basado en MinGW, su runtime es app-local y no requiere
elevación. Si la Suite incluye el redistribuible oficial MSVC
`vc_redist.x64.exe`, Windows puede pedir UAC al ejecutarlo aunque los archivos y
registros de GrxFirma sigan siendo por usuario.

Las rutas de los seis componentes son fijas y se validan antes de copiar o
eliminar. Cada directorio lleva un marcador de instalación y se rechazan
reparse points; una ruta amplia o ajena no puede usarse como destino
destructivo. Los dos desktops y el launcher llevan además un marcador de
propiedad de la Suite.

Al volver a ejecutar el instalador, los componentes seleccionados se actualizan
y una interfaz desmarcada se retira solo si había sido gestionada por la Suite.
Una instalación manual o independiente sin ese marcador se conserva. El
desinstalador aplica la misma regla: elimina los desktops y el launcher
propiedad de la Suite, pero no adopta ni borra directorios ajenos. El
desinstalador se crea antes de instalar componentes opcionales y el marcador de
propiedad se escribe antes de copiar sus ficheros, de modo que un fallo parcial
se pueda limpiar y reintentar.

Las versiones anteriores de los NSIS instalaban la Suite y sus componentes por
equipo en `HKLM`. Los cuatro instaladores actuales revisan las vistas de
registro de 32 y 64 bits antes de modificar el sistema. Si detectan una Suite
antigua o la edición por equipo del componente que se intenta instalar, se
detienen: primero hay que retirarla desde **Configuración > Aplicaciones**
aceptando UAC, o ejecutar su `uninstall.exe` como administrador, y después
repetir la instalación por usuario.

## Avisos de nuevas versiones

WinUI y Qt/QML usan el backend compartido para consultar la última Release
estable de GitHub una vez al arrancar, salvo que el usuario desactive
**Configuración > Avisar de nuevas versiones**. También se puede comprobar
manualmente desde **Acerca de**. El backend necesita salida HTTPS a
`api.github.com:443`; el botón de la Release abre `github.com:443`.

La Suite solo muestra información y nunca descarga, instala ni ejecuta una
actualización. El canal de publicación debe tener una Release estable
accesible sin autenticación y etiquetada con la versión empaquetada. No se debe
distribuir un token de GitHub dentro de la aplicación. Un error de red o de
proxy no impide firmar y la comprobación manual indica cómo reintentar.

## Construcción

La entrega de usuario debe construirse y validarse en Windows real. Los
componentes Go pueden compilarse de forma cruzada, pero WinUI necesita
XAML/MSBuild/Windows SDK y los efectos GUI, registro e instalación no quedan
validados desde Linux.

Para construir la entrega recomendada con WinUI y mantener Qt como alternativa:

1. construye `desktop-winui`;
2. construye `desktop-qml` si quieres ofrecer la alternativa Qt;
3. construye la suite exigiendo las stages que deban formar parte de ella.

```powershell
.\packaging\windows\build-desktop-winui.ps1
.\packaging\windows\build-desktop-qml.ps1
.\packaging\windows\build-suite.ps1 --with-winui --with-qt --nsis
```

WinUI debe compilarse en Windows. Desde Git Bash, MSYS2, Cygwin o WSL, su script
`sh` delega la compilación en el Windows anfitrión. Una vez creadas ambas
stages, la construcción equivalente es:

```bash
./packaging/windows/build-desktop-winui.sh
./packaging/windows/build-desktop-qml.sh
./packaging/windows/build-suite.sh --with-winui --with-qt --nsis
```

`--with-winui` y `--with-qt` no activan una compilación implícita: exigen que
exista la stage completa correspondiente y fallan si falta. Sin esos flags, una
stage completa existente se integra automáticamente; si no existe, esa
interfaz se omite.

Con `--nsis`, los scripts PowerShell resuelven `makensis.exe` antes de compilar:
primero respetan `MAKENSIS`, después consultan `PATH` y por último las rutas
estándar `Program Files\NSIS`. Si no existe un ejecutable válido, fallan con un
diagnóstico accionable sin esperar a que terminen WinUI o Qt.

Las stages esperadas son:

```text
release/windows-desktop-winui/GrxFirma-0.0.90-desktop-winui-windows-amd64
release/windows-desktop-qml/GrxFirma-0.0.90-desktop-qml-windows-amd64
```

### Rutas largas al compilar NSIS en Windows

Al construir desde una ruta profunda —por ejemplo una snapshot bajo
`%LOCALAPPDATA%\Temp`— el payload autocontenido puede superar el límite de
ruta que todavía aplica el compilador NSIS a algunos ficheros. La ruta
PowerShell mide antes de invocar `makensis` la stage completa, el script y la
salida. A partir de 240 caracteres crea, solo durante esa invocación, un alias
de unidad `SUBST` sobre la raíz del repositorio:

- elige una letra realmente libre y comprueba que apunta al repositorio
  esperado;
- pasa a NSIS únicamente las rutas equivalentes bajo ese alias;
- vuelve a comprobar que ni siquiera las rutas relativas del payload superan
  el presupuesto preventivo;
- retira el alias en un bloque `finally`, también cuando `makensis` falla.

No copia ni renombra el payload, no crea junctions dentro de la entrega y la
letra temporal no queda embebida en el instalador. Si no existe una letra
libre, `SUBST` no está disponible, la raíz es UNC o la ruta relativa sigue
siendo excesiva, el build se detiene con un error explícito. En ese caso debe
usarse una copia local más corta; no se deben crear enlaces o junctions
manuales en las stages.

## Firma Authenticode

Sin firma, Windows SmartScreen puede mostrar "aplicación no reconocida" al
ejecutar el instalador. El script `build-suite.sh` firma automáticamente los
`.exe` y `.dll` elegibles y el instalador NSIS si se definen estas variables
(requiere `osslsigncode`, en Debian/Ubuntu
`apt-get install osslsigncode`):

```bash
export WINDOWS_SIGNCODE_PFX=/ruta/al/certificado.pfx
export WINDOWS_SIGNCODE_PASS_FILE=/ruta/al/fichero-password   # chmod 600
# opcional; por defecto http://timestamp.digicert.com
export WINDOWS_SIGNCODE_TS_URL=http://timestamp.digicert.com
./packaging/windows/build-suite.sh --with-winui --with-qt --nsis
```

Este mecanismo local produce artefactos firmados para pruebas o distribución
controlada, pero no sustituye el flujo oficial descrito en
[`docs/RELEASE_SIGNING.md`](../../docs/RELEASE_SIGNING.md). Sin el certificado
de firma de código oficial, su huella fijada y las restantes credenciales del
entorno protegido no se puede generar ni publicar una release oficial.

Notas:

- La password se lee de fichero (`-readpass`), nunca de argumentos, para que
  no quede expuesta en la lista de procesos (la misma política que en el resto de secretos).
- Cada binario se sella con RFC 3161: la firma sigue siendo válida cuando el
  certificado caduque.
- El launcher/backend Go canónico se firma una sola vez. Después se copian esos
  mismos bytes a los dos payloads y se regenera el manifiesto WinUI completo.
- `vc_redist.x64.exe` no se firma con la identidad del proyecto: conserva y
  debe superar la validación de su firma de proveedor.
- El gate de release verifica firma y sello de tiempo en todos los binarios PE,
  incluidos los runtimes; una DLL omitida o un manifiesto WinUI incoherente
  detiene la entrega.
- El certificado debe ser de tipo *code signing* (OV o EV) emitido a la
  entidad publicadora. Una firma válida identifica al editor, pero no garantiza
  por sí sola que SmartScreen omita todos los avisos: también intervienen la
  reputación y la política del equipo.
- Sin las variables, el build funciona igual pero sale **sin firmar** (útil
  exclusivamente como candidato técnico o de desarrollo).

### Alternativa gratuita: SignPath Foundation (proyectos open source)

[SignPath.io](https://signpath.io) ofrece firma de código gratuita a
proyectos open source a través de su fundación
([signpath.org](https://signpath.org)). Este proyecto cumple los requisitos
básicos: licencia de software libre (`EUPL-1.2`) y repositorio público con CI.

Cómo funciona y qué implica:

- La clave privada viviría en el HSM del servicio y la firma se integraría con
  GitHub Actions.
- **El publisher que ve el usuario en Windows es "SignPath Foundation"**, no
  "Alberto Avidad Fernández". Elimina el aviso de "editor desconocido", pero no
  muestra la identidad propia. Para un despliegue institucional oficial puede
  preferirse igualmente un certificado OV/EV propio (las variables
  `WINDOWS_SIGNCODE_*` de arriba quedan listas para ese caso).
- Exigen publicar una política de firma y usar builds trazables salidos del CI.
- La solicitud es un formulario en signpath.org y la revisa una persona;
  debe hacerla quien administra el proyecto.

El workflow oficial ya admite esta vía: si encuentra los secretos de SignPath,
las etiquetas sin sufijo de prueba se firman con SignPath en lugar del PFX.
Hace falta que SignPath apruebe el proyecto y fijar la huella de su
certificado. Véanse `docs/distribucion/SIGNPATH.md` y
`docs/distribucion/CERTIFICADOS.md`.

## Observaciones

- Los registros de `Native Messaging` y `afirma://` se escriben en `HKCU`.
- La CLI, el `nativehost` y el handler `afirma://` pueden compilarse cruzados
  para `amd64` desde Linux. WinUI y sus instaladores deben construirse y
  validarse también en Windows real.
- WinUI es `self-contained`: distribuye app-local los runtimes de .NET y
  Windows App SDK y no requiere instalar Qt. No significa ejecutable único ni
  compatibilidad con arquitecturas de 32 bits.
- El handler `afirma://` usa Fyne cuando dispone del soporte gráfico necesario
  y, en Windows x64, activa un fallback Win32 seguro cuando no hay OpenGL. El
  fallback admite `sign`, `cosign`, `countersign`, `selectcert`, `save`, `load`
  y `signandsave`; `batch` y cualquier operación no admitida fallan cerradas sin
  ejecutar la acción.
- La suite Qt/QML requiere un kit Qt 6 para Windows completo. El validador
  rechaza stages sin plugins obligatorios como `qmlsettingsplugin.dll`.
- Si el usuario arranca la REST local en `127.0.0.1:63118`, tiene también:
  - `/` como consola técnica web local para firma, verificación, certificados, diagnóstico y firma múltiple por `/sign-batch`;
  - `/signer` como firmador web local con sello visible y firma múltiple.
- El sello visible expuesto por esas superficies web soporta:
  - una página concreta;
  - rangos como `1,3-5`;
  - todas las páginas con `all`.
- WinUI expone el mismo alcance de páginas con preview PDF real, geometría y
  rotación configurables, guardas anti-TOCTOU y validación posterior a la
  firma. En la instalación Windows esa preview usa `Windows.Data.Pdf` y no
  necesita `pdfinfo`, `pdftoppm` ni Poppler. También ofrece cofirma múltiple
  guiada para `PAdES`, `ODF` y `OOXML`
  y `EncryptedData` con captura Win32 nativa, buffers borrables y transporte
  IPC binario `secretB64`. Certificados permite validar online y establecer o
  quitar el predeterminado. La firma por lotes WinUI permite selección
  múltiple/carpeta de primer nivel, carpeta de salida, cancelación y resumen
  por documento con resultados parciales, con límites de 128 documentos,
  100 MB por documento y 256 MB totales. Certificados permite cargar P12/PFX
  temporalmente o importarlo en un destino persistente anunciado por el
  backend, con diálogo Win32, buffers borrables y retirada/limpieza confirmada.
- Configuración WinUI crea, rota y retira credenciales de proxy mediante el
  almacén DPAPI del usuario. La entrada del password es transitoria y borrable;
  la configuración persistente conserva solo referencias opacas.
- Diagnóstico WinUI presenta las fases locales con icono y texto, inspecciona
  estados/recuentos de TLS sin rutas ni huellas y, solo en Windows, permite
  instalar o retirar con confirmación la confianza local inventariada por
  GrxFirma. No modifica raíces ajenas ni prueba una sede o `@firma` sin una
  operación real.
- WinUI es la opción recomendada en el instalador dual y Qt/QML sigue siendo
  opcional. La campaña Windows del 29-07-2026 instaló la Suite WinUI y cerró el
  recorrido físico de PAdES visible con el P12 oficial QA. Esto no implica
  paridad funcional completa: la accesibilidad manual, la localización
  runtime, el hardware/certificados de producción y los gates externos siguen
  pendientes.
- Los controles de servicio Windows no se anuncian: las acciones
  `service_status`, `service_install`, `service_start`, `service_stop` y
  `service_uninstall` carecen todavía de adaptador en esa plataforma.
- Si la suite integra `desktop-qml` y existe `cmd/gui-qml/help`, el paquete
  arrastra también `help/` para que `Abrir ayuda` pruebe antes el PDF del
  idioma seleccionado.
- El runtime reconoce estas variantes de ayuda PDF:
  - `help/ayuda-<locale>.pdf`
  - `help/ayuda-<lang>.pdf`
  - `help/ayuda.pdf`
  - `help/<locale>/ayuda.pdf`
  - `help/<lang>/ayuda.pdf`
- Se incluyen artefactos de extensión para Firefox y Chromium, aunque la instalación automática final en navegadores Chromium puede requerir distribución firmada o política corporativa.

Para QA local puede usarse el certificado sintético de
`scripts/windows-qa`: crea una identidad efímera no exportable únicamente en
`CurrentUser\My`, mantiene un inventario privado y retira la huella y su clave
exactas. No se añade a almacenes de confianza, no completa portales que exijan
una CA admitida y no sustituye Authenticode.

La Suite no es una release pública mientras falten la firma Authenticode y su
timestamp, el XPI Firefox firmado, el canal de tienda/política de Chrome y
Edge, y la repetición de las pruebas de portales sobre el artefacto candidato
exacto. La evidencia histórica de Chrome/Firefox no se hereda por compilar un
commit posterior.

## Reproducibilidad

Las rutas Bash y PowerShell construyen Go con dependencias de solo lectura, PGO
desactivado, `-trimpath`, metadatos VCS desactivados y `buildid` vacío, sin
eliminar el valor de `main.version`. Los ZIP se escriben con orden y timestamps
estables. NSIS no conserva los tiempos de los ficheros de entrada y todos los
empaquetadores reciben `SOURCE_DATE_EPOCH`, que por defecto es la fecha de
`HEAD` y puede fijarse explícitamente.

La igualdad byte a byte exige fijar las versiones de Go, Python/zlib o .NET,
Qt/windeployqt, NSIS y Windows SDK/makeappx. Los binarios y plugins Qt pueden
contener metadatos de su toolchain. Authenticode con servidor de sellado de
tiempo, la firma de Store y cualquier firma posterior cambian legítimamente el
artefacto y no son reproducibles bit a bit.

## Paquete MSIX (Microsoft Store)

El workflow [`msix.yml`](../../.github/workflows/msix.yml) construye la suite
completa (incluido el frontend Qt con kit MSVC y su runtime) y la empaqueta
como `GrxFirma-0.0.90-windows-amd64.msix` con `makeappx` en un runner
Windows. Se lanza a mano (`gh workflow run msix.yml`) o al etiquetar `v*`;
el `.msix` queda como artefacto del run.

Manifest (`packaging/windows/msix/AppxManifest.xml.in`):

- App principal: la GUI Qt/QML. Segunda `Application` oculta que registra el
  protocolo `afirma://` (firma web) vía `uap:Protocol`.
- `runFullTrust` (app Win32 clásica), Windows 10 1809+ (`10.0.17763`).

Para publicar en la Store:

1. Reservar el nombre siguiendo la
   [documentación oficial de Partner Center](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/reserve-your-apps-name)
   (cuenta de desarrollador del editor).
2. Copiar de ahí `Identity/Name` y `Publisher` (formato `CN=GUID`) y
   lanzar el workflow con esos valores (inputs `identity_name` y
   `publisher`). **Sin los valores reales del Partner Center la Store
   rechaza el paquete.**
3. Subir el `.msix` en la submission; **la Store firma el paquete** al
   publicarlo (no hace falta Authenticode propio para esta vía).

Notas:

- Para instalarlo localmente sin la Store (sideload) el MSIX debe ir
  firmado con un certificado en el que la máquina confíe; para pruebas es
  más cómodo el instalador NSIS de arriba.
- El registro del host de mensajería nativa de los navegadores requiere
  claves de registro que una app MSIX no escribe por sí sola: la
  integración con extensiones desde la versión Store puede requerir el
  paso manual documentado en el README del nativehost.

## Directivas de grupo (administradores)

La carpeta `policies` contiene la plantilla `GrxFirma.admx` y sus textos en
español (`es-ES`) e inglés (`en-US`). Cópiala al almacén central de directivas
del dominio o a `%WINDIR%\PolicyDefinitions` para configurar la aplicación por
GPO en *Configuración del equipo > Plantillas administrativas > GrxFirma*.
Los valores se describen en `docs/POLITICA_MAQUINA.md`.

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autoría: Alberto Avidad Fernández

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
