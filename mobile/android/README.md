<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma para Android

Proyecto Android nativo en Kotlin para seleccionar documentos mediante Storage
Access Framework, preparar certificados PKCS#12, firmar y verificar a través del
nucleo Go enlazado con `gomobile bind`.

## DNIe 3.0/4.0 por NFC

La opción «DNIe por NFC» usa `jmulticard`, `jmulticard-android` y
`jmulticard-jse` 2.0, con BouncyCastle 1.86. La conexión NFC oficial de
jmulticard y `BcCryptoHelper` realizan PACE con Brainpool sin registrar un
proveedor criptográfico global. Un adaptador usa `SecureRandom` para la
aleatoriedad de CWA-14890, ya que `BcCryptoHelper` 2.0 no siembra su generador.
El usuario introduce el CAN de seis cifras, acerca el DNIe, selecciona el
certificado de FIRMA y confirma con el PIN. La clave privada no sale de la
tarjeta. La app no guarda CAN ni PIN en almacenamiento; al salir de la pantalla se cierra la sesión
NFC. El núcleo Go recibe solo resúmenes y verifica cada firma RSA contra el
certificado antes de crear PAdES, CAdES o XAdES.

La resolución de dependencias, el `gradle.lockfile` y
`gradle/verification-metadata.xml` se completan fuera de este entorno sin red.
Después hay que reconstruir el AAR `gomobile` con el nuevo método
`installExternalIdentityJSON`, fijar su SHA-256 y compilar `productionDebug`.

Antes de distribuir, un operador debe probar con un móvil NFC y DNIe 3.0 y
4.0 reales: lectura de ambos certificados, elección de FIRMA, CAN incorrecto,
PIN incorrecto e intentos restantes, bloqueo, retirada durante lectura y
firma, certificado caducado, NFC desactivado y móvil sin NFC. Debe abrir y
verificar fuera de la aplicación las salidas PAdES, CAdES y XAdES, comprobar
que incluyen las CA intermedias disponibles y repetir el recorrido con
TalkBack, texto ampliado y rotación de pantalla. Hay que comprobar también
que el lector acepta la operación RSA sobre DigestInfo ya calculado; el
núcleo rechaza la salida si la firma no corresponde al certificado.

## Variantes

- `verificationDebug`: compila, ejecuta selectores SAF, valida estados y permite
  probar la interfaz sin afirmar que existe un backend criptografico.
- `productionDebug`: exige un AAR valido y su SHA-256 aprobado.
- `productionRelease`: ademas exige credenciales de firma Android fuera del
  repositorio. Es la unica variante apta para empaquetado oficial.

La variante de verificacion muestra el backend como no disponible y mantiene
deshabilitadas las operaciones criptograficas. No contiene un firmador simulado.

## Compilacion verificable

Requisitos: JDK 17, Android SDK Platform 36, Build Tools 36.0.0 y NDK
28.2.13676358. Se pueden instalar sin `sudo` con:

```bash
ACCEPT_ANDROID_SDK_LICENSES=1 scripts/mobile/android/install-toolchain.sh
```

El instalador no puede modificar el entorno de la terminal que lo invoca.
Después de instalar, aplique las rutas explícitamente, sin evaluar su salida:

```bash
export JAVA_HOME="$HOME/.cache/grxfirma-android/jdk-17.0.19+10"
export ANDROID_HOME="$HOME/.cache/grxfirma-android/android-sdk"
export ANDROID_SDK_ROOT="$ANDROID_HOME"
export PATH="$JAVA_HOME/bin:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$PATH"
```

En GitHub Actions, `--github-env "$GITHUB_ENV"` registra esas mismas rutas de
forma segura para los pasos posteriores; el script rechaza destinos simbólicos,
no regulares o valores con saltos de línea.

Para instalar además un emulador API 36 reproducible, añada
`INSTALL_ANDROID_EMULATOR=1`.

