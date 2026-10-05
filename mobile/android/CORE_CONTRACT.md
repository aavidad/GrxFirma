<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Contrato AAR Android v2

El AAR de produccion debe proceder de `gomobile bind ./mobilebind` y contener
`libgojni.so` para `armeabi-v7a`, `arm64-v8a` y `x86_64`.

## API Java generada requerida

Clase estatica `mobilebind.Mobilebind`:

```text
newAndroidFacade(String filesDir, String noBackupFilesDir) -> mobilebind.Facade
```

La funcion Go correspondiente debe ser publica y montar los casos de uso reales,
incluidos catalogo/importador de certificados, clave Android, motor de firma,
verificador, aprobacion y perfil de plataforma. No es valido construir la
fachada con servicios `nil`.

`NewAndroidFacade` usa `noBackupFilesDir` para los temporales privados que
necesita PAdES. Ambos directorios deben existir, ser rutas absolutas reales de
la aplicacion, no enlaces simbolicos y ser distintos entre si.

Clase `mobilebind.Facade`:

```text
mobileContractJSON() -> String
clearSession()
selectCertificateJSON(String) -> String
importCertificateJSON(String) -> String
importCertificateBytesJSON(byte[], String) -> String
importCertificateSecretBytesJSON(byte[], byte[]) -> String
installExternalIdentityJSON(String, mobilebind.ExternalDigestSigner) -> String
signJSON(String) -> String
sealPreviewJSON(String) -> String
verifyJSON(String) -> String
inspectSignatureJSON(String) -> String
processBatchJSON(String) -> String
createHashJSON(String) -> String
checkHashJSON(String) -> String
protectJSON(String, byte[]) -> String
unprotectJSON(String, byte[]) -> String
validateVeriFactuJSON(String) -> String
createENIDocumentJSON(String) -> String
validateENIJSON(String) -> String
eniCatalogsJSON() -> String
csvLegendJSON(String) -> String
certificateDetailsJSON() -> String
checkCertificateRevocationJSON(String) -> String
diagnosticsJSON() -> String
probeTimestampAuthorityJSON(String) -> String
readVeriFactuQRJSON(String) -> String
queryVeriFactuQRJSON(String) -> String
checkUpdateJSON(String) -> String
```

Android usa `importCertificateSecretBytesJSON`: PKCS#12 y contraseña UTF-8
cruzan JNI como buffers mutables. Kotlin conserva la contraseña en `CharArray`
y limpia los buffers también cuando falla la importación. Los dos métodos
anteriores se conservan por compatibilidad. La biblioteca PKCS#12 Go todavía
requiere una conversión interna a `string`; no se promete borrado perfecto de
esa copia ni del heap administrado.

`installExternalIdentityJSON` recibe el certificado de FIRMA y, si están
disponibles, las CA intermedias en DER codificado Base64. El callback recibe
únicamente un resumen y el nombre del hash. El núcleo comprueba la firma RSA
devuelta contra el certificado antes de construir PAdES, CAdES o XAdES. La
clave privada del DNIe permanece en la tarjeta.

`sealPreviewJSON` recibe `certificate_id` y `options` del mismo formato que
`signJSON`. Devuelve `image_base64` (PNG). Exige la identidad de la sesión y
usa el compositor PAdES real; la aplicación dibuja ese PNG en la página
renderizada por `PdfRenderer`.

`mobileContractJSON()` debe devolver, como minimo:

```json
{
  "contract_version": 2,
  "platform": "android",
  "services": {
    "sign": true,
    "verify": true,
    "select_certificate": true,
    "import_certificate": true,
    "seal_preview": true,
    "external_signer": true
  }
}
```

Cada valor `true` afirma que el servicio esta configurado y puede ejecutar una
operacion real. La aplicacion rechaza versiones, plataformas o servicios
incompletos y no habilita botones de firma.

El contrato operativo actual tambien declara, sin sobrestimar capacidades:

