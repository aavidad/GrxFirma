<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Firma y publicación de releases oficiales

Estado actual: el flujo y sus verificadores están implementados, pero no consta
en este expediente una ejecución oficial con certificado Authenticode,
timestamp, XPI Firefox firmado y canal gestionado Chrome/Edge sobre el candidato
final. Por tanto, los artefactos técnicos actuales no deben anunciarse como
release pública.

## Política

Todo tag `v*` activa un release oficial y el workflow falla de forma cerrada.
No se publica ningún asset si falta una credencial o si falla cualquiera de
estos controles:

- Authenticode y sello de tiempo de todos los instaladores y ejecutables
  distribuidos en Windows; los binarios propios exigen la huella oficial y las
  firmas válidas de proveedor se conservan;
- verificación independiente de los ZIP Windows en un segundo runner;
- Developer ID Application, Developer ID Installer, Hardened Runtime,
  Gatekeeper, notarización y ticket grapado del PKG macOS, repetidos después en
  un runner macOS sin acceso a claves privadas;
- checksums SHA-256 de Linux y del conjunto completo, ambos firmados con
  OpenPGP;
- coincidencia exacta entre tag SemVer y `VERSION.txt`;
- commit del tag ya integrado en la rama principal del repositorio;
- inventario, tamaños, hashes, SBOM y layout público esperado.

El workflow no mezcla assets con una GitHub Release preexistente para el mismo
tag. Si una publicación llegó a crearse, un reintento exige revisar y retirar
explícitamente aquella release antes de volver a ejecutar el proceso.

Los workflows de pruebas, compatibilidad y smoke no usan estas credenciales y
pueden seguir generando artefactos sin firma. Esos artefactos no son releases
oficiales ni deben redistribuirse como tales.

No existe un modo degradado de publicación Windows. Sin un certificado
Authenticode oficial vigente, su PFX, contraseña y huella fijada, el job de
release falla antes de publicar. Un instalador local sin firma o firmado con
una identidad de pruebas sigue siendo un candidato técnico, nunca la release
pública.

`scripts/release/build-official-release.sh` también falla siempre para impedir
que una agrupación local sin verificadores independientes se confunda con una
publicación oficial. `assemble_installers.sh` solo genera candidatos técnicos.

## Secretos de GitHub

Configurar estos secretos en el repositorio o en la organización:

| Secreto | Contenido |
| --- | --- |
| `WINDOWS_SIGNING_PFX_BASE64` | PKCS#12/PFX Base64 con una identidad privada válida para firma de código |
| `WINDOWS_SIGNING_PFX_PASSWORD` | Contraseña del PFX |
| `WEB_EXT_API_KEY` | Identificador de API JWT de addons.mozilla.org para firmar el XPI Firefox no listado |
| `WEB_EXT_API_SECRET` | Secreto de API JWT de addons.mozilla.org; solo se expone al job aislado que firma el XPI |
| `MACOS_APPLICATION_CERT_P12_BASE64` | P12 Base64 de `Developer ID Application` |
| `MACOS_APPLICATION_CERT_PASSWORD` | Contraseña del P12 de aplicación |
| `MACOS_INSTALLER_CERT_P12_BASE64` | P12 Base64 de `Developer ID Installer` |
| `MACOS_INSTALLER_CERT_PASSWORD` | Contraseña del P12 de instalador |
| `MACOS_NOTARY_KEY_P8_BASE64` | Clave API `.p8` de App Store Connect, en Base64 |
| `RELEASE_GPG_PRIVATE_KEY_BASE64` | Exportación de la clave privada OpenPGP dedicada, en Base64 |
| `RELEASE_GPG_PASSPHRASE` | Frase de paso de la clave OpenPGP |