Con `JAVA_HOME`, `ANDROID_HOME` y `ANDROID_SDK_ROOT` configurados:

```bash
scripts/mobile/android/validate-project.sh
```

Con un emulador o dispositivo conectado, las pruebas instrumentadas se añaden
con `RUN_ANDROID_DEVICE_TESTS=1`. También se pueden ejecutar directamente con
`scripts/mobile/android/test-device.sh`.

Ese script valida `verificationDebug`. Para ejecutar el mismo arnés contra el
nucleo real, configure primero el AAR y su hash como se indica abajo y ejecute:

```bash
cd mobile/android
./gradlew --no-daemon --no-configuration-cache \
  connectedProductionDebugAndroidTest
```

Las expectativas son deliberadamente distintas por variante:
`verificationDebug` exige backend no disponible, mientras
`productionDebug` exige backend operativo y habilita verificación tras recibir
un documento por `ACTION_SEND`.

Para probar operaciones criptográficas reales con material sintético y
efímero, configure el AAR productivo y ejecute:

```bash
scripts/mobile/android/test-production-signing.sh
```

El arnés genera un PKCS#12 RSA de dos días sin datos personales, lo importa en
la aplicación, crea y verifica firmas PAdES, CAdES separada y XAdES, extrae las
evidencias y destruye la clave privada, el PKCS#12 y las fixtures privadas. Si
están disponibles, OpenSSL y `pdfsig` vuelven a comprobar CAdES y PAdES fuera
del núcleo Android.

El certificado oficial QA de Cliente @firma
`EIDAS CERTIFICADO PRUEBAS - 99999999R` también puede comprobarse sin
versionarlo:

```bash
export GRXFIRMA_ANDROID_OFFICIAL_TEST_P12=/ruta/al/P12-oficial-QA
export GRXFIRMA_ANDROID_OFFICIAL_TEST_PASSWORD_FILE=/ruta/al/secreto-QA-en-0600
scripts/mobile/android/test-official-certificate-signing.sh
```

El arnés solo acepta la huella SHA-256 del certificado oficial QA vigente
autorizado para la campaña. La contraseña no aparece en código, argumentos ni
registros: se entrega mediante un fichero regular `0600` (recomendado) o la
variable `GRXFIRMA_ANDROID_OFFICIAL_TEST_PASSWORD`, se copia en ficheros
temporales con permisos `0600`, el test la lee con un límite estricto y todas
las copias se eliminan al terminar. Además de la verificación del núcleo Android, OpenSSL,
`pdfsig`, `qpdf` y un proceso CLI separado comprueban, cuando están
disponibles, CAdES, PAdES y la integridad XAdES. La confianza se informa por
separado y no se deduce únicamente de que la firma criptográfica sea íntegra.

El APK verificable queda en
`app/build/outputs/apk/verification/debug/`. No debe distribuirse como version
operativa porque no firma ni verifica.

## Release oficial Android

`versionName` se obtiene de `VERSION.txt`; un
`GRXFIRMA_ANDROID_VERSION_NAME` explícito solo se admite si coincide
exactamente. `versionCode` se deriva como `major*1000000 + minor*1000 + patch`,
con posibilidad de fijarlo mediante `GRXFIRMA_ANDROID_VERSION_CODE` dentro
del rango Android.

La variante `productionRelease` exige un árbol Git limpio y estos valores
externos:

- AAR y `GRXFIRMA_ANDROID_CORE_SHA256`;
- `GRXFIRMA_ANDROID_SOURCE_COMMIT`, igual al `HEAD` completo;
- almacén, contraseñas y alias de firma Android;
- `GRXFIRMA_ANDROID_SIGNING_CERT_SHA256`, fijada fuera del repositorio.

