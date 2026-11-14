<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Interoperabilidad local con material sintético

```sh
export GOPROXY=off GOCACHE=/tmp/codex-gocache-26 GOFLAGS=-mod=readonly
# Directorio opcional para conservar originales, firmados, certificados y TSA.
GRXFIRMA_CONFORMANCE_ARTIFACTS=/tmp/conformidad-release \
  go test -count=1 -v ./test/conformance -run TestInteroperabilidadLocal
```

| Caso | Comprobación independiente |
|---|---|
| PAdES básico | qpdf comprueba estructura; pdfsig valida firma y cobertura; OpenSSL valida CMS extraído con ByteRange y certificado sintético explícito |
| PAdES-B-T | Las anteriores y `openssl ts -verify` de respuesta RFC 3161 frente a la petición, con certificado TSA explícito; se comprueba que el token está embebido y su huella corresponde a la firma CMS |
| CAdES | `openssl cms -verify -binary`, contenido recuperado idéntico al original; se rechaza el contenido alterado |
| XAdES enveloped | Verificador interno y xmlsec1, cuando esté instalado, con certificado público fijado, referencias de documento y SignedProperties |
| FacturaE | Firma de la muestra sintética existente y verificación interna; xmlsec1, si está instalado, comprueba XMLDSig y referencias; no se modifica la política FacturaE |

También se altera el contenido firmado en las dos variantes PAdES para exigir
que OpenSSL rechace la manipulación. No se desactiva la verificación CMS ni se
usa `-noverify`. `-purpose any` en CMS permite el certificado sintético de firma
sin EKU de correo; la TSA se valida con su propósito timestamping y EKU crítico
exclusivo, como exige RFC 3161.

La TSA se ejecuta dentro de la prueba: el cliente real `tsaclient` intercambia
petición y respuesta HTTP mediante un transporte en memoria para 127.0.0.1.
Así no requiere sockets, DNS ni servicios y conserva nonce y messageImprint.
Esto prueba el intercambio RFC 3161 y su inclusión, no la conectividad TCP/TLS
ni la confianza de una TSA cualificada. No se usan identidades reales. En la salida local, pdfsig muestra 01/01/1970
como Signing Time del PAdES-B-T; el GenTime del token RFC 3161 se valida por
separado. Se conserva ese diagnóstico pendiente de estudiar, sin presentarlo
como conformidad completa del perfil.

La base NSS de pdfsig es temporal y vacía. `-nocert -no-ocsp` evita consultas de
confianza por red y conserva la validación criptográfica; OpenSSL comprueba el
certificado sintético anclado explícitamente. Los certificados no llevan AIA,
OCSP ni puntos CRL. No se exportan claves privadas al conservar artefactos.

El soporte existente `exttools.Available` deja en el log cada herramienta
faltante sin omitir la firma ni las verificaciones internas. Para una máquina
preparada con **todas** las herramientas, `GRXFIRMA_REQUIRE_EXTERNAL_TOOLS=1`
convierte cada ausencia en fallo. El 14/11/2026 faltaba xmlsec1: XAdES y FacturaE
no tienen aún evidencia independiente ejecutada en este entorno. La invocación
a xmlsec1 queda preparada, pero deberá probarse al instalarlo. No hay `t.Skip`
nuevos ni se rebaja el gate DSS existente.

## Montar DSS en una máquina local

No se ha descargado DSS en este entorno. En una estación autorizada y con
acceso a los artefactos oficiales, preparar una distribución de DSS y su
aplicación REST compatibles con la versión de Java requerida por esa edición.
Fijar versión y SHA-256, verificar origen y licencia, y trasladar el paquete a
la máquina de pruebas sin documentos reales. No se fija aquí una versión que
no se ha podido comprobar.

1. Desplegar el servicio REST en `127.0.0.1`, accesible solo desde el equipo o
   contenedor de pruebas; sin acceso público ni salida a validadores remotos.
2. Configurar un almacén de confianza de laboratorio con los certificados
   sintéticos de firma y TSA exportados por estas pruebas. Para B-T, seleccionar
   un instante de validación coherente y aportar, si lo exige la política DSS,
   cadena, CRL/OCSP sintéticos locales. Desactivar la descarga automática de
   listas de confianza/AIA/OCSP externas, aportando los datos locales necesarios.
3. Exponer el contrato REST que espera `cmd/dssrunner`: petición con
   `signedDocument`, `originalDocuments` y `tokenExtractionStrategy`; respuesta
   con `DiagnosticData` y `SimpleReport`. Si la distribución tiene otro
   contrato, añadir un adaptador local y probarlo; no bastan HTTP 200 ni
   `SignatureIntact=true` para declarar conformidad.
4. Construir el runner existente y **fijar siempre** el endpoint local. Su
   valor predeterminado apunta a un servicio público; no ejecutarlo sin esta
   variable:

   ```sh
   go build -o /tmp/grxfirma-dssrunner ./cmd/dssrunner
   export GRXFIRMA_DSS_ENDPOINT=http://127.0.0.1:8080/services/rest/validation/validateSignature
   /tmp/grxfirma-dssrunner --format pades \
     --signed /tmp/conformidad-release/pades-t/firmado.pdf \
     --original /tmp/conformidad-release/pades-t/original.pdf \
     --expect TOTAL_PASSED
   ```

5. Repetir para PAdES básico, CAdES, XAdES y FacturaE (esta última como `xades`),
   con sus originales. Revisar además perfil/política EPES de FacturaE,
   SignedProperties, digest, cadena y sello. Alterar documento y token: DSS debe
   rechazar ambos. Conservar versión/configuración de DSS, certificados,
   originales, firmados, informes completos y hashes. El éxito exige
   `TOTAL_PASSED`, sin convertir `INDETERMINATE` en aprobación.

Las pruebas `TestConformidadDSS_*` existentes generan otras identidades en cada
ejecución: un servidor con confianza estática necesita aprovisionar esas
identidades mediante un runner de laboratorio. Los artefactos persistidos de
`TestInteroperabilidadLocal` permiten primero preparar la confianza y después
validar exactamente esas muestras. Esta receta queda pendiente de ejecución;
no constituye certificación ETSI ni interoperabilidad universal.
