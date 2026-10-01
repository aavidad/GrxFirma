<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma Desktop WinUI para Windows

Esta infraestructura construye y valida la aplicación de escritorio WinUI 3
como un paquete `unpackaged`, `self-contained` y `x64`. El paquete incluye el
backend IPC Go y puede integrarse en el instalador NSIS de la Suite como
interfaz recomendada. Puede instalarse junto a Qt/QML, que permanece como
alternativa opcional y soportada. Ambas interfaces no declaran todavía paridad funcional
entre sí; su existencia y su integración no acreditan por sí solas el cierre
físico de ninguna de ellas.

`Self-contained` significa que los runtimes de .NET y Windows App SDK viajan
app-local con WinUI: el usuario no necesita instalar .NET, Windows App Runtime
ni Qt por separado. No significa un único ejecutable ni elimina la dependencia
del sistema operativo. La salida actual es únicamente para Windows `x64`.

## Toolchain fijada

- Windows 10 22H2 x64 o Windows 11 x64 para el host de compilación.
- .NET SDK `10.0.302`, fijado por `global.json`.
- Windows App SDK `2.3.1`, fijado por el proyecto y su lockfile.
- Windows SDK `10.0.26100`.
- Go según `go.mod`.

Instalación mínima del SDK .NET:

```powershell
winget install --id Microsoft.DotNet.SDK.10 --exact
dotnet new install Microsoft.WindowsAppSDK.WinUI.CSharp.Templates
```

WinUI no necesita Qt ni OpenGL. El SDK y los paquetes deben descargarse desde
sus canales oficiales. El proyecto debe versionar `packages.lock.json` junto al
`.csproj`; el empaquetador siempre restaura con `--locked-mode`. Los requisitos
gráficos de Qt/Fyne y el fallback Win32 del handler `afirma://` pertenecen a
componentes separados y no cambian este contrato.

El binario publicado apunta a Windows 10 1809 (`10.0.17763`) o posterior. La
compilación, publicación autocontenida y pruebas de `Core` se han ejecutado en
Windows 10 Pro 22H2 x64. En `894c751`, con el backend binario de `5db7991`,
pasaron 98 contratos Python, 68 pruebas Core en Windows 10, publicación
`self-contained`, compilación Go y validación de stage y ZIP técnico. El
29-07-2026 el estado posterior superó `100/100` pruebas Core, se integró e
instaló mediante la Suite NSIS y completó una firma PAdES con sello visible,
rotación y verificación independiente usando únicamente el certificado oficial
QA de la FNMT. Las pruebas accesibles, de hardware/certificados de producción y
de todas las operaciones reales siguen siendo gates separados.

## Construcción

Desde PowerShell en Windows:

```powershell
.\packaging\windows\build-desktop-winui.ps1
```

El script usa por defecto
`cmd/gui-winui/src/GrxFirma.WinUI/GrxFirma.WinUI.csproj`. Si el layout
cambia, hay que seleccionar explícitamente el proyecto de la aplicación:

```powershell
.\packaging\windows\build-desktop-winui.ps1 `
  -ProjectPath cmd/gui-winui/src/GrxFirma.WinUI/GrxFirma.WinUI.csproj
```

Desde Git Bash, MSYS2, Cygwin o WSL:

```bash
./packaging/windows/build-desktop-winui.sh
```

El `.sh` delega en `powershell.exe` del Windows anfitrión. No intenta
cross-compilar WinUI en Linux: el compilador XAML, MSBuild y el Windows SDK
deben ejecutarse en Windows.

## Integración e instalación con la Suite

Después de generar la stage WinUI, la Suite puede exigirla y crear el
instalador NSIS:

```powershell
.\packaging\windows\build-desktop-winui.ps1
.\packaging\windows\build-suite.ps1 --with-winui --nsis
```

Para ofrecer también Qt/QML como alternativa coexistente:

```powershell
.\packaging\windows\build-desktop-winui.ps1
.\packaging\windows\build-desktop-qml.ps1
.\packaging\windows\build-suite.ps1 --with-winui --with-qt --nsis
```

En la página de componentes, WinUI aparece preseleccionada y recomendada; Qt
aparece como opción adicional. Ambas pueden instalarse a la vez. La Suite crea
un acceso directo independiente por interfaz y conserva una única instalación
del backend Go en
`%LOCALAPPDATA%\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe`.

La copia `app/grxfirma-gui.exe` del payload WinUI solo sirve para validar
que frontend y Suite usan exactamente el mismo backend. No se copia dentro de
`DesktopWinUI`. Al actualizar, una interfaz desmarcada se retira únicamente si
lleva el marcador de propiedad de la Suite; una instalación independiente se
conserva. La desinstalación aplica el mismo límite y rechaza junctions, enlaces
o rutas no autorizadas.

## Artefactos

La salida se crea en `release/windows-desktop-winui/`:

```text
GrxFirma-0.0.90-desktop-winui-windows-amd64/
  app/
    grxfirma-winui.exe
    grxfirma-winui.dll
    grxfirma-winui.pri
    <runtime .NET y Windows App SDK>
    grxfirma-gui.exe
    help/
      guia-usuario.txt
  README_DESKTOP_WINUI_WINDOWS.md
  VERSION.txt
  PUBLISH-MANIFEST.sha256
