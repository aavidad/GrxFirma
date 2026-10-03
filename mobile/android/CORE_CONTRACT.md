<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Contrato AAR Android v1

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
installExternalIdentityJSON(String, mobilebind.ExternalDigestSigner) -> String
signJSON(String) -> String
sealPreviewJSON(String) -> String
verifyJSON(String) -> String
```

Android usa `importCertificateBytesJSON` para no crear una segunda copia
Base64 del PKCS#12 en la capa Kotlin. `importCertificateJSON` se conserva por
compatibilidad del contrato. Los buffers mutables se sobrescriben al terminar;
la contraseña cruza como `String` por una limitación explícita de `gobind` y no
se declara borrado perfecto del heap administrado.

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
  "contract_version": 1,
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
    "actions": ["sign"],
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