```json
{
  "identity_store": {
    "mode": "memory_session",
    "persistent": false,
    "max_identities": 1
  },
  "approval": "native_ui_explicit_action",
  "signing": {
    "actions": ["sign", "cosign", "countersign"],
    "profiles_by_format": {
      "CAdES": ["baseline", "t", "lt", "lta"],
      "PAdES": ["baseline", "t", "lt"],
      "XAdES": ["baseline", "t"]
    },
    "actions_by_format": {
      "CAdES": ["sign", "cosign", "countersign"],
      "PAdES": ["sign", "cosign"],
      "XAdES": ["sign", "cosign", "countersign"]
    },
    "tsa_url_schemes": ["http", "https"],
    "key_types_by_format": {
      "CAdES": ["RSA", "ECDSA"],
      "PAdES": ["RSA", "ECDSA"],
      "XAdES": ["RSA"]
    }
  },
  "verification": {
    "cryptographic_integrity": true,
    "system_trust_anchors": false,
    "revocation": "embedded_evidence_only"
  }
}
```

La identidad PKCS#12 y su clave permanecen solo en memoria hasta reemplazarla,
llamar a `clearSession()` o terminar el proceso. No se afirma persistencia en
Android Keystore ni protección mediante hardware TEE. La verificacion valida la
integridad criptografica, pero sin anclas del sistema la confianza se devuelve
como `unknown`, con advertencia. `verifyJSON` devuelve por separado
`integrity_status`, `certificate_status`, `trust_status` y
`revocation_mode`; la UI no convierte esos ejes en una etiqueta global de
«firma válida».
XAdES queda limitado a identidades RSA porque esa es la compatibilidad real del
motor actual; CAdES y PAdES admiten RSA y ECDSA.

`verifyJSON` recibe siempre la firma en `content_base64`. Para una firma
separada, la aplicación añade el contenido que se firmó en
`original_content_base64`:

```json
{
  "name": "documento-firmado.p7s",
  "mime_type": "application/pkcs7-signature",
  "content_base64": "<firma CAdES en Base64>",
  "original_content_base64": "<documento original en Base64>"
}
```

El campo del original se omite en firmas embebidas como las PAdES y XAdES
generadas por este cliente. La UI no intenta deducir ni sustituir el original:
lo solicita de forma opcional y explícita mediante SAF.

## Gate de artefacto

`validate_core_aar.py` comprueba:

1. ZIP y rutas internas seguras.
2. `classes.jar` y las tres ABI obligatorias.
3. clases y metodos publicos mediante `javap`.
4. SHA-256 exacto cuando se proporciona el valor aprobado.

La fachada Go implementa este contrato. El AAR no se versiona: debe generarse
desde un commit limpio con `scripts/mobile/android/build-core-aar.sh`, fijarse
por SHA-256 y pasar este gate. `productionRelease` sigue requiriendo la firma
Android oficial externa al repositorio.

`signJSON` recibe la operación en `action` y el perfil en `options.profile`.
El perfil predeterminado es `baseline`. Para T/LT/LTA exige `options.tsaURL`;
B con TSA produce T. La fachada rechaza credenciales, fragmentos, esquemas
ajenos a HTTP(S), puertos inválidos y perfiles que el formato no genera.
La TSA solo la configura la persona usuaria. Se admite `http` por
compatibilidad con TSA públicas como la de la FNMT y con TSA internas de la
organización; de la respuesta RFC 3161 se comprueban la firma, la huella y
el nonce. La red del núcleo Go (TSA, OCSP, CRL) no pasa por la pila de
Java, así que
`usesCleartextTraffic=false` del manifiesto Android no la limita: las reglas
de esquema, credenciales y redirecciones son las de la fachada.
PAdES no admite contrafirma ni LTA; XAdES no admite LT/LTA en este motor.
CAdES usa los firmadores comunes de cofirma, contrafirma, TSA y revocación.
El sellado CAdES-T conserva los firmantes y los atributos de contrafirma.

