<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma

GrxFirma es una aplicación de escritorio y móvil para firmar y verificar
documentos con certificado electrónico. Reimplementa en Go, C# (WinUI 3),
Qt/QML y Kotlin la funcionalidad de AutoFirma, la aplicación Java del Gobierno
de España, y mantiene el protocolo `afirma://` para que los portales de la
Administración que ya lo usan sigan funcionando. Está pensada para cualquier
persona o entidad que firme en esas sedes, sin depender de una organización
concreta.

## Plataformas

- Windows 10 (1809) o posterior, x64: interfaz WinUI 3 e interfaz Qt como
  alternativa; instalador NSIS por usuario.
- Linux: interfaz Qt, paquete `.deb` y archivo `tar.gz`.
- Android 8.0 (API 26) o posterior: aplicación nativa en Kotlin.
- macOS e iOS: no hay versiones. No dispongo de un Mac ni de un iPhone para
  compilarlas y probarlas. Si quieres que existan, préstame o regálame uno y
  me pongo con ello 😄 (avidad@dipgra.es). En el repositorio queda código de
  partida sin validar.

## Funciones principales

- Firma y verificación CAdES, XAdES, PAdES (con sello visible), XMLdSig, ODF,
  OOXML, FacturaE y ASiC-XAdES.
- Cofirma guiada de PAdES, ODF y OOXML, y firma por lotes.
- Certificados del almacén del sistema, ficheros P12/PFX y tarjetas PKCS#11.
- Huellas (hash) de ficheros y carpetas, y protección de documentos con CMS.
- Integración con portales mediante `afirma://` y extensiones de navegador
  (Native Messaging).
- Interfaz en once idiomas, entre ellos castellano, catalán, euskera, gallego
  e inglés.

## Relación con AutoFirma

GrxFirma acepta las mismas peticiones `afirma://` que AutoFirma y produce los
formatos de firma que piden las sedes. Algunas decisiones cambian respecto a la
aplicación Java: no se mantiene abierto un servidor local mientras no hay una
firma en curso, no se cargan complementos de terceros dentro del proceso, las
contraseñas de proxy se guardan en el almacén seguro del sistema y en Windows
la interfaz es nativa. Los límites conocidos están en
[docs/LIMITES_Y_EXCLUSIONES.md](docs/LIMITES_Y_EXCLUSIONES.md).

## Estructura

El motor está en Go y sigue una arquitectura de puertos y adaptadores: el
dominio y los casos de uso no conocen la interfaz ni el sistema operativo.