No se deben almacenar certificados, claves, contraseñas ni perfiles de
notarización en el repositorio. El runner macOS usa un llavero efímero y lo
elimina en un paso `always()`. Windows valida primero el PFX en memoria con
`EphemeralKeySet`, importa temporalmente la identidad necesaria en
`CurrentUser\My` para `signtool` y la elimina, junto con su clave privada, en el
bloque de limpieza aunque falle la finalización.

El workflow oficial activa `GRXFIRMA_REQUIRE_SIGNED_FIREFOX_XPI=1` y usa
`web-ext@10.5.0` para obtener una única copia firmada por AMO. Linux, Windows y
macOS descargan ese mismo artefacto verificado antes de construir sus paquetes.
Si AMO no devuelve un XPI firmado que conserve la versión y el identificador
Gecko de las fuentes, ninguna plataforma se compila ni publica.

El workflow referencia el entorno GitHub `official-release`. Hay que crearlo
con revisores obligatorios, restringir el despliegue a tags protegidos y, de
forma preferente, guardar ahí los secretos anteriores. Sin esas reglas, el
nombre del entorno por sí solo no aporta separación de funciones.

## Variables de GitHub

Configurar también estas variables no secretas a nivel de repositorio u
organización, para que los runners verificadores sin entorno protegido puedan
leer las huellas esperadas:

| Variable | Valor esperado |
| --- | --- |
| `WINDOWS_SIGNING_CERT_THUMBPRINT` | Huella SHA-1 de 40 hexadecimales del certificado Authenticode |
| `WINDOWS_TIMESTAMP_URL` | URL HTTP de timestamp Authenticode; si está vacía se usa DigiCert |
| `MACOS_CODESIGN_IDENTITY` | Nombre completo de `Developer ID Application: ... (TEAMID)` |
| `MACOS_INSTALLER_IDENTITY` | Nombre completo de `Developer ID Installer: ... (TEAMID)` |
| `MACOS_TEAM_ID` | Apple Developer Team ID de 10 caracteres |
| `MACOS_NOTARY_KEY_ID` | Key ID de 10 caracteres de la clave API |
| `MACOS_NOTARY_ISSUER_ID` | Issuer UUID de App Store Connect |
| `RELEASE_GPG_FINGERPRINT` | Huella completa de 40 o 64 hexadecimales de la clave OpenPGP |

Las huellas son anclas de confianza y deben publicarse también por un canal
independiente del propio GitHub Release. Incluir la clave pública junto a una
firma no basta por sí solo para establecer confianza.

## Preparación del material

Ejemplo portable para convertir un fichero binario a Base64 sin saltos:

```bash
base64 < certificado.pfx | tr -d '\n'
```

