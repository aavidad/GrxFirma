<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Banco de pruebas FIRe local

Este banco comprueba que GrxFirma funciona como cliente de la rama «certificado
local» de FIRe, el componente de firma que muchas Administraciones usan en sus
portales. Funciona en un solo equipo, sin credenciales reales y sin depender de
ningún portal externo. Los scripts están en `scripts/fire-banco/`.

## Qué cubre

El recorrido es el de un portal real con FIRe:

1. El portal crea un lote, añade documentos y pide la firma con el cliente Java
   oficial de FIRe (`fire-client-java`). Se autentica ante FIRe con un
   certificado de aplicación por TLS mutuo, igual que en producción.
2. Chrome del sistema, sin interfaz y controlado con Playwright, abre la página
   de FIRe y pulsa «Firmar». El `autoscript.js` de FIRe, sin modificar,
   construye la URL `afirma://batch` con un lote JSON trifásico
   (`batchpresignerurl`, `batchpostsignerurl`, `stservlet`, `jsonbatch=true`).
3. El manejador `afirma://` de GrxFirma recibe esa URL, firma con un P12
   sintético contra `preSignBatchService` y `postSignBatchService` de FIRe y
   deja el resultado cifrado en el servicio de almacenamiento de FIRe.
4. La página de FIRe recoge el resultado y lleva al portal a su URL de éxito o
   de error. El portal recupera cada firma o el error.
5. Cada firma se verifica con GrxFirma (integridad y cadena hasta la CA del
   banco), con OpenSSL (CAdES, comparando el contenido con el original) y con
   `pdfsig` (PAdES válida y que cubre el documento completo), si está instalado.

Hay tres escenarios:

| Escenario | Lote | Resultado esperado |
|-----------|------|--------------------|
| `mixto` | CAdES implícita, XAdES Enveloping y PAdES | Las tres firmas se recuperan y verifican |
| `parcial` | CAdES, XAdES y un «PDF» que no lo es, sin `stoponerror` | FIRe marca solo ese documento con `PRESIGN_ERROR`; los otros dos se firman |
| `detener` | El mismo lote con `stoponerror` | FIRe aborta el lote y lleva al portal a la URL de error (código 50) |

### Versiones fijadas

`scripts/fire-banco/versiones.env` fija todo lo que se descarga:

- FIRe 2.4, `ctt-gob-es/fire`, commit `fb54e53cfb9601a0951ebabb86c617646a134c04`.
- clienteafirma 1.9, `ctt-gob-es/clienteafirma`, commit
  `1f29aa7a2aae8eda40a70fed79b0428153c78df8` (etiqueta `v1.9`). FIRe usa su
  núcleo trifásico (`afirma-server-triphase-signer-core`) y sus módulos de firma.
  Se compilan desde la fuente y se instalan en el repositorio Maven del banco,
  así que el WAR de FIRe no lleva binarios de Maven Central del cliente @firma.
- Apache Maven 3.9.9 y Apache Tomcat 9.0.98, comprobados con su SHA-512.

## Cómo ejecutarlo

Requisitos: Git, curl, OpenSSL, un JDK 11, 17 o 21 con `javac`, Go, Python 3 con
Playwright y Google Chrome. No se instala nada en el sistema: Maven, Tomcat, las
fuentes y la PKI quedan en `$FIRE_BANCO_DIR`, que por defecto es
`/tmp/fire-banco` y nunca puede estar dentro del repositorio.

```bash
scripts/fire-banco/preparar.sh      # descarga, compila y configura (unos 40 s con red rápida)
scripts/fire-banco/arrancar.sh      # Tomcat en https://127.0.0.1:18443/fire-signature
scripts/fire-banco/probar-lote.sh   # todos los escenarios; o mixto | parcial | detener
scripts/fire-banco/parar.sh
```

`probar-lote.sh` compila GrxFirma desde el árbol actual cada vez, así que sirve
de prueba de regresión después de tocar `internal/adapters/inbound/legacy/afirmauri/`.
Termina con código distinto de cero y un mensaje `FALLO:` si algo no cuadra.
Los registros de cada escenario quedan en `$FIRE_BANCO_DIR/prueba/<escenario>/`:
la URL `afirma://` capturada, la salida de GrxFirma, la del navegador, una
captura de la página final y las firmas.

Variables útiles: `FIRE_BANCO_DIR`, `FIRE_BANCO_JAVA_HOME`,
`FIRE_BANCO_RECOMPILAR=1` (vuelve a compilar FIRe) y `FIRE_BANCO_ESPERA`
(segundos de espera del navegador, 120 por defecto).

## Cómo está montado

- FIRe funciona sin base de datos: una sola aplicación (`default.appId`) cuyo
  certificado de cliente se compara con `default.certificate`, con
  `security.checkCertificate=true` y `security.checkApplication=true`. Las
  sesiones viven en memoria y el único proveedor es `local`.
- Tomcat escucha solo en `127.0.0.1`: HTTPS en el puerto 18443 y apagado en el
  18005. No hay conector HTTP ni AJP. El certificado TLS y el de la aplicación
  los firma la CA sintética del banco.