El preflight exporta el certificado público del alias mediante `keytool`, sin
poner la contraseña en argumentos, y compara su huella. El APK incorpora como
metadatos el `sourceCommit` y SHA-256 del AAR realmente enlazado. El verificador
de artefactos comprueba de nuevo firma APK, firma AAB estricta, paquete, versión
y ambos metadatos. Además reconstruye el AAR en un runner sin credenciales,
reproduce el `llvm-strip --strip-unneeded` de Gradle con el NDK fijado
28.2.13676358 y compara byte a byte las `libgojni.so` resultantes de sus tres
ABI con las incluidas en el APK antes de generar `ANDROID-SIGNATURES.json`.

El APK oficial usa APK Signature Scheme v2 y v3 con RSA determinista. v1 queda
deshabilitado por obsoleto y v4 no forma parte del APK publicable. El gate
rechaza cualquier combinación distinta. También rechaza el bloque
`0x504b4453` de información de dependencias que AGP añade cifrado al APK: ese
bloque no interviene en la integridad v2/v3 y cambia entre construcciones. El
APK directo ya queda inventariado por el SBOM externo de la release. El AAB
conserva la información de dependencias para Google Play.

El workflow oficial central construye y publica un APK y un AAB junto a esa
evidencia. Antes reconstruye ambos dos veces con la misma clave QA fija
almacenada como secreto externo; los bytes deben coincidir. Las credenciales
oficiales y QA se crean en el directorio efímero del runner y se eliminan
siempre. Los artefactos Android entran en el manifiesto y checksums OpenPGP de
la misma GitHub Release que escritorio; no existe una publicación paralela.

Para reproducir localmente el gate con una identidad QA externa:

```bash
export GRXFIRMA_ANDROID_SOURCE_COMMIT="$(git rev-parse HEAD)"
export GRXFIRMA_ANDROID_KEYSTORE=/ruta/qa-release.p12
export GRXFIRMA_ANDROID_KEYSTORE_PASSWORD=...
export GRXFIRMA_ANDROID_KEY_ALIAS=...
export GRXFIRMA_ANDROID_KEY_PASSWORD=...
export GRXFIRMA_ANDROID_SIGNING_CERT_SHA256=<64-hex>
scripts/mobile/android/verify-release-reproducibility.sh
```

## Integracion del nucleo

El contrato binario obligatorio se define en [CORE_CONTRACT.md](CORE_CONTRACT.md).
El AAR se ubica por defecto en `app/core/grxfirma.aar` y nunca se versiona. Su
ruta y hash tambien se pueden proporcionar con:

```bash
export GRXFIRMA_ANDROID_CORE_AAR=/ruta/grxfirma.aar
export GRXFIRMA_ANDROID_CORE_SHA256=<64-hex>
```

`scripts/mobile/android/build-core-aar.sh` construye y valida el AAR desde el
commit actual, no desde cambios locales sin commit. Usa una revision fijada de
`golang.org/x/mobile`, ejecuta las pruebas de `mobilebind`, genera las tres ABI
y muestra el SHA-256 que debe aprobar Gradle. La compilacion se realiza sobre
un `git archive` de `HEAD` y desactiva el sellado VCS de Go dentro de esa copia,
ademas de eliminar del binario las rutas temporales con `-trimpath`; por ello
usa una ruta de trabajo estable derivada de los blobs Go de `HEAD` y el AAR es
reproducible byte a byte con Go 1.26 dentro del mismo entorno de herramientas,
aunque el origen sea un `worktree` o el commit solo cambie documentación. Dos
compilaciones simultaneas del mismo núcleo se rechazan para no mezclar sus
ficheros intermedios:

```bash
scripts/mobile/android/build-core-aar.sh /tmp/grxfirma.aar
export GRXFIRMA_ANDROID_CORE_AAR=/tmp/grxfirma.aar
export GRXFIRMA_ANDROID_CORE_SHA256=$(sha256sum /tmp/grxfirma.aar | cut -d' ' -f1)
cd mobile/android
./gradlew --no-daemon --no-configuration-cache assembleProductionDebug
```