`inspectSignatureJSON` devuelve `has_signature` y `format`. Reutiliza los
parsers del motor y admite CAdES separado sin el original. Permite proponer
cofirma; su resultado no acredita integridad ni confianza.

El contrato incluye `engine_version`, fijado desde `VERSION.txt` al construir
el AAR con `-X grxfirma/mobilebind.engineVersion`. Un AAR sin sellado declara
`development`; Android lo muestra como versión no disponible. La app rechaza
un AAR v1 y exige los nuevos métodos v2 antes de habilitar operaciones.

Después de firmar, Android verifica la salida real. Para CAdES usa el original
firmado o, en cofirma/contrafirma, el original que haya seleccionado la persona.
El informe permanece separado del resultado de firma y del guardado. Una
verificación que falla no impide guardar la firma ni se presenta como éxito.
`verifyJSON` conserva firmantes, resúmenes, dictamen, cobertura, evidencias,
advertencias y errores; el JSON se puede exportar mediante SAF.

## Herramientas: huellas, protección y lote

`createHashJSON` recibe `content_base64`, `algorithm` (SHA-256, SHA-1,
SHA-384 o SHA-512) y `format` (`hex`, `base64` o `bin`). Devuelve `hash`,
`output_base64` y `extension`. El contenido del fichero es el mismo que guarda
el escritorio: hexadecimal en minúsculas terminado en `h` (`.hexhash`), Base64
(`.hashb64`) o los bytes del resumen (`.hash`). `checkHashJSON` recibe el
documento, `hash_file_base64` (máximo 4 KiB) y `hash_file_name`; deduce formato
y algoritmo como el escritorio y devuelve `valid`, `expected_hash` y
`actual_hash`.

`protectJSON(payload, secret)` usa los contenedores CMS del escritorio:
`cms` (EnvelopedData, `.enveloped`), `authenvelopeddata`
(`.authenveloped.p7m`) y `cms-encrypted` (EncryptedData, `.encrypted.p7m`).
Los destinatarios llegan en `recipients[].certificate_base64` (X.509 público
DER o PEM, máximo 16) y, con `include_session_certificate`, el certificado de
la sesión. Se aplican las mismas reglas que al importar un destinatario en
escritorio: RSA de 2048 bits o más, vigente, no CA y con cifrado de clave.
Android no guarda libreta de destinatarios. Para EncryptedData, `secret` es la
clave AES-256 en Base64 canónico (44 bytes ASCII); el núcleo borra el buffer al
volver. Con `sign: true` crea SignedAndEnvelopedData firmado con la identidad
PKCS#12 de la sesión; el DNIe no se admite en esta operación.

`unprotectJSON(payload, secret)` detecta el contenedor por su contenido y usa la
clave RSA de la identidad PKCS#12 importada o la clave transitoria. El DNIe no
expone descifrado. Los fallos se devuelven con un mensaje cerrado que no
distingue entre clave errónea y destinatario ajeno.

`processBatchJSON` firma hasta 16 documentos (32 MiB en total) con una única
aprobación y la identidad de la sesión. Cada documento pasa las mismas
validaciones que `signJSON` (nombre, MIME, acción, perfil y TSA). La respuesta
trae `items[]` en el orden de entrada, cada uno con `ok` y la firma o un error
propio. No admite `session`: `remote_exchange` sigue en `false`. El DNIe exige
PIN por firma, así que Android no le ofrece el lote. El JSON del lote admite
unos 87 MiB: el Base64 de los 32 MiB de documentos más una imagen de sello de
hasta 2 MiB repetida en cada documento y 2 MiB para nombres y opciones.

El contrato declara `process_batch`, `hash`, `protect`, `unprotect` y
`protect_sign`, además de `limits.batch_items`, `limits.batch_input_bytes`,
`protection` y `hash`.