GrxFirma-0.0.90-desktop-winui-windows-amd64.zip
SHA256SUMS.txt
ARTIFACTS.md
```

El directorio `app/` se mantiene aislado para que el instalador no mezcle las
bibliotecas autocontenidas con los ejecutables de native messaging ni con
otros componentes de la Suite.

La publicación incluye siempre `app/help/guia-usuario.txt`. La pantalla
**Ayuda** prioriza esta guía local de texto plano, que Windows 10 puede abrir
con su editor incluido sin conexión, lector PDF ni asociación para Markdown.
El validador rechaza el artefacto si la guía falta o está incompleta.

## Avisos de nuevas versiones

WinUI puede consultar una vez por arranque la última Release estable en GitHub.
El ajuste **Configuración > Avisar de nuevas versiones** queda habilitado por
defecto y se puede desactivar; **Acerca de > Comprobar actualizaciones**
permite repetir la consulta manualmente. Se requiere HTTPS a
`api.github.com:443` y, para abrir la página, a `github.com:443`.

El backend recibe la versión real mediante `main.version` en el empaquetado y
solo devuelve una comparación tipada. WinUI no descarga, instala ni ejecuta
artefactos. La Release debe ser pública y estable; no se distribuye un token
para acceder a un repositorio privado. Un fallo no bloquea la firma local.

## Contratos de seguridad e integridad

El empaquetador:

- exige exactamente .NET SDK `10.0.302` y rechaza SDK prerelease o roll-forward;
- exige `packages.lock.json` y usa restauración bloqueada;
- publica para `win-x64`, sin MSIX, single-file, trimming ni ReadyToRun;
- dirige `bin`, `obj`, cachés temporales y la publicación a un árbol de trabajo
  corto, exclusivo, estable y desechable bajo `%TEMP%`, evitando el límite
  histórico de rutas Windows al extraer paquetes WinUI;
- elimina el árbol temporal y cualquier `bin/obj` que el build haya intentado
  crear bajo `cmd/gui-winui`;
- crea el stage de forma atómica, después de validarlo;
- rechaza symlinks, junctions y otros reparse points;
- rechaza claves privadas, almacenes, `.env` y key logs TLS;
- comprueba que el apphost WinUI y el backend Go son PE AMD64;
- comprueba la presencia de `coreclr`, `hostfxr`, WinUI y Windows App Runtime;
- exige el índice `grxfirma-winui.pri` que permite cargar los recursos XAML
  en la aplicación sin empaquetar;
- comprueba en `deps.json` el RID `win-x64` y Windows App SDK `2.3.1`;
- genera un manifiesto SHA-256 de todos los ficheros publicados;
- normaliza los timestamps de metadatos y ordena las entradas del ZIP.

Antes de instalar WinUI, la Suite rechaza rutas absolutas o con `..`, entradas
ausentes, ficheros no inventariados y cualquier hash distinto del declarado en
`PUBLISH-MANIFEST.sha256`. También exige que la copia del backend incluida en
el payload sea idéntica al launcher compartido.

En la finalización oficial de Windows, el launcher canónico se firma con
Authenticode una sola vez. Después de firmar todos los demás PE, sus bytes se
replican en los payloads Qt y WinUI y se regenera el manifiesto WinUI completo.
El verificador independiente vuelve a comprobar inventario, hashes e igualdad
del backend. `build-desktop-winui.ps1` no maneja certificados por sí mismo.

### Límite de reproducibilidad de WinUI 3

En Windows App SDK `2.3.1`, el generador XAML/WinRT no produce actualmente una
`grxfirma-winui.dll` byte-idéntica entre dos compilaciones limpias. La
comparación controlada con el mismo SDK, lockfiles, ruta intermedia,
`SOURCE_DATE_EPOCH`, un solo nodo MSBuild y build servers desactivados aisló la
variación al orden interno de las clases `WinRTTypeDetails` generadas. En la
prueba realizada, 514 de las 516 entradas fueron byte-idénticas. Las otras dos
fueron la DLL y `PUBLISH-MANIFEST.sha256`, que registra el hash real de esa DLL.

Por tanto, el contrato actual garantiza dependencias bloqueadas, conjunto de
ficheros controlado, timestamps/orden ZIP estables e integridad SHA-256 completa
de cada build. No promete todavía un ZIP WinUI byte-idéntico. No se reescribe ni
normaliza la DLL después de compilar: eso ocultaría el comportamiento del
generador y alteraría un binario que más adelante debe firmarse.

Las variables de entorno cuyo nombre identifica contraseñas, tokens, secretos,
credenciales o key logs se retiran temporalmente durante la compilación. El
empaquetador WinUI aislado no acepta certificados, contraseñas ni opciones de
firma.

## Límites deliberados

La stage WinUI aislada no registra por sí sola `afirma://` ni Native Messaging,
no crea accesos directos y no genera MSIX. Es la Suite NSIS la que instala,
actualiza y desinstala WinUI junto con esos componentes.