El flujo local es real: importacion PKCS#12, catalogo y seleccion de la
identidad de sesion, firma CAdES/PAdES (RSA o ECDSA), XAdES (RSA), verificacion
y guardado SAF. Al verificar una firma separada CAdES, la interfaz permite
seleccionar el documento original opcional que exige el verificador. No se
habilitan flujos remotos, biometria ni persistencia de la clave. Solo se
conserva una identidad en memoria. La acción visible `Olvidar certificado`,
descartar el resultado sin guardarlo y el cierre del modelo de pantalla llaman
a `clearSession()`; terminar el proceso también elimina el estado por diseño.
No se usa ni se afirma Android Keystore o hardware TEE en este flujo PKCS#12.

Para un PDF, «Firma visible» abre un editor que dibuja la página con
`PdfRenderer`. El sello se mueve, redimensiona y gira con gestos o botones
accesibles; el PNG de vista previa procede del mismo compositor Go que firma.
Se puede aplicar a una página o a todas (hasta 128), elegir opacidad, QR HTTPS,
texto/color, logo institucional o una imagen PNG/JPEG de hasta 2 MiB. Las preferencias de
geometría y estilo se guardan sin certificado ni documento; la imagen elegida
se copia a `noBackupFilesDir`. El AAR expone `sealPreviewJSON` y la firma recibe
las opciones PAdES existentes del motor.

La prueba instrumentada específica de sello visible reutiliza el certificado
sintético efímero del arnés productivo; firma a 30° con opacidad del 50 %, abre
el PDF firmado con `PdfRenderer`, verifica su integridad en el núcleo y ejecuta
`pdfsig` en el anfitrión cuando está instalado:

```bash
scripts/mobile/android/test-visible-seal.sh
```

El PKCS#12 cruza JNI como un buffer mutable, sin la copia Base64 que existía en
el contrato inicial, y se sobrescribe en ambos lados cuando deja de usarse. La
contraseña se recoge como `CharArray`, se codifica directamente a bytes UTF-8 y
cruza JNI como buffer mutable con `importCertificateSecretBytesJSON`. Se limpia
en ambos lados, incluso en errores y cancelaciones. La biblioteca PKCS#12 Go
aún necesita una conversión interna a `string`, cuyo borrado no puede garantizarse.

PAdES usa temporalmente un subdirectorio privado con permisos `0700` dentro de
`noBackupFilesDir`; los archivos de trabajo se intentan eliminar al terminar y
la fachada vuelve a limpiar ese directorio al iniciarse y al borrar la sesion.
La integridad de firma se verifica localmente. Como el nucleo mobile no integra
todavia las anclas de confianza del sistema, la pantalla separa integridad,
vigencia del certificado, confianza y revocación. Nunca presenta una validez
global cuando la confianza es desconocida; la revocación durante la verificación se limita a evidencias
embebidas. Los perfiles LT/LTA de firma pueden obtener evidencias por red con
el proveedor seguro de escritorio; esa obtención no activa la verificación en línea.

La app solicita permisos SAF transitorios de lectura/escritura y no los
persiste. Al arrancar libera permisos persistentes que pudieran quedar de
versiones anteriores. Mientras hay una operación o un resultado pendiente de
guardar, se rechazan nuevos intents y cambios de documento para no mezclar
sesiones.

## Controles de seguridad

