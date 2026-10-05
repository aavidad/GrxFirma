<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Certificados de distribución: guía paso a paso

Esta guía sirve para crear las identidades de firma de Windows y Android y
guardarlas en GitHub. El workflow [release.yml](../../.github/workflows/release.yml)
ya sabe usarlas. Basta con crearlas y dar de alta los secretos y variables con
los nombres exactos que aparecen aquí.

Antes de empezar:

- Todo se guarda en el entorno protegido `official-release` (Settings →
  Environments). Si no existe, créalo con revisores obligatorios y limita el
  despliegue a las etiquetas `v*`. Los secretos van en el entorno.
- Las variables (huellas, alias, URL) van a nivel de repositorio (Settings →
  Secrets and variables → Actions → Variables). Los jobs que verifican no
  usan el entorno protegido y necesitan leerlas.
- Los ejemplos usan la CLI `gh` con sesión iniciada en el repositorio
  `aavidad/GrxFirma`. También se puede hacer desde la web de GitHub.
- No guardes claves, contraseñas ni ficheros `.p12`/`.pfx` en el repositorio,
  en el correo, en chats ni en carpetas compartidas sin cifrar.

Para Windows hay dos vías y basta con una: SignPath Foundation (apartado A) o
un certificado Authenticode propio en PFX (apartado B). El workflow elige la
vía según los secretos que encuentre. Android tiene su propia clave
(apartado C).

## A. Windows con SignPath Foundation

SignPath Foundation firma sin coste el software libre que acepta en su
programa. El certificado es de la fundación y está en su HSM, así que nadie del
proyecto tiene la clave privada. Windows mostrará «SignPath Foundation» como
editor, no «Alberto Avidad Fernández».

### A.1. Solicitud y condiciones

1. Publica el sitio con GitHub Pages y comprueba que se ve la
   [política de firma de código](../sitio/firma-de-codigo.html). SignPath exige
   una página pública con su mención, los roles y la nota de privacidad. Si el
   aprobador cambia, actualiza esa página antes de pedir firmas.