| Carpeta | Contenido |
|---------|-----------|
| `internal/domain`, `internal/application`, `internal/ports` | Núcleo: modelo, casos de uso y contratos |
| `internal/adapters` | Adaptadores de entrada (CLI, REST local, `afirma://`, WebSocket) y de salida (almacenes de certificados, PKCS#11, ficheros) |
| `cmd/grxfirma` | Línea de órdenes y servidor REST local |
| `cmd/grxfirma-gui` | Proceso Go que atiende a las interfaces gráficas por IPC |
| `cmd/gui-winui` | Interfaz WinUI 3 (C#) para Windows |
| `cmd/gui-qml` | Interfaz Qt/QML (C++) para Linux y Windows |
| `cmd/grxfirmauri` | Manejador del protocolo `afirma://` |
| `cmd/nativehost` | Host de Native Messaging para las extensiones |
| `mobilebind` | Fachada del motor para móvil (`gomobile bind`) |
| `mobile/android`, `mobile/ios` | Aplicaciones móviles |
| `third_party` | Copias propias del lector y del firmador PDF |
| `packaging` | Empaquetado por plataforma y extensiones de navegador |
| `scripts` | Puertas de CI, cumplimiento y utilidades de publicación |
| `docs` | Arquitectura, decisiones (ADR) y documentación de uso |

## Compilación

Requisitos: Go 1.26 (`go.mod`), .NET SDK 10.0.302 (`global.json`, solo
WinUI), Qt 6 con `qmake6` (interfaz Qt), JDK 17 y SDK de Android con API 36
(Android). Las versiones exactas de cada dependencia están fijadas en el
repositorio.

Componentes Go por separado y suite de Linux (incluye Qt si encuentra
`qmake6`):

```bash
go build ./cmd/grxfirma ./cmd/grxfirma-gui ./cmd/grxfirmauri ./cmd/nativehost
packaging/linux/build-suite.sh --deb
```

Windows, desde PowerShell en el propio Windows:

```powershell
.\packaging\windows\build-desktop-winui.ps1
.\packaging\windows\build-desktop-qml.ps1
.\packaging\windows\build-suite.ps1 --with-winui --with-qt --nsis
```

Android: `scripts/mobile/android/build-core-aar.sh` genera el núcleo Go como
AAR y Gradle compila la aplicación en `mobile/android`; los pasos están en
[mobile/android/README.md](mobile/android/README.md). Para Linux y Windows hay
más detalle en [README_LINUX_SUITE](packaging/linux/README_LINUX_SUITE.md) y
[README_WINDOWS_SUITE](packaging/windows/README_WINDOWS_SUITE.md).

## Pruebas

```bash
go test ./...
go vet ./...
python3 -m unittest discover -s scripts/ci/tests
python3 -m unittest discover -s cmd/gui-qml/tests -p 'test_*.py'
```

WinUI se prueba en Windows con `dotnet test` sobre
`cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests`, y Android con
`scripts/mobile/android/validate-project.sh`.

## Decisiones principales

- Un único motor Go para todas las plataformas; cada interfaz habla con él por
  IPC (socket local o *named pipe* protegido en Windows) o, en móvil, mediante
  `gomobile bind` ([ADR-001](docs/ADR-001-mobile-tech.md),
  [ADR-005](docs/ADR-005-ipc-windows-named-pipe.md)).
- Ningún servidor local abierto por defecto en escritorio, y ninguno en móvil
  ([ADR-desktop-security](docs/ADR-desktop-security.md),
  [ADR-mobile-security](docs/ADR-mobile-security.md)).
- Sin complementos cargados en tiempo de ejecución; las ampliaciones se
  compilan y se revisan con la aplicación
  ([ADR-004](docs/ADR-004-extensibilidad-sin-plugins-runtime.md)).
- Firma PAdES sobre el PDF original, sin recrearlo
  ([ADR-pades-pdfsign-visible](docs/ADR-pades-pdfsign-visible.md)).
- Secretos en el almacén del sistema operativo, nunca en la configuración ni
  en los argumentos de la línea de órdenes
  ([ADR-003](docs/ADR-003-proxy-secrets-by-os.md)).
- Textos de interfaz en catálogos por idioma y compilaciones reproducibles.

La arquitectura completa está en [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
Las variables de entorno, la línea de órdenes, la protección CMS, las huellas
y la integración con navegadores se describen en
[docs/USO_AVANZADO.md](docs/USO_AVANZADO.md); el resto de la documentación, en
[docs/INDICE.md](docs/INDICE.md).

## Descargas

Las versiones publicadas están en
[GitHub Releases](https://github.com/aavidad/GrxFirma/releases) y en la web
del proyecto, <https://aavidad.github.io/GrxFirma/>.

## Licencia

El código propio de GrxFirma se distribuye bajo la
[EUPL 1.2 o posterior](LICENSE). Los componentes de terceros conservan sus
licencias, inventariadas en [legal/README.md](legal/README.md).

## Autor y contacto

Alberto Avidad Fernández, que trabaja en la Oficina de Software Libre de la
Diputación de Granada. Contacto: <avidad@dipgra.es>. Para avisar de un fallo
de seguridad, consulte [SECURITY.md](SECURITY.md).