## Formatos de escritorio, Veri*Factu, ENI y leyenda CSV

`signJSON` admite además `xmldsig`, `odf`, `ooxml`, `facturae`, `asic-xades`
y `verifactu`, con el motor de firma del escritorio. Con `format` vacío o
`auto` la fachada aplica las reglas de escritorio por extensión (PDF, OOXML,
ODF, `.asics`, `.dsig`/`.xmlsig`, XML) y las completa con el MIME de SAF y,
para XML, con el primer elemento del contenido: un XML cuya raíz es
`Facturae` se firma como FacturaE. Veri*Factu nunca se elige solo.

| Formato | Acciones | Perfiles | Claves |
| --- | --- | --- | --- |
| XMLdSig | sign, cosign | baseline | RSA |
| ODF | sign, cosign | baseline | RSA |
| OOXML | sign, cosign | baseline | RSA |
| FacturaE | sign | baseline | RSA |
| ASiC-XAdES | sign | baseline | RSA |
| VeriFactu | sign | baseline | RSA |

Estos formatos no reciben TSA. Con una identidad ECDSA, `signJSON` responde
con el mensaje cerrado «El formato elegido solo admite certificados con clave
RSA.». Si el XML no es un registro Veri*Factu responde `verifactu.root`; los
demás rechazos del motor Veri*Factu llegan como su clave `verifactu.*`. El
lote admite los mismos formatos salvo Veri*Factu.

`validateVeriFactuJSON` recibe `files[]` (`name`, `content_base64`), hasta 64
registros de 10 MiB y 32 MiB en total, y devuelve `valid`, `errors`,
`warnings` y `records[]` con `file`, `type`, `hash`, `calculated_hash`,
`previous_hash`, `signed`, `valid` e `issues[]` (`field`, `key`, `level`).
Las claves son las del catálogo de escritorio (`verifactu.*`); Android las
traduce. No consulta a la AEAT ni usa la red.

`createENIDocumentJSON` recibe `signature_base64`, `original_base64`
(obligatorio para CAdES explícita), `organs[]` (DIR3), `origin`
(`ciudadano` o `administracion`), `state` (EE01-EE04, EE99),
`document_type` (TD01-TD20, TD99), `identifier`, `source_identifier`,
`capture_date` (RFC 3339) y `content_format`. Devuelve `content_base64` y
`signature_type` (TF02-TF06). Los errores son claves cerradas:
`eni.validacion.*` del motor y `eni.error.unsigned_pdf`,
`eni.error.explicit_cades`, `eni.error.unrecognized`,
`eni.error.content_format`, `eni.error.origin` y
`eni.error.signature_mismatch`. Antes de envolver, la fachada coteja sin red
que la firma corresponde al original (el `messageDigest` de la CAdES
explícita, las referencias de la XAdES separada) y que el PDF firmado no se
ha alterado; si falla, responde `eni.error.signature_mismatch`. `validateENIJSON` devuelve
`valid` e `issues[]` con claves `eni.validacion.*`; no verifica las firmas.
`eniCatalogsJSON` devuelve `document_states`, `document_types` y
`file_states`. La firma y el original suman como máximo 48 MiB (una firma
implícita o PAdES llega sola; una separada es pequeña frente a su original de
hasta 32 MiB) y el JSON admite 65 MiB. El expediente ENI (carpeta de documentos con índice firmado)
no está en Android.

`csvLegendJSON` recibe `csv`, `csv_url` y `csv_text` y devuelve `url`
(normalizada por el motor, con el dominio IDN en ASCII) y `text`. Los errores
son `csv.error.code_missing`, `code_invalid`, `url_missing`, `url_invalid` y
`text_invalid`. La firma PAdES recibe la leyenda con las opciones `csv`,
`csvUrl`, `csvText` y `csvQR`.

