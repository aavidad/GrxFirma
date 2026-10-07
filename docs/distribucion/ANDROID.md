<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Distribución Android

GrxFirma para Android se publica por tres canales: GitHub Releases (APK firmado
por el proyecto), F-Droid (APK compilado y firmado por F-Droid) y Google Play
(AAB firmado por el proyecto y redistribuido por Google). Todo lo que depende
del código ya está en el repositorio. Al responsable le queda crear la clave,
dar de alta los secretos, abrir las cuentas y subir.

| Canal | Qué se sube | Quién firma | Qué falta |
| --- | --- | --- | --- |
| GitHub Releases | APK de `release.yml` | Clave del proyecto | Clave y secretos ([CERTIFICADOS.md](CERTIFICADOS.md#c-clave-de-firma-de-android)) |
| F-Droid | Receta en `fdroiddata` | F-Droid | Etiqueta `vX.Y.Z` y merge request |
| Google Play | AAB de `release.yml` | Clave del proyecto (subida) y Google (distribución) | Cuenta, ficha, formularios y prueba cerrada |

Un móvil no puede actualizar una app instalada desde un canal con un APK de
otro canal firmado con otra clave: hay que desinstalar antes. GitHub y Google
Play pueden compartir firma (véase [Play App Signing](#3-play-app-signing));
F-Droid usa siempre la suya.

## Qué hace la app con la red, los permisos y los datos

Este apartado es la base de la ficha de seguridad de datos de Google Play, de
los antifeatures de F-Droid y de la [política de privacidad](../sitio/privacidad.html).
Se ha comprobado en el manifiesto, en `build.gradle.kts` y en el código Kotlin
de la versión 0.0.124.

### Permisos del manifiesto

| Permiso | Para qué | Cuándo se usa |
| --- | --- | --- |
| `android.permission.INTERNET` | Pedir fecha y hora certificadas a la TSA que configure el usuario; consultar OCSP o CRL para saber si un certificado está revocado; cotejar un QR tributario con la AEAT; buscar actualizaciones en GitHub | Solo cuando el usuario pulsa el botón correspondiente o activa la TSA en Preferencias. Firmar y verificar funciona sin conexión |
| `android.permission.NFC` | Leer el DNIe acercándolo al móvil | Solo al elegir «Usar mi DNIe». `android.hardware.nfc` va con `required="false"`, así que la app se instala también en móviles sin NFC |
| `…DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION` | Lo añade AndroidX automáticamente para proteger receptores internos | No es un permiso que vea el usuario ni da acceso a nada externo |

No pide cámara, almacenamiento, ubicación, contactos ni notificaciones. Los
documentos se abren y se guardan con el selector del sistema (SAF) y la app no
conserva el permiso sobre ellos entre sesiones. La foto del QR la hace la app
de cámara del sistema, que escribe en un fichero temporal privado mediante un
`FileProvider`; por eso no hace falta el permiso `CAMERA`.

El tráfico en claro está prohibido (`usesCleartextTraffic="false"` y
`network_security_config.xml`): todas las conexiones van por HTTPS con las
autoridades de confianza del sistema.

### Qué sale del móvil

| Acción del usuario | Destino | Qué se envía |
| --- | --- | --- |
| Firmar con fecha y hora certificadas | La TSA configurada | La huella (hash) de la firma, nunca el documento |
| Comprobar la revocación | El servicio OCSP o CRL que indique el certificado | Los datos que identifican el certificado (emisor y número de serie); una descarga de CRL no envía nada |
| Cotejar un QR con la AEAT | Agencia Tributaria | NIF del emisor, número, fecha e importe de la factura que figuran en el QR |
| Buscar actualizaciones | API de GitHub | Ningún dato propio; GitHub ve la IP como en cualquier conexión |

### Qué guarda en el móvil

Preferencias privadas (formato por defecto, perfil, URL de la TSA, nombre de
salida, tema, tiempo de cierre del certificado, idioma y diseño del sello),
excluidas de las copias de seguridad (`allowBackup="false"` y
`data_extraction_rules.xml`). No guarda contraseñas, PIN, claves, certificados
ni documentos. Los PDF temporales del editor del sello y la foto del QR van a
`cacheDir` y se borran al terminar. En la variante de producción las capturas
de pantalla están bloqueadas (`FLAG_SECURE`).

### Dependencias

El classpath de `productionRelease` solo contiene software libre: AndroidX,
Material Components, Kotlin y kotlinx-coroutines (Apache-2.0), jmulticard del
Gobierno de España (EUPL/GPL, para el DNIe) y BouncyCastle (MIT). No hay
Google Play Services, Firebase, Crashlytics, analítica ni publicidad. El
núcleo de firma es el AAR Go de `mobilebind`, que se compila desde el código
con gomobile. Para comprobarlo de nuevo:

```bash
grep -E 'productionReleaseRuntimeClasspath' mobile/android/app/gradle.lockfile \
  | cut -d: -f1,2 | sort -u
```

## 1. GitHub Releases

La variante distribuible es `productionRelease` de [mobile/android](../../mobile/android/README.md). El workflow [release.yml](../../.github/workflows/release.yml), al empujar una etiqueta `v*`, compila el AAR Go con `scripts/mobile/android/build-core-aar.sh`, lo valida por SHA-256, construye el APK y el AAB con `assembleProductionRelease bundleProductionRelease` y verifica con `verify_release_artifacts.py` la firma, la versión, el `versionCode`, el AAR y el commit de origen. Antes, el job `verify-android-reproducibility` compila dos veces con la clave QA y exige que salgan idénticos. El job `release-policy` detiene la release si falta cualquiera de los secretos de [RELEASE_SIGNING.md](../RELEASE_SIGNING.md#secretos-de-github), también los de otras plataformas.

El AAB firmado sale en el artefacto `android-installers` del workflow, junto al
APK y a `ANDROID-SIGNATURES.json`. No publiques el APK `verificationDebug`: no
hace operaciones criptográficas.

Para comprobar un APK descargado, usa `apksigner` de Android Build Tools y
coteja la huella SHA-256 del certificado con la publicada por un canal
independiente:

```bash
apksigner verify --verbose --print-certs GrxFirma-version-android.apk
```

La verificación debe terminar bien, con v2 y v3 válidas y la huella esperada.
Compara también el SHA-256 del archivo con `SHA256SUMS.txt` de la release y
verifica su firma OpenPGP según [RELEASE_SIGNING.md](../RELEASE_SIGNING.md).

## 2. F-Droid

### La receta

La receta está en [packaging/fdroid/io.github.aavidad.grxfirma.yml](../../packaging/fdroid/io.github.aavidad.grxfirma.yml)
y se copia tal cual a `metadata/io.github.aavidad.grxfirma.yml` de
[fdroiddata](https://gitlab.com/fdroid/fdroiddata). Hace esto:

- Descarga Go 1.26.8 de go.dev y comprueba su SHA-256 antes de usarlo. Instala
  la plataforma 36 y las Build Tools 36.0.0 del SDK y usa el NDK r28c
  (28.2.13676358), las mismas versiones que `install-toolchain.sh`.
- Compila el AAR desde el código con `build-core-aar.sh` (gomobile fijado por
  revisión) y después `assembleProductionRelease`. No usa ningún binario
  precompilado del proyecto.
- Borra antes del escaneo los `.syso` de Windows y `tests/certificates`, que
  no intervienen en Android. El escáner de F-Droid acepta el
  `gradle-wrapper.jar` oficial y la distribución de Gradle se descarga con su
  `distributionSha256Sum`.
- Con `GRXFIRMA_FDROID_BUILD=1`, Gradle genera el APK sin firma y se niega a
  usar credenciales externas. F-Droid lo firma con su clave.
- Declara el antifeature `NonFreeNet` por el cotejo opcional con la AEAT, un
  servicio no libre. Si quien revise opina que una consulta opcional a la
  Administración no lo merece, se puede quitar. Las demás conexiones (TSA,
  OCSP/CRL y GitHub) son a petición del usuario y no dependen de servicios
  propietarios.
- No repite nombre, resumen ni descripción: F-Droid los lee de
  `mobile/android/fastlane/metadata/android/` (la carpeta `fastlane` dentro del
  `subdir` de la compilación).

El `versionCode` se calcula en Gradle a partir de `VERSION.txt`
(`mayor*1000000 + menor*1000 + parche`): 0.0.124 da 124. Como no es un literal,
la receta lo lee con `UpdateCheckData` y la expresión `^0\.0\.(\d+)`, que solo
vale para 0.0.x. Cuando la versión pase a 0.1.0 o superior, hay que cambiar esa
línea en `fdroiddata` (por ejemplo, capturar el número completo desde un
fichero que lo contenga). Mientras no se cambie, checkupdates no propondrá
versiones nuevas, pero tampoco una equivocada.

Con `AutoUpdateMode: Version` y `UpdateCheckMode: Tags ^v\d+\.\d+\.\d+$`, el
robot de F-Droid detecta cada etiqueta nueva y añade la compilación sin
intervención. Por eso cada versión necesita su etiqueta `vX.Y.Z` en GitHub
sobre el commit de `VERSION.txt` correspondiente.

### Reproducibilidad comprobada

Con el código de 0.0.124 se hicieron dos compilaciones F-Droid sin firma
(`GRXFIRMA_FDROID_BUILD=1`, `SOURCE_DATE_EPOCH` del commit, `TZ=UTC`) y dos
compilaciones independientes del AAR. Los dos APK salieron idénticos byte a
byte y los dos AAR también (`dc8b0e6f…`). El APK contiene `versionCode=124`,
`versionName=0.0.124`, las ABI `arm64-v8a`, `armeabi-v7a` y `x86_64`, y no
lleva el bloque de dependencias cifrado de AGP (`includeInApk = false`).

Más adelante se puede pedir a F-Droid que publique el APK firmado por el
proyecto si su compilación coincide con la de GitHub. Para eso se añaden a la
receta `Binaries:` con la URL del APK de la release y `AllowedAPKSigningKeys:`
con la huella de [CERTIFICADOS.md](CERTIFICADOS.md#c2-huella-sha-256). No lo
actives hasta comprobar que F-Droid reproduce el APK oficial; si no coincide,
la compilación falla.

### Envío: merge request a fdroiddata

1. Comprueba que existe en GitHub la etiqueta `v0.0.124` (o la versión que
   toque) y que `VERSION.txt` dice lo mismo en ese commit.
2. Crea una cuenta en GitLab.com y haz un fork de
   <https://gitlab.com/fdroid/fdroiddata>.
3. En tu fork, crea una rama `io.github.aavidad.grxfirma` y copia la receta
   como `metadata/io.github.aavidad.grxfirma.yml`. Quita la cabecera de
   comentarios de licencia si el linter la rechaza.
4. Si tienes fdroidserver instalado, prueba en local antes de abrir la merge
   request:

   ```bash
   fdroid readmeta
   fdroid rewritemeta io.github.aavidad.grxfirma
   fdroid lint io.github.aavidad.grxfirma
   fdroid build -v -l io.github.aavidad.grxfirma
   ```

   `rewritemeta` reordena los campos al estilo de fdroiddata; acepta sus
   cambios. Si no lo tienes, la CI de la merge request ejecuta lo mismo.
5. Haz commit con el mensaje `New app: GrxFirma` y abre la merge request
   contra `master` con la plantilla «App inclusion». Marca las casillas de la
   plantilla y pega este texto:

   > GrxFirma signs and verifies electronic signatures (PAdES, CAdES, XAdES)
   > on the device, with a certificate file or the Spanish DNIe over NFC.
   > Source: https://github.com/aavidad/GrxFirma (EUPL-1.2). The Go core is
   > built from source with gomobile in the recipe; no prebuilt binaries are
   > used. All dependencies are free software (AndroidX, Material, Kotlin,
   > jmulticard, BouncyCastle); no Google Play Services or Firebase. Network
   > access is only used on user request (timestamping, OCSP/CRL, an optional
   > check against the Spanish Tax Agency, update check). Store metadata is in
   > `mobile/android/fastlane/`. The upstream author is the submitter.

6. Atiende los comentarios de quien revise. Lo normal es que pidan cambios de
   formato o algún ajuste del entorno de compilación. Cuando la fusionen, la
   app aparece en el repositorio principal en uno o dos ciclos de publicación
   (unos días).

## 3. Google Play

### 1. Cuenta de desarrollador

1. Entra en <https://play.google.com/console/signup> con la cuenta de Google
   que vaya a ser la del proyecto. Elige cuenta personal (la autoría es
   personal; una cuenta de organización exige número D-U-N-S).
2. Paga la cuota única (25 USD) y completa la verificación de identidad con el
   DNI. Puede tardar unos días.
3. Las cuentas personales nuevas tienen que hacer una prueba cerrada con al
   menos 12 personas durante 14 días seguidos antes de poder publicar en
   producción. Reúne esas personas desde el principio (bastan sus cuentas de
   Google en una lista de correo de Play Console).

### 2. Crear la app

En Play Console, «Crear aplicación»: nombre `GrxFirma`, idioma predeterminado
español (España) `es-ES`, tipo «Aplicación», gratuita. Acepta las
declaraciones. El ID de paquete queda fijado con la primera subida:
`io.github.aavidad.grxfirma`.

### 3. Play App Signing

Google firma lo que distribuye con la «clave de firma de apps» y tú le subes
el AAB firmado con la «clave de subida». `release.yml` firma el AAB con la
clave oficial del proyecto (`grxfirma-release.p12`).

Recomendación: en «Configuración de la app → Integridad de la app → Firma de
apps», elige usar tu propia clave y sube `grxfirma-release.p12` cifrada con la
herramienta PEPK que ofrece la propia consola. Así Google distribuye la app
con la misma firma que el APK de GitHub y el usuario puede pasar de un canal a
otro sin desinstalar. Puedes usar la misma clave como clave de subida (no hay
que tocar `release.yml`) o registrar otra; si registras otra, el AAB que sube
`release.yml` tiene que ir firmado con esa.

La alternativa es dejar que Google genere la clave de firma. Es más sencillo,
pero entonces Google Play y GitHub quedan como canales con firmas distintas.
Esta decisión no se puede deshacer después de la primera publicación.

### 4. Subir el AAB

1. Empuja la etiqueta `vX.Y.Z` y espera a que termine `release.yml`.
2. Descarga el artefacto `android-installers` del workflow y saca
   `GrxFirma-X.Y.Z-android.aab`.
3. En «Pruebas → Prueba cerrada», crea una versión, sube el AAB y pega como
   notas de la versión el contenido de
   `mobile/android/fastlane/metadata/android/<idioma>/changelogs/<versionCode>.txt`.
4. Cuando acabe la prueba cerrada, promociona la misma versión a producción.

El `versionCode` (124 para 0.0.124) sube con cada versión de `VERSION.txt`,
como pide Play.

### 5. Ficha de la tienda

Los textos y gráficos están en `mobile/android/fastlane/metadata/android/` en
diez idiomas: `es-ES`, `ca`, `gl-ES`, `eu-ES`, `en-US`, `fr-FR`, `de-DE`,
`it-IT`, `pt-PT` y `zh-CN`. Play no tiene valenciano como idioma de ficha; los
usuarios con el móvil en valenciano ven la ficha en catalán o en español, y la
app sí sale en valenciano.

| Campo de Play Console | Fichero |
| --- | --- |
| Nombre de la aplicación (30 caracteres) | `title.txt` |
| Descripción breve (80 caracteres) | `short_description.txt` |
| Descripción completa (4000 caracteres) | `full_description.txt` |
| Icono 512×512 | `en-US/images/icon.png` (también en `es-ES`) |
| Gráfico de funciones 1024×500 | `<idioma>/images/featureGraphic.png` |
| Capturas de teléfono | `<idioma>/images/phoneScreenshots/*.png` |
| Notas de la versión | `changelogs/<versionCode>.txt` |

Puedes copiar y pegar cada idioma en «Presencia en tienda → Ficha principal»
y añadir las traducciones con «Gestionar traducciones». Si prefieres
automatizarlo, `fastlane supply` lee esta misma estructura con una cuenta de
servicio de Play Console (la clave JSON de esa cuenta es un secreto y no debe
entrar en el repositorio).

Categoría: «Productividad». Datos de contacto: `avidad@dipgra.es` y el sitio
<https://aavidad.github.io/GrxFirma/>.

### 6. Política de privacidad

URL: <https://aavidad.github.io/GrxFirma/privacidad.html> (fuente en
[docs/sitio/privacidad.html](../sitio/privacidad.html)). Publica antes el
sitio con GitHub Pages. La política describe las cuatro conexiones de la tabla
«Qué sale del móvil».

### 7. Seguridad de los datos (Data safety)

Respuestas basadas en el apartado [Qué hace la app con la red, los permisos y
los datos](#qué-hace-la-app-con-la-red-los-permisos-y-los-datos):

- ¿Tu app recoge o comparte alguno de los tipos de datos de usuario
  obligatorios? **Sí**, solo por el cotejo con la AEAT. El desarrollador no
  recibe ningún dato: no hay servidor propio, cuenta, analítica ni informes de
  errores, y los documentos y las claves no salen del móvil. Pero Play llama
  «recogida» a cualquier dato que la app transmite fuera del dispositivo,
  aunque vaya a un tercero, y el cotejo envía el NIF, el número, la fecha y el
  importe de una factura. Lo que va a la TSA, a los servicios OCSP/CRL y a
  GitHub no son datos del usuario (la huella de una firma, el número de serie
  de un certificado público o nada).
- Tipo de dato: «Información financiera → Otra información financiera».
  - ¿Se recoge? Sí. ¿Se comparte? No: el envío lo pide expresamente el
    usuario, y Play no lo considera compartir.
  - ¿Se trata de forma efímera? Sí: la app no lo guarda.
  - ¿Es obligatorio? No, es opcional (el usuario decide pulsar «Cotejar con la
    AEAT»).
  - Finalidad: «Funcionalidad de la aplicación».
- ¿Se cifran todos los datos en tránsito? Sí: solo HTTPS, el tráfico en claro
  está bloqueado.
- ¿Pueden los usuarios pedir que se eliminen sus datos? La app no conserva
  nada que borrar; responde que los datos no se almacenan.

Si quien revise la ficha considera que el cotejo no es recogida, se puede
cambiar a «No recoge datos» sin tocar la app. Lo importante es que la ficha y
la política de privacidad digan lo mismo.

### 8. Resto de formularios de «Contenido de la aplicación»

- Acceso a la aplicación: todas las funciones están disponibles sin
  credenciales especiales. Para revisar la firma, quien revise necesita un
  certificado `.p12` de pruebas; si lo piden, entrégale uno sintético, nunca
  uno real.
- Anuncios: la app no contiene anuncios.
- Clasificación de contenido (cuestionario IARC): categoría «Utilidad,
  productividad, comunicación u otros»; responde «No» a violencia, sexo,
  lenguaje, drogas, juego, compras e interacción entre usuarios. Resultado
  esperado: PEGI 3 / Para todos.
- Público objetivo: mayores de 18 años (es una herramienta de firma con
  certificados personales); no está dirigida a menores.
- Aplicación de noticias: no. Aplicación del Gobierno: no (no se publica en
  nombre de una Administración). Funciones financieras: ninguna.
- Permisos sensibles: ninguno de los que exigen declaración (ubicación en
  segundo plano, SMS, registro de llamadas, accesibilidad, todos los
  ficheros).

### 9. Comprobaciones antes de enviar a revisión

- `targetSdk = 36` (Play exige al menos 35 en 2026).
- El AAB sale firmado con la clave registrada en Play; `release.yml` lo
  verifica con `GRXFIRMA_ANDROID_SIGNING_CERT_SHA256`.
- La política de privacidad está publicada y la URL responde.
- Hay al menos dos capturas de teléfono por idioma principal, el icono y el
  gráfico de funciones.