En PowerShell:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("certificado.pfx"))
```

Requisitos mínimos de los certificados:

- Windows: certificado vigente, clave privada, EKU Code Signing y cadena de una
  CA que Windows confíe. La huella configurada debe coincidir exactamente.
- macOS: dos identidades vigentes, `Developer ID Application` y
  `Developer ID Installer`, del mismo Team ID.
- Apple Notary: clave API revocable con acceso suficiente para notarización.
- OpenPGP: clave dedicada a releases, protegida por frase de paso y con copia
  de recuperación custodiada fuera de GitHub.

## Contrato de firma de la Suite Windows dual

La Suite contiene un único launcher/backend Go canónico:
`grxfirma-gui.exe`. Los payloads WinUI y Qt incluyen inicialmente una copia
solo para validar que fueron construidos contra ese mismo motor. El finalizador
oficial:

1. detecta qué payloads completos existen y pasa `HAS_WINUI` y `HAS_QT` a NSIS;
2. excluye de la lista de firma las dos copias del backend y cualquier
   `vc_redist.*.exe`;
3. firma una sola vez el backend canónico y los demás PE propios;
4. conserva las firmas Authenticode válidas de proveedor y falla si son
   inválidas o carecen de timestamp;
5. después de terminar todas las firmas, replica los bytes exactos del backend
   canónico a los payloads WinUI y Qt;
6. regenera el inventario SHA-256 completo de WinUI y lo verifica antes de
   reconstruir el ZIP;
7. genera NSIS con los componentes detectados y firma también el desinstalador
   y el instalador final.

`vc_redist.x64.exe` conserva la identidad de Microsoft: no se vuelve a firmar
como si fuera un binario de GrxFirma. El verificador independiente abre los
ZIP, rechaza entradas inseguras, comprueba que las copias del backend sean
idénticas, valida `PUBLISH-MANIFEST.sha256` y verifica las firmas y timestamps
de todos los PE según corresponda.

Los contratos automatizados simulan el cambio de bytes que introduce
Authenticode para comprobar el orden de firma, las exclusiones y la
regeneración del manifiesto sin necesitar una clave real. Esa simulación no
sustituye la validación de una firma oficial, el servidor de timestamp ni
SmartScreen antes de publicar.

Apple exige firmar el software con Developer ID, activar Hardened Runtime,
usar timestamp seguro y notarizar la distribución. Referencias:

- [Notarizing macOS software before distribution](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)
- [Customizing the notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)

## Evidencias publicadas

Una release correcta incluye:

- `WINDOWS-SIGNATURES.json` y `SHA256SUMS-windows.txt`;
- `MACOS-NOTARIZATION.json` con el ID, estado aceptado por Apple y hash del PKG
  ya grapado;
- `SHA256SUMS-linux.txt` y `SHA256SUMS-linux.txt.asc`;
- `SHA256SUMS.txt` y `SHA256SUMS.txt.asc` para el conjunto completo;
- `RELEASE-SIGNING-KEY.asc`, `manifest.json`, `release-metadata.json` y
  `ARTIFACTS.md`;
- SBOM SPDX JSON.

En Windows, `WINDOWS-SIGNATURES.json` inventaría también los PE contenidos en
los ZIP. `SHA256SUMS-windows.txt` cubre los dos ZIP, los dos instaladores y el
propio manifiesto de firmas. Cualquier diferencia posterior obliga a
reconstruir y volver a firmar; no se corrige una release publicada modificando
la evidencia a mano.

El release macOS publica solo el `.pkg` firmado y notarizado. El `.tar.gz` que
puede generar el build de compatibilidad es un artefacto técnico intermedio y
no se publica como distribución oficial porque no recibe un ticket notarial
independiente.

La firma OpenPGP de Linux autentica artefactos standalone y sus checksums. No
convierte el `.deb` en un repositorio APT. Si se crea un repositorio APT, habrá
que firmar además `Release`/`InRelease` y gobernar por separado su clave.

## Verificación manual

Windows, en un host Windows:

```powershell
./scripts/release/verify-windows-release.ps1 `
  -ArtifactDirectory release/windows-official `
  -ExpectedThumbprint $env:WINDOWS_SIGNING_CERT_THUMBPRINT `
  -RequireEvidence `
  -RequireDualGui
```

macOS, en un host macOS:

```bash
./scripts/release/verify-macos-release.sh release/macos-suite
```

OpenPGP y SHA-256:

```bash
export RELEASE_GPG_FINGERPRINT=HUELLA_PUBLICADA_POR_CANAL_INDEPENDIENTE
./scripts/release/verify-signed-checksums.sh \
  release/installers \
  release/installers/SHA256SUMS.txt \
  release/installers/SHA256SUMS.txt.asc \
  release/installers/RELEASE-SIGNING-KEY.asc
```

## Rotación y revocación

- Rotar antes de la caducidad y probar una release candidata sin sustituir la
  huella oficial hasta completar la validación.
- Revocar inmediatamente credenciales expuestas y eliminar sus secretos de
  GitHub.
- Conservar los logs de notarización, ID de ejecución, commit, manifiesto,
  SBOM y huellas del release en el expediente de publicación.
- No reutilizar la clave OpenPGP de release para desarrollo diario.