WinUI es la interfaz recomendada de Windows y Qt/QML es una alternativa
opcional sobre el mismo backend Go. Ninguna debe presentarse como producto
publicado solo porque su stage exista. Firma ya conecta los modos simple,
cofirma y contrafirma,
preview y sello PDF visible con guardas de integridad y validación posterior;
la previsualización instalada usa `Windows.Data.Pdf.PdfDocument`, no requiere
Poppler y conserva el renderer IPC como alternativa;
la cofirma múltiple guiada conecta `sign_multicosign` para `PAdES`, `ODF` y
`OOXML`. La firma por lotes conecta `sign_batch`, combina varios ficheros con
una carpeta de primer nivel, exige una carpeta de salida y muestra el resultado
confirmado por documento, incluidos estados parciales. Aplica límites de 128
documentos, 100 MB por documento y 256 MB totales; al cancelar no convierte
elementos pendientes en éxitos.
Certificados conecta validación online, gestor de Windows y establecer/quitar
el predeterminado. Protección ya admite `EncryptedData` mediante diálogo Win32
nativo, buffers borrables y el campo IPC binario `secretB64`, sin persistencia
ni logging del secreto. Certificados permite usar P12/PFX solo durante la
sesión o importarlo en un destino persistente tipado por el backend. El
fichero y la contraseña se transportan como buffers `credentialB64` y
`passwordB64`, limitados a 2 MiB y 4 KiB; las copias controladas se borran y la
retirada individual o total de temporales exige confirmación. La localización
runtime y la validación visual y accesible continúan abiertas. El ZIP aislado
sigue siendo un artefacto técnico de desarrollo y QA; la vía de usuario es el
instalador de la Suite.

Configuración conecta el almacén seguro de proxy: la contraseña se obtiene con
el diálogo Win32, se transporta en un buffer borrable y el backend Windows la
guarda mediante DPAPI de usuario. `settings.json` no recibe usuario ni
contraseña en claro, solo metadatos opacos. Diagnóstico comprueba el estado del
almacén de proxy y el inventario TLS saneado. En Windows, y únicamente cuando
el motor publica el ciclo de vida gestionado, permite instalar y retirar con
confirmación la confianza TLS local inventariada por GrxFirma. La retirada
no recorre ni modifica certificados ajenos.

El backend Windows no anuncia las acciones `service_status`,
`service_install`, `service_start`, `service_stop` ni `service_uninstall`
porque no existe todavía un adaptador de servicio para esa plataforma. El
frontend no debe mostrarlas como capacidad disponible.

Windows 10 1809 es el mínimo técnico declarado. Para producción, el equipo debe
seguir recibiendo actualizaciones de seguridad, por ejemplo mediante una
edición LTSC aún soportada o el programa ESU que corresponda.

El arnés `scripts/windows-qa/Manage-GrxFirmaSyntheticSigningCertificate.ps1`
puede crear para QA una identidad RSA/SHA-256 efímera, no exportable y limitada
a `CurrentUser\My`, con inventario y borrado exacto de la clave. Sirve para
recorrer selección y firma local sin certificados personales. No instala una
raíz, no representa una identidad admitida por portales y no firma código.

Antes de publicar siguen siendo obligatorios y externos a este stage:
Authenticode SHA-256 con timestamp oficial; XPI Firefox firmado; distribución
de Chrome/Edge mediante tienda o política; y una campaña de navegadores,
portales y accesibilidad ligada al commit, instalador y hashes finales.