- solo permisos NFC e `INTERNET`; este último permite la TSA y obtener evidencias LT/LTA, sin permisos de almacenamiento amplios;
- la TSA solo la configura la persona usuaria. Se valida sin credenciales ni fragmento y admite HTTP además de HTTPS por compatibilidad con TSA públicas que se usan por HTTP, como la de la FNMT, y con TSA internas de la organización. Antes de incorporar la respuesta RFC 3161 se comprueban su firma, la huella y el nonce;
- `usesCleartextTraffic=false` (configuración de seguridad de red de Android) solo afecta a las bibliotecas de red de Java y Kotlin. El núcleo Go abre sus propias conexiones y no lo tiene en cuenta: la TSA en HTTP y las consultas de revocación (OCSP y CRL, normalmente en HTTP) salen por él. Las reglas de esquema, credenciales y redirecciones (la TSA no puede redirigir a otro origen) las aplica el núcleo, no esa opción de Android;
- copias de seguridad y transferencia de datos deshabilitadas;
- documentos abiertos exclusivamente mediante URI `content://`;
- permisos SAF de alcance transitorio, sin retención entre sesiones;
- limite de 32 MiB para documentos y 4 MiB para PKCS#12;
- contrasenas y bytes sensibles no se guardan en preferencias ni estado;
- identidad PKCS#12 limitada a una por sesion, con borrado explícito desde la UI;
- resultados de firma limitados a 48 MiB y borrados de memoria al guardarlos;
- errores del nucleo saneados antes de cruzar el enlace JNI;
- ayuda local en los once idiomas de escritorio, estados de verificación separados y
  encabezados accesibles para lector de pantalla;
- `FLAG_SECURE` activo en producción para impedir capturas del documento,
  certificados, contraseña y resultados;
- release bloqueada sin AAR fijado por hash y firma externa.

## Primera oleada de paridad Android

La interfaz incluye operación de firma, cofirma o contrafirma, perfiles B/T/LT/LTA
y configuración TSA. Se propone cofirma cuando el motor detecta firmas en el
documento. El formato automático conserva el formato detectado al cofirmar.
CAdES admite los cuatro perfiles; PAdES B/T/LT y firma/cofirma; XAdES B/T.
Las combinaciones restantes se rechazan con una explicación, sin bajar de nivel.

Cada salida se verifica antes de pedir destino de guardado. El dictamen se
conserva al guardar y distingue integridad, vigencia, confianza y revocación.
La pantalla muestra firmantes, emisor, huella, cobertura y evidencias. El botón
«Exportar informe JSON de verificación» abre `ACTION_CREATE_DOCUMENT` con
`application/json`; cancelar o fallar permite reintentar sin repetir la firma.

El selector propio permite seguir al sistema o elegir es, en, ca, valencià,
gl, eu, fr, de, it, pt o zh. Se usa `AppCompatDelegate.setApplicationLocales`,
con persistencia de AppCompat en versiones antiguas y el gestor del sistema en
Android 13 y posteriores. Valencià usa `ca-ES-valencia` y recursos
`values-b+ca+ES+valencia`. Los textos de interfaz y los parámetros están en los
once catálogos Android; los diagnósticos del motor que coinciden con el catálogo
de escritorio usan un subconjunto empaquetado en `assets/locales`.
Los datos de identidad y los detalles técnicos sin traducción coincidente se
muestran literalmente; el JSON exportado conserva el informe del motor.

«Acerca de» muestra versiones de app y motor, EUPL 1.2 o posterior, el contacto
avidad@dipgra.es, ayuda y las novedades locales de esta oleada. Para producción
hay que reconstruir el AAR v2 y fijar su nuevo SHA-256; no se ha generado ni
publicado un APK con estos cambios.

## Segunda oleada (parte A): lote, huellas y protección

La sección plegable «Más herramientas» añade tres grupos:

- **Firmar varios ficheros**: `ACTION_OPEN_DOCUMENT` con
  `EXTRA_ALLOW_MULTIPLE` (hasta 16 ficheros y 32 MiB en total). Se firman con
  `processBatchJSON`, una sola aprobación y el formato, perfil y TSA elegidos;
  sin sello visible y siempre con la operación «firma». El resultado se muestra
  por fichero y las firmas se guardan en una carpeta elegida con
  `ACTION_OPEN_DOCUMENT_TREE`, sin persistir el permiso. Si alguna no se puede
  guardar, sigue en memoria para elegir otra carpeta. El DNIe pide PIN en cada
  firma y no se ofrece para el lote.