2. Rellena la solicitud en [signpath.org](https://signpath.org). Puedes pegar el
   texto de [SIGNPATH.md](SIGNPATH.md#texto-listo-para-pegar-en-la-solicitud)
   y dar como URL de la política
   `https://aavidad.github.io/GrxFirma/firma-de-codigo.html` (o la dirección
   real de Pages).
3. Lee y acepta las condiciones de SignPath Foundation. Las principales:
   licencia aprobada por la OSI (EUPL-1.2 lo es), repositorio público, binarios
   compilados en CI desde el código publicado, sin malware ni funciones
   ocultas, y una persona que apruebe cada firma.
4. Pregunta expresamente a SignPath si acepta firmar las bibliotecas de
   terceros que se distribuyen sin firma de su fabricante (las DLL de Qt, por
   ejemplo). El workflow las envía porque la verificación exige que todos los
   PE estén firmados. Si SignPath no lo acepta, avisa antes de la primera
   release: habrá que cambiar ese criterio en el workflow y en el verificador.

### A.2. Configuración en SignPath

Cuando SignPath apruebe la solicitud:

1. Comprueba que el proyecto tiene el slug `grxfirma` y la política de firma
   `release-signing`. Si SignPath asigna otros nombres, hay que cambiarlos en
   los dos pasos `signpath/github-action-submit-signing-request` de
   `release.yml`.
2. Vincula el repositorio `aavidad/GrxFirma` como sistema de compilación de
   confianza (GitHub.com) siguiendo las instrucciones de SignPath.
3. Crea dos configuraciones de artefacto con estos slugs. Cada una describe el
   ZIP que GitHub sube en una ronda.

   `windows-payload` (ronda 1: ejecutables, bibliotecas y desinstaladores):

   ```xml
   <?xml version="1.0" encoding="utf-8"?>
   <artifact-configuration xmlns="http://signpath.io/artifact-configuration/v1">
     <zip-file>
       <pe-file-set>
         <include path="payload/**/*.exe" max-matches="unbounded" />
         <include path="payload/**/*.dll" max-matches="unbounded" />
         <include path="uninstallers/*-uninstall.exe" min-matches="2" max-matches="2" />
         <for-each>
           <authenticode-sign />
         </for-each>
       </pe-file-set>
     </zip-file>
   </artifact-configuration>
   ```

   `windows-installers` (ronda 2: los dos instaladores):

   ```xml
   <?xml version="1.0" encoding="utf-8"?>
   <artifact-configuration xmlns="http://signpath.io/artifact-configuration/v1">
     <zip-file>
       <pe-file-set>
         <include path="*-setup.exe" min-matches="2" max-matches="2" />
         <for-each>
           <authenticode-sign />
         </for-each>
       </pe-file-set>
     </zip-file>
   </artifact-configuration>
   ```

   Si el editor de SignPath pide otra sintaxis, adáptala, pero cada fichero
   enviado debe volver firmado y no debe volver ningún otro. Si falta una
   firma, el workflow falla antes de publicar.
4. Crea un usuario de CI (o token de API) con permiso para enviar solicitudes
   al proyecto. Las aprobaciones las hace Alberto Avidad desde su cuenta.
5. Apunta la huella SHA-1 (40 hexadecimales) del certificado de SignPath
   Foundation. Aparece en los detalles del certificado en SignPath. También se
   puede sacar de cualquier fichero firmado por ellos, en PowerShell:

   ```powershell
   (Get-AuthenticodeSignature .\fichero-firmado.exe).SignerCertificate.Thumbprint
   ```

### A.3. Secretos y variables

| Dónde | Nombre | Valor |
| --- | --- | --- |
| Secreto del entorno `official-release` | `SIGNPATH_API_TOKEN` | Token de API del usuario de CI de SignPath |
| Secreto del entorno `official-release` | `SIGNPATH_ORGANIZATION_ID` | Identificador de la organización en SignPath |
| Variable del repositorio | `WINDOWS_SIGNING_CERT_THUMBPRINT` | Huella SHA-1 del certificado de SignPath Foundation |

```bash
gh secret set SIGNPATH_API_TOKEN --env official-release
gh secret set SIGNPATH_ORGANIZATION_ID --env official-release
gh variable set WINDOWS_SIGNING_CERT_THUMBPRINT --body "HUELLA_SHA1_SIN_ESPACIOS"
```

`gh secret set` sin `--body` pide el valor por teclado y así no queda en el
historial de la terminal.

### A.4. Cómo funciona en cada release

- Solo las etiquetas sin sufijo (`v0.0.118`) usan SignPath. Las de prueba
  (`v0.0.118-rc.1`) necesitan el PFX del apartado B. Si solo hay SignPath, la
  release de prueba falla en el primer paso y lo explica.
- Cada release pide dos firmas en SignPath, que hay que aprobar en su web: la
  ronda 1 (ejecutables, bibliotecas y desinstaladores) y la ronda 2 (los dos
  instaladores). Cada ronda espera hasta una hora.
- El workflow compara cada fichero devuelto con el enviado. Solo admite que se
  haya añadido la firma al final. Después verifica la huella, el algoritmo
  SHA-256 y el sello de tiempo RFC3161. Luego regenera `WINDOWS-SIGNATURES.json`
  y `SHA256SUMS-windows.txt`, y el job `verify-windows` lo repite todo en otro
  runner sin credenciales.
- SignPath Foundation renueva su certificado periódicamente. Cuando cambie,
  actualiza `WINDOWS_SIGNING_CERT_THUMBPRINT`; si no, la release falla al
  verificar.

## B. Windows con un certificado Authenticode propio (PFX)

Esta vía ya existía y sigue igual. Se activa cuando están los dos secretos del
PFX. Si también están los de SignPath, las etiquetas oficiales usan SignPath y
el PFX queda para las de prueba. En ese caso la huella fijada solo coincide con
uno de los dos certificados, así que lo más sencillo es configurar una sola
vía.

Requisitos del certificado:

- vigente, con clave privada y uso extendido «Firma de código» (Code Signing);
- emitido por una CA en la que confíe Windows;
- el PFX debe contener exactamente una identidad privada de firma de código.

Atención: desde junio de 2023 las CA públicas entregan las claves de firma de
código en hardware (token USB o HSM en la nube) y no permiten exportarlas a un
PFX. Si la CA entrega el certificado así, esta vía no sirve tal
cual. El workflow no tiene todavía una vía para tokens ni para servicios en la
nube como Azure Artifact Signing (antes Trusted Signing). Esos servicios rotan
el certificado a diario, y el verificador actual fija una huella concreta.
Habría que adaptarlo antes.

| Dónde | Nombre | Valor |
| --- | --- | --- |
| Secreto del entorno `official-release` | `WINDOWS_SIGNING_PFX_BASE64` | El fichero PFX en Base64, sin saltos de línea |
| Secreto del entorno `official-release` | `WINDOWS_SIGNING_PFX_PASSWORD` | Contraseña del PFX |
| Variable del repositorio | `WINDOWS_SIGNING_CERT_THUMBPRINT` | Huella SHA-1 del certificado (40 hexadecimales) |
| Variable del repositorio (opcional) | `WINDOWS_TIMESTAMP_URL` | Servidor RFC3161; si no se define, se usa `http://timestamp.digicert.com` |

En Linux o macOS:

```bash
base64 < certificado.pfx | tr -d '\n' | gh secret set WINDOWS_SIGNING_PFX_BASE64 --env official-release
gh secret set WINDOWS_SIGNING_PFX_PASSWORD --env official-release
openssl pkcs12 -in certificado.pfx -nokeys -clcerts | openssl x509 -noout -fingerprint -sha1
gh variable set WINDOWS_SIGNING_CERT_THUMBPRINT --body "HUELLA_SHA1_SIN_DOS_PUNTOS"
```

En PowerShell:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("certificado.pfx")) | gh secret set WINDOWS_SIGNING_PFX_BASE64 --env official-release
(Get-PfxCertificate -FilePath .\certificado.pfx).Thumbprint
```

`openssl` muestra la huella con dos puntos; quítalos antes de guardarla. Si el
PFX trae la cadena completa, quédate con la huella del certificado final, el
que tiene «Firma de código».

## C. Clave de firma de Android

La release oficial firma el APK y el AAB con una clave propia del proyecto. Es
la identidad de la app en el canal de GitHub Releases: todas las versiones
futuras deben firmarse con la misma. Si se pierde, los usuarios tendrán que
desinstalar la app para instalar una versión firmada con otra clave.

El workflow también necesita una clave QA distinta (C.4). Con ella compila dos
veces para comprobar que el resultado es reproducible. Esa clave nunca firma
nada que se publique.

### C.1. Crear la clave oficial

Hazlo en un equipo de confianza, con el disco cifrado y sin sincronización con
la nube en la carpeta de trabajo:

```bash
umask 077
keytool -genkeypair -v \
  -storetype PKCS12 \
  -keystore grxfirma-release.p12 \
  -alias grxfirma-release \
  -keyalg RSA -keysize 4096 -sigalg SHA256withRSA \
  -validity 10000 \
  -dname "CN=GrxFirma, O=Alberto Avidad Fernández, L=Granada, ST=Granada, C=ES"
```

- `keytool` pide la contraseña del almacén. Usa una larga y generada al azar.
- En PKCS12 la clave usa la misma contraseña que el almacén. Por eso
  `GRXFIRMA_ANDROID_KEYSTORE_PASSWORD` y `GRXFIRMA_ANDROID_KEY_PASSWORD` tienen
  el mismo valor.
- `-validity 10000` son unos 27 años. Google Play pide que la validez pase de
  2033.
- El alias puede ser otro; el workflow lo lee de la variable
  `GRXFIRMA_ANDROID_KEY_ALIAS`.

### C.2. Huella SHA-256

El workflow compara la huella SHA-256 del certificado (en DER) con la variable
`GRXFIRMA_ANDROID_SIGNING_CERT_SHA256`. Obtenla así (pide la contraseña):

```bash
keytool -exportcert -keystore grxfirma-release.p12 -alias grxfirma-release | sha256sum | cut -d' ' -f1
```

Sale en 64 hexadecimales en minúsculas y sin separadores, que es el formato
que espera la variable. La misma huella con dos puntos, para la web, se ve con:

```bash
keytool -list -v -keystore grxfirma-release.p12 -alias grxfirma-release | grep 'SHA256:'
```

Publica la huella en la web del proyecto y en un canal independiente de
GitHub, para que cualquiera pueda comprobarla con `apksigner` (véase
[ANDROID.md](ANDROID.md)). Para F-Droid: la receta actual deja que F-Droid firme
con su propia clave, así que no hace falta darle la huella. Si más adelante se
pide a F-Droid que publique el APK firmado por el proyecto (compilación
reproducible), el campo `AllowedAPKSigningKeys` de la receta lleva esta misma
huella en minúsculas y sin dos puntos.

### C.3. Secretos y variables oficiales

| Dónde | Nombre | Valor |
| --- | --- | --- |
| Secreto del entorno `official-release` | `ANDROID_SIGNING_KEYSTORE_BASE64` | `grxfirma-release.p12` en Base64, sin saltos de línea |
| Secreto del entorno `official-release` | `GRXFIRMA_ANDROID_KEYSTORE_PASSWORD` | Contraseña del almacén |
| Secreto del entorno `official-release` | `GRXFIRMA_ANDROID_KEY_PASSWORD` | La misma contraseña |
| Variable del repositorio | `GRXFIRMA_ANDROID_KEY_ALIAS` | `grxfirma-release` |
| Variable del repositorio | `GRXFIRMA_ANDROID_SIGNING_CERT_SHA256` | La huella de C.2 |

```bash
base64 < grxfirma-release.p12 | tr -d '\n' | gh secret set ANDROID_SIGNING_KEYSTORE_BASE64 --env official-release
gh secret set GRXFIRMA_ANDROID_KEYSTORE_PASSWORD --env official-release
gh secret set GRXFIRMA_ANDROID_KEY_PASSWORD --env official-release
gh variable set GRXFIRMA_ANDROID_KEY_ALIAS --body grxfirma-release
gh variable set GRXFIRMA_ANDROID_SIGNING_CERT_SHA256 --body "HUELLA_DE_C2"
```

La tubería evita dejar el Base64 en un fichero o en el portapapeles.

### C.4. Clave QA para la prueba de reproducibilidad

Créala con los mismos comandos y otro fichero y alias (por ejemplo
`grxfirma-qa.p12` y `grxfirma-qa`). Guárdala aparte de la oficial.

| Dónde | Nombre | Valor |
| --- | --- | --- |
| Secreto del entorno `official-release` | `ANDROID_QA_KEYSTORE_BASE64` | `grxfirma-qa.p12` en Base64 |
| Secreto del entorno `official-release` | `ANDROID_QA_KEYSTORE_PASSWORD` | Contraseña del almacén QA |
| Secreto del entorno `official-release` | `ANDROID_QA_KEY_PASSWORD` | La misma contraseña |
| Variable del repositorio | `ANDROID_QA_KEY_ALIAS` | `grxfirma-qa` |
| Variable del repositorio | `ANDROID_QA_SIGNING_CERT_SHA256` | Huella SHA-256 de la clave QA, calculada como en C.2 |

### C.5. Copia de seguridad

Guarda:

- el fichero `grxfirma-release.p12`;
- su contraseña y el alias;
- la huella SHA-256, para comprobar que una copia recuperada es la buena.

Haz al menos dos copias sin conexión en sitios distintos. Por ejemplo, dos
memorias USB cifradas, una en la caja fuerte del servicio y otra en otra sede.
La contraseña va por separado: en el gestor de contraseñas corporativo o en
sobre cerrado, nunca junto al fichero. Comprueba cada copia con el comando de
C.2 antes de borrar la de trabajo.

No la guardes en el repositorio ni en ningún otro repositorio, en el correo, en
chats, en carpetas compartidas o nubes sin cifrar, ni en el runner de CI.
GitHub solo guarda la copia cifrada del secreto y no deja descargarla, así que
tampoco cuenta como copia de seguridad.

Al terminar, borra la copia de trabajo y cualquier fichero intermedio:

```bash
shred -u grxfirma-release.p12 grxfirma-qa.p12
```

En discos SSD `shred` no garantiza el borrado físico. Por eso conviene trabajar
desde el principio en un volumen cifrado.

## D. Lanzar y comprobar una release oficial

### D.1. Lo que falta además de estos certificados

El job `release-policy` exige todas las credenciales oficiales antes de
compilar. Además de Windows y Android hacen falta las de macOS (Developer ID y
notarización), Firefox (AMO) y la clave OpenPGP de las sumas. La lista completa
y sus nombres están en [RELEASE_SIGNING.md](../RELEASE_SIGNING.md#secretos-de-github).
Si falta alguna, la release se para en el primer job y la nombra.

### D.2. Lanzarla

1. Prepara la versión con `scripts/nueva-version.sh` y completa
   `docs/NOVEDADES.md`. `VERSION.txt` debe coincidir con la etiqueta, sin la
   `v`.
2. Integra ese commit en `main`. El workflow rechaza etiquetas sobre commits
   que no estén en la rama principal.
3. Crea y sube la etiqueta:

   ```bash
   git tag -a v0.0.118 -m "GrxFirma 0.0.118"
   git push origin v0.0.118
   ```

4. Aprueba el despliegue en el entorno `official-release` cuando GitHub lo
   pida (pestaña Actions de la ejecución «Release v0.0.118»).
5. Con SignPath, aprueba en su web las dos solicitudes de firma cuando
   aparezcan. El resumen del job `release-policy` indica qué vía de firma
   Windows se ha elegido.

### D.3. Comprobarla

- Todos los jobs deben terminar en verde, en especial `verify-windows`,
  `verify-android` y `publish`. La release no se publica si falla cualquiera.
- Descarga todos los ficheros de la release en una carpeta y comprueba la
  firma OpenPGP y las sumas. `SHA256SUMS.txt` lista los ficheros con su
  subcarpeta (`windows/`, `linux/`...), mientras que GitHub los publica todos al
  mismo nivel; el `sed` quita esa subcarpeta:

  ```bash
  gpg --show-keys --with-fingerprint RELEASE-SIGNING-KEY.asc
  gpg --import RELEASE-SIGNING-KEY.asc
  gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt
  sed 's#  [^ ]*/#  #' SHA256SUMS.txt | sha256sum -c --ignore-missing
  ```

  La huella que muestra el primer comando debe coincidir con la publicada por
  un canal independiente.

- En Windows, comprueba firmas, sellos de tiempo y evidencias con el mismo
  verificador del workflow, desde un clon del repositorio en la etiqueta (lee
  la versión de `VERSION.txt`):

  ```powershell
  ./scripts/release/verify-windows-release.ps1 `
    -ArtifactDirectory .\descargas `
    -ExpectedThumbprint HUELLA_SHA1 `
    -RequireEvidence -RequireDualGui
  ```

  Para una comprobación rápida de un instalador:
  `Get-AuthenticodeSignature .\GrxFirma-0.0.118-windows-amd64-setup.exe | Format-List *`.
  `Status` debe ser `Valid` y `TimeStamperCertificate` no puede estar vacío.
- En Android: `apksigner verify --verbose --print-certs` sobre el APK. La
  huella SHA-256 debe coincidir con la de C.2.
- Instala la versión en un equipo limpio y mira el editor que muestra Windows
  al ejecutar el instalador.

Si una release falla después de crear la publicación en GitHub, no la
completes a mano. Retírala y vuelve a ejecutar el workflow, como explica
[RELEASE_SIGNING.md](../RELEASE_SIGNING.md).