El contrato declara `signing.formats` y los servicios `verifactu_validate`,
`eni_document`, `eni_validate` y `csv_legend`. Android enlaza estos métodos
como opcionales: con un AAR anterior oculta lo que no esté declarado.

## Tercera oleada: certificado, diagnóstico, QR tributario y versiones

Todos estos métodos son opcionales para la app: si un AAR anterior no los
enlaza o no los declara en `services`, la interfaz oculta la función. Ninguno
devuelve textos para mostrar; solo estados cerrados y datos. Los que usan red
solo se llaman tras una acción explícita.

| Servicio | Método | Red |
| --- | --- | --- |
| `certificate_details` | `certificateDetailsJSON()` | no |
| `certificate_online_check` | `checkCertificateRevocationJSON` | OCSP/CRL del certificado |
| `diagnostics` | `diagnosticsJSON()` | no |
| `tsa_probe` | `probeTimestampAuthorityJSON` | TSA elegida |
| `verifactu_qr_read` | `readVeriFactuQRJSON` | no |
| `verifactu_qr_query` | `queryVeriFactuQRJSON` | servicio público de la AEAT |
| `update_check` | `checkUpdateJSON` | API pública de GitHub |

`certificateDetailsJSON` devuelve `expiring_soon_days` (30) y
`certificates[]` con `certificate_id`, `subject`, `issuer`, `fingerprint`,
`nif`, `organization`, `kind` (`fisica`, `representacion`, `sello`,
`empleado_publico` o `desconocido`), `key_type`, `key_bits`, `not_before`,
`not_after` (RFC 3339 UTC), `days_left`, `status` (`valid`, `expiring_soon`,
`expired`, `not_yet_valid`), `external`, `can_encrypt`, `has_ocsp` y
`has_crl`. Hoy la sesión guarda como máximo una identidad.

`checkCertificateRevocationJSON` recibe `certificate_id` de la sesión y usa el
comprobador OCSP/CRL de escritorio (30 s como máximo). Devuelve `status`
(`valid`, `revoked`, `inconclusive`, `unavailable`), `method` (`OCSP`/`CRL`),
`checked_at`, `revoked_at`, `has_ocsp` y `has_crl`. No cambia la verificación
de firmas, que sigue con `revocation: embedded_evidence_only`.

`diagnosticsJSON` devuelve `engine_version`, `contract_version`, `platform`,
`go_version`, `architecture`, `engine_time_utc` y `session_identity` (booleano).
No incluye titular, NIF, huella ni rutas.

`probeTimestampAuthorityJSON` recibe `url`, la valida con las reglas de
`signJSON` y pide un sello RFC 3161 sobre un SHA-256 aleatorio con el cliente
de firma (nonce y firma de la TSA comprobados, 20 s como máximo). Devuelve
`status` (`ok`, `invalid_url`, `unreachable`, `timeout`, `rejected`,
`bad_response`), `https`, `tsa_time`, `local_time`, `skew_seconds` (hora del
dispositivo menos hora de la TSA) y `elapsed_ms`.

`readVeriFactuQRJSON` recibe `url` y devuelve `url`, `nif`, `numserie`,
`fecha`, `importe`, `verifiable` y `test`, validados con `LeerQRVeriFactu` de
escritorio sin acceder a la red. `queryVeriFactuQRJSON` vuelve a validar la URL
y llama a `ConsultarQRVeriFactu`: HTTPS, hosts y rutas oficiales de la AEAT,
sin proxy ni redirecciones, TLS 1.2 o superior y 12 s como máximo. Devuelve
`response` con el JSON de la AEAT (256 KiB como máximo). Los errores son las
claves `verifactu.qr_url`, `verifactu.qr_params` y `verifactu.qr_service`.