- `preparar.sh` genera la PKI con OpenSSL en cada banco nuevo: CA, servidor,
  aplicación y firmante (`CN=PRUEBA SINTETICO FIRMANTE - 99999999R`, con
  validez de 30 días). Las contraseñas son aleatorias y solo existen en
  `$FIRE_BANCO_DIR/pki/`. Nada de esto entra en Git.
- GrxFirma se ejecuta con un `HOME` propio dentro del banco, así que no toca la
  configuración, los almacenes ni los navegadores del usuario. Su círculo de
  confianza solo incluye `https://127.0.0.1:18443`.

### Por qué hay una prueba Go con etiqueta `firebanco`

El `grxfirmauri` sin interfaz gráfica rechaza siempre las firmas web porque
nadie puede confirmarlas, y no debe existir una variable de entorno que salte
esa confirmación. Por eso el lanzador del banco no ejecuta el binario, sino
`cmd/grxfirmauri/banco_fire_test.go`. Esa prueba pasa la URL capturada al
mismo manejador que usa `grxfirmauri` (`runE2EAislado`, el arnés de las pruebas
e2e del paquete) y solo cambia el diálogo de confirmación por un aprobador de
prueba. Se compila únicamente con `-tags firebanco` y se omite si falta
`FIRE_BANCO_URI`, así que nunca entra en `go test ./...`.

### Cómo llega la URL `afirma://` a GrxFirma

Un Chrome sin interfaz no puede entregar un enlace `afirma://` al sistema
operativo. `navegador.py` lo captura con el evento CDP
`Page.frameRequestedNavigation`, en el mismo punto en que Chrome lo pasaría al
manejador de protocolo, y lanza GrxFirma con esa URL. La página y el
`autoscript.js` son los de FIRe, sin cambios.

## Resultado de la ejecución del 5 de octubre de 2026

Ejecutado en el equipo de desarrollo (Linux, JDK 17, Chrome del sistema) sobre
la rama `feat/fire-banco`, con el banco preparado desde cero:

- `mixto`: CAdES (1995 bytes), XAdES (5752 bytes) y PAdES (55357 bytes)
  recuperadas por el portal. GrxFirma da integridad y confianza válidas en las
  tres. OpenSSL verifica la CAdES y su contenido coincide con el original.
  `pdfsig` da «Signature is Valid» y «Total document signed».
- `parcial`: `cades` y `xades` firmadas y verificadas; `roto` con
  `PRESIGN_ERROR`, tal como lo informa FIRe.
- `detener`: FIRe aborta el lote y el portal recibe el error 50 («Error al
  firmar el lote»). Es el comportamiento de FIRe con `stoponerror`: cualquier
  documento que no quede `DONE_AND_SAVED` anula el lote entero.

No apareció ningún fallo de compatibilidad en el motor. GrxFirma ya reenvía al
postfirmador el lote actualizado con los errores de la prefirma, como hace
AutoFirma Java (`BatchSigner.signJSON` de clienteafirma 1.9).

La verificación de GrxFirma informa de «revocación no concluyente» porque los
certificados sintéticos no tienen CRL ni OCSP. El banco solo exige integridad y
cadena válidas.

## Límites

- Cubre FIRe 2.4 con clienteafirma 1.9. Otras versiones de FIRe 2.x que usen
  los portales no están probadas; para hacerlo hay que cambiar
  `versiones.env` y repetir la ejecución.
- FIRe 2.4 usa siempre el modo de «servidor intermedio» (`setForceWSMode(true)`:
  servicios `storage` y `retrieve`), así que el banco no ejercita el canal
  WebSocket local de GrxFirma.
- Chrome acepta el certificado TLS de FIRe con `ignore_https_errors`, porque la
  CA del banco no está en su almacén. La validación TLS de GrxFirma frente a
  FIRe sí es real: confía en la CA sintética mediante `SSL_CERT_FILE`.
- El selector de certificados y el diálogo de confirmación de la interfaz
  gráfica no intervienen; los cubren las pruebas de la interfaz y las de
  portales reales.
- FIRe 2.4 necesita `audit.dir` aunque la auditoría esté desactivada; sin él da
  un `NullPointerException` en `AuditSignatureRecorder`. `preparar.sh` lo
  configura.
- `fire-client-java` declara Java 1.6, que los JDK actuales no compilan.
  `preparar.sh` lo compila con `-Djdk.version=1.8`.

## Cl@ve Firma no está cubierta

Cl@ve Firma (certificados centralizados en la nube) es otra rama de FIRe y no
pasa por GrxFirma: cuando el usuario la elige, la firma ocurre entera entre el
navegador, FIRe y los servidores de la Administración. Para probarla haría
falta un organismo adherido a Cl@ve, una aplicación dada de alta en un FIRe con
acceso a Cl@ve Firma y usuarios con certificado en la nube. No existe un
servicio de pruebas público y llamar a Cl@ve sin alta iría contra sus
condiciones de uso. FIRe trae un conector de simulación
(`fire-signature-connector-clavefirma-test`), pero solo probaría FIRe, no
GrxFirma, así que el banco no lo activa.