- **Huella de un fichero**: SHA-256, SHA-1, SHA-384 o SHA-512 en hexadecimal
  (`.hexhash`), Base64 (`.hashb64`) o binario (`.hash`), con el mismo contenido
  que guarda el escritorio. La comprobación lee el fichero de huella (máximo
  4 KiB) y deduce formato y algoritmo como el escritorio.
- **Proteger o desproteger**: CMS EnvelopedData, AuthEnvelopedData o
  EncryptedData. Los destinatarios son certificados públicos elegidos con SAF
  (máximo 16, 64 KiB cada uno) y, si se marca, el certificado de la sesión;
  Android no guarda libreta de destinatarios. EncryptedData usa una clave
  AES-256 en Base64 canónico que se escribe dos veces o se genera con
  `SecureRandom`; viaja como `CharArray`/`byte[]` y se borra tras usarse.
  «Proteger y firmar» crea SignedAndEnvelopedData con el PKCS#12 de la sesión.
  Desproteger usa la clave RSA del PKCS#12 importado o la clave de
  EncryptedData; el DNIe no expone descifrado.

Descartar un resultado de estas herramientas no olvida el certificado de la
sesión; descartar una firma sí, como antes. Los campos de clave excluyen
autocompletado y no guardan estado.

Pendiente: sello visible y cofirma en el lote, DNIe en lote y en proteger y
firmar, perfil alto (ML-KEM) y libreta persistente de destinatarios, huellas de
carpetas completas y abrir SignedAndEnvelopedData de firmantes cuya cadena no
alcance las anclas del sistema.

## Segunda oleada (parte B): formatos, Veri*Factu, ENI y leyenda CSV

- **Formatos de escritorio**: el desplegable añade XMLdSig, ODF, OOXML,
  FacturaE, ASiC-XAdES y «Registro Veri*Factu» cuando el AAR los declara en
  `signing.formats`. Firman con el motor de escritorio, solo con RSA y solo en
  perfil B (sin TSA). XMLdSig, ODF y OOXML admiten cofirma. «Automático» sigue
  las reglas de escritorio por extensión y añade el MIME de SAF y la raíz
  `Facturae` del XML. Veri*Factu se elige a mano, como en escritorio, y no se
  firma en lote.
- **Leyenda CSV y sello por página**: el editor del sello permite poner el
  sello en varias páginas, cada una con su posición y giro, y estampar la
  leyenda CSV (código, URL HTTPS de cotejo con `{csv}`, texto opcional y QR).
  `csvLegendJSON` valida el código y normaliza la URL con el IDN del motor
  antes de aceptar el editor. El código CSV no se guarda en preferencias y se
  olvida al cambiar de documento.
- **Veri*Factu y ENI**: sección plegable nueva. La comprobación de registros
  admite hasta 64 XML (10 MiB cada uno, 32 MiB en total) y muestra, por
  registro, la huella calculada y cada incidencia con la clave del motor. No
  consulta a la AEAT. El documento ENI se crea con la firma elegida como
  documento (y el original si la firma es separada), órganos DIR3, origen,
  estado de elaboración y tipo documental en desplegables con los códigos NTI
  de `eniCatalogsJSON`, y fecha de captura con `MaterialDatePicker` (sin
  fechas futuras). Se guarda por SAF como resultado de herramienta. La
  comprobación de un ENI revisa la estructura, no las firmas.

Las claves `verifactu.*`, `eni.validacion.*`, `eni.codigo.*` y `csv.error.*`
se traducen con los catálogos empaquetados en `assets/locales`, generados por
clave desde los de escritorio.

Pendiente: expediente ENI (carpeta de documentos con índice firmado),
exportar el informe Veri*Factu a un fichero, leer el QR tributario y la
consulta a la AEAT, perfiles T para los formatos nuevos y sello visible en el
lote. La revisión de usabilidad independiente y la prueba en dispositivo de
esta parte siguen pendientes.

Comprobaciones locales adicionales:

```bash
python3 scripts/mobile/android/check_locales.py
python3 scripts/mobile/android/sync_verification_locales.py
GOFLAGS=-buildvcs=false GOCACHE=/tmp/codex-and1-gocache go test ./mobilebind/...
```

`check_locales.py` comprueba cobertura, duplicados, parámetros y referencias.
`sync_verification_locales.py` actualiza los diagnósticos empaquetados desde el
catálogo desktop y los recursos Android. Las pruebas Kotlin de estado, opciones,
JSON, contraseñas y ViewModel están en `src/test`; los recorridos de guardado,
verificación posterior, exportación y selección de idioma están en `src/androidTest`.

## Cuarta oleada: expediente ENI, lote con sello y cofirma, DNIe

- **Expediente ENI** (en «Veri*Factu y ENI»): se elige con
  `ACTION_OPEN_DOCUMENT_TREE` la carpeta de documentos ENI, sin persistir el
  permiso. Entran los ficheros XML (hasta 64 y 32 MiB en total; se examinan como
  mucho 512 entradas), en orden alfabético como en escritorio; el resto se
  cuenta y se deja fuera. Se piden órganos DIR3, clasificación, estado
  (E01-E03 con su descripción del catálogo), identificador e interesados
  opcionales y fecha de apertura con `MaterialDatePicker` (sin fechas futuras).
  El índice se firma con el certificado de la sesión (RSA). Si un fichero no es
  un documento ENI, se indica por su nombre y no se firma nada. El expediente
  se guarda por SAF como resultado de herramienta.
- **Lote**: operación «Firmar» o «Cofirmar» y casilla para añadir el sello
  visible a los PDF. Se usa el sello guardado en «Firma visible» adaptado a
  cada PDF: la página elegida (o la última si el PDF es más corto), todas las
  páginas (hasta 128) o las posiciones por página que existan. Para medir el
  PDF, `PdfRenderer` lee una copia temporal en `noBackupFilesDir` que se
  sobrescribe y se borra enseguida. La leyenda CSV no se añade en lote.
- **DNIe en lote y en «proteger y firmar»**: el PIN se pide una vez. En el lote
  se guarda solo en memoria, dentro del `PasswordCallback` de jmulticard,
  hasta que termina la operación, y se borra también si no llega a empezar.
  Después de cada firma se cierra el canal seguro de PIN, igual que hace
  `DnieNfc.sign` de jmulticard, y la firma siguiente vuelve a abrir PACE y a
  verificar el PIN con la tarjeta, que es lo que exige la clave de FIRMA. Al
  primer fallo (PIN erróneo, tarjeta retirada) la app deja de usar la tarjeta
  para no gastar intentos y explica el motivo junto al resultado de cada
  fichero. La lectura NFC sigue abierta durante todo el lote: hay que mantener
  el DNIe apoyado. Límites que no dependen de la app: `PasswordCallback.getPassword`
  devuelve copias que jmulticard no siempre borra, y jmulticard guarda el CAN en
  un `String` estático para reabrir PACE.
- **Perfiles con sello de tiempo** para XMLdSig, ODF, OOXML, FacturaE y
  ASiC-XAdES: el motor de escritorio no los admite (solo CAdES, PAdES y XAdES
  añaden sello de tiempo), así que siguen limitados al perfil B.

Los textos nuevos están en `res/values*/strings_ola4.xml` (once idiomas) y las
pantallas en `section_expediente.xml` y `section_batch_wave4.xml`.
`check_locales.py` comprueba todos los `strings*.xml` de cada idioma.

Pendiente: probar en un móvil real el lote con DNIe 3.0 y 4.0 (varias firmas
seguidas con un solo PIN, PIN erróneo a mitad de lote y retirada de la tarjeta),
el expediente con documentos ENI de otras aplicaciones y la revisión de
usabilidad independiente. También hay que reconstruir el AAR con
`createENIFileJSON` y fijar su nuevo SHA-256.