`checkUpdateJSON` recibe `current_version` (`[0-9A-Za-z.+-]`, 32 bytes) y
consulta `https://api.github.com/repos/aavidad/GrxFirma/releases/latest` con el
cliente de escritorio (redirecciones solo al mismo origen, respuesta acotada,
destino `https://github.com/aavidad/GrxFirma/releases/tag/...`) sobre un
transporte sin proxy, TLS 1.2 o superior y 10 s como máximo. Devuelve
`status` (`newer`, `current`, `not_comparable`, `no_releases`, `error`),
`error_code` (los de `updatecheck.ErrorCode`), `current`, `latest` y `url`.
Nunca descarga ni instala nada.

## Cuarta oleada: expediente ENI, lote con sello y cofirma, DNIe

`createENIFileJSON` crea un expediente ENI con la operación
`generar-expediente` del escritorio. Recibe `documents[]` (`name`,
`content_base64`; documentos ENI en XML, hasta 64 y 32 MiB en total; el JSON
admite unos 44 MiB),
`certificate_id`, `organs[]` (DIR3), `classification` (código SIA o
`<DIR3>_PRO_<id>`), `state` (`E01`, `E02` o `E03`, de
`eni.EstadosExpediente`), `identifier` opcional, `opening_date` opcional en
RFC 3339 (no más de un día en el futuro) e `interested[]` opcional (hasta 16).
Los documentos se ordenan por nombre, como en escritorio. Antes de firmar se
revisa cada documento con `eni.ValidarXML`: si alguno falla, la respuesta trae
`ok: false` e `issues[]` con el nombre del fichero en `field` y su clave
`eni.validacion.*`, sin pedir la firma. Si todo es correcto, el índice se firma
con `FirmarNodoXAdES` y la identidad de la sesión (PKCS#12 o DNIe; solo RSA,
como el XAdES del motor) y la respuesta trae `ok: true`, `content_base64` y
`documents`. Los demás errores son claves `eni.validacion.*` o el mensaje
cerrado de clave RSA.

`processBatchJSON` ya aceptaba `action` y `options` por documento. Android los
usa ahora para la cofirma en lote y para el sello visible PAdES: cada PDF lleva
sus propias opciones `visibleSeal*`, calculadas con su número de páginas. La
leyenda CSV no se usa en lote porque su código es propio de cada documento. Los
límites del lote no cambian (16 documentos, 32 MiB).

El DNIe, que solo firma resúmenes con `ExternalDigestSigner`, sirve ahora para
el lote y para `protectJSON` con `sign: true`. SignedAndEnvelopedData usa
go-cryptobin, que exige una `*rsa.PrivateKey`; el binario móvil registra un
firmador propio (`external_cms.go`) que solo acepta el firmador externo, calcula
el SHA-256 del contenido y delega la firma PKCS#1 v1.5.

Perfiles con sello de tiempo: el motor de escritorio solo añade sello de tiempo
a CAdES (T/LT/LTA), PAdES (T/LT) y XAdES (T). XMLdSig, ODF, OOXML, FacturaE y
ASiC-XAdES se firman siempre en perfil B, también en escritorio; Android sigue
rechazando para ellos los perfiles T, LT y LTA y no les envía la TSA.

El contrato declara los servicios `eni_file`, `batch_visible_seal`,
`batch_cosign`, `external_signer_batch` y `external_signer_protect_sign`, y
`limits.eni_file_documents`. Android los trata como opcionales: con un AAR
anterior mantiene el comportamiento previo.

## Informe de verificación imprimible

Con `include_html_report: true` en la petición de `verifyJSON` (solo si el
contrato declara `verify_report_html`), la respuesta trae además
`report_html_base64`: el mismo informe HTML de escritorio
(`informeverificacion.HTML`, autocontenido, con CSP sin scripts y valores
escapados), de 4 MiB como máximo. Como el móvil no evalúa la confianza con las
anclas del sistema, el informe nunca declara «firma válida»: si el documento no
ha cambiado dice «firma íntegra · validez del certificado no acreditada». La
plantilla está en castellano, igual que en escritorio.
