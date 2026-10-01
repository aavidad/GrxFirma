<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Safari Web Extension

## Estado de la integracion

Safari no usa los manifiestos `NativeMessagingHosts` de Chrome o Firefox. La
extension debe vivir dentro de una app macOS y Safari entrega sus mensajes a un
`SafariWebExtensionHandler` perteneciente a esa app. Apple documenta este
modelo en [Messaging between the app and JavaScript in a Safari web extension](https://developer.apple.com/documentation/safariservices/messaging-between-the-app-and-javascript-in-a-safari-web-extension).

`build-safari.sh` ya no deja el handler generado por Xcode como marcador. El
flujo actual:

- convierte la extension Chromium en un directorio temporal vacio y solo
  publica el resultado despues de validar y compilar;
- sustituye de forma determinista `SafariWebExtensionHandler.swift`;
- sustituye la vista de la app contenedora por la configuracion local;
- anade al `background.js` copiado un shim Safari que elimina el fallback REST
  JavaScript; las fuentes Chromium y Firefox no se modifican;
- crea entitlements de App Sandbox, red saliente y Keychain compartido;
- enlaza esos entitlements en Debug y Release;
- valida el proyecto y compila todos los targets sin firma.

En macOS el script solo publica el proyecto si Xcode compila todos los targets
sin firma. La firma, notarizacion y la prueba en Safari real siguen requiriendo
Xcode y credenciales Apple; no pueden certificarse desde Linux.

## Contrato de seguridad

El handler solo admite `ping`, `getCertificates`, `sign` y `verify`. Traduce
esas operaciones al REST local de GrxFirma y no permite rutas de fichero ni
destinos arbitrarios.

En Safari el handler es el unico puente REST. Si falla, la extension devuelve
el error nativo en vez de repetir la firma desde JavaScript sin el Bearer del
Keychain. Esto evita esperas acumuladas y reintentos ambiguos tras un timeout.

Safari vincula el canal nativo exclusivamente con la app contenedora e ignora
el identificador de host usado por otros navegadores. El handler no recibe la
URL de la pestana: el limite de origen sigue siendo el manifiesto convertido,
que solo inyecta scripts en los dominios institucionales declarados y no define
`externally_connectable`.

- Endpoint cerrado a `https://127.0.0.1:63118` o `https://[::1]:63118`.
- Sin redirecciones HTTP ni excepciones a la validacion TLS del sistema. Ademas
  exige que la hoja presentada coincida con la huella SHA-256 configurada; una
  CA instalada en el equipo no basta para suplantar el servicio loopback.
- Bearer de 32 a 512 caracteres almacenado en el Keychain con proteccion
  `ThisDeviceOnly`; nunca se incrusta en JavaScript ni en el proyecto.
- `getCertificates`, `sign` y `verify` fallan si no hay token valido.
- Cada `sign` exige `deviceOwnerAuthentication` de macOS. No hay reutilizacion
  temporal ni aprobacion desatendida, solo se permite una firma simultanea y se
  aplica una pausa minima entre solicitudes para impedir spam de dialogos. El
  texto de autorizacion incluye formato, identificador de certificado y SHA-256
  del documento exacto que se enviara al servicio.
- Un lease en el Keychain compartido impide firmas simultaneas incluso si Safari
  crea mas de un proceso de extension. Tiene propietario y caducidad; tras un
  timeout conserva el bloqueo 30 segundos adicionales antes del enfriamiento.
- Lista cerrada de campos y opciones, Base64 estricto y respuestas de error
  controladas que no incluyen payloads ni secretos.
- El build rechaza `externally_connectable` y cualquier permiso de host,
  opcional o de contenido, que salga de la allowlist institucional/loopback.
- Maximo 26 MiB por sobre JSON, 12 MiB de Base64 por entrada (aprox. 9 MiB
  binarios), 18 MiB de Base64 de salida, 1 MiB de opciones, 512 certificados y
  170 segundos por operacion. El puente admite como maximo dos peticiones
  REST concurrentes.

Apple no publica en esa API un limite de transporte que garantice esos tamanos
en todas las versiones de Safari. Los valores anteriores son topes defensivos
del handler, no una certificacion del canal: la prueba macOS debe incluir
documentos cercanos a cada limite y registrar el mayor tamano interoperable.
La cancelacion HTTP no puede demostrar que un servicio remoto no llego a firmar.
El handler envia `request_id` en `/sign` y el backend conserva durante 24 horas
solo su hash SHA-256: un reintento concurrente o posterior, incluso tras
reiniciar el proceso, recibe `409` y no genera una segunda firma. El registro
esta limitado, se escribe de forma atomica con permisos privados y falla
cerrado si queda corrupto. Esta garantia es de ejecucion como maximo una vez;
por privacidad no persiste el documento ni la firma para reconstruir una
respuesta perdida tras un timeout.

La app contenedora permite guardar el Bearer, la huella TLS y el endpoint,
eliminar el Bearer y abrir los ajustes de Safari. App y extension seleccionan
explicitamente el grupo de acceso Keychain
`$(AppIdentifierPrefix)<bundle-id>.shared`; ninguno de esos valores se guarda
en `UserDefaults` ni en un fichero de configuracion. El modelo sigue la guia de
Apple para [compartir elementos Keychain entre targets](https://developer.apple.com/documentation/security/sharing-access-to-keychain-items-among-a-collection-of-apps).

## Construccion en macOS

Requisitos:

- macOS con Xcode completo seleccionado mediante `xcode-select`;
- `xcrun safari-web-extension-converter` y `xcodebuild`;
- Python 3.9 o posterior;
- una fuente de extension con `manifest.json` y permiso `nativeMessaging`.

Ejecucion:

```bash
./packaging/browser-extensions/build-safari.sh
```

Variables admitidas:

```bash
SAFARI_EXTENSION_SRC=packaging/browser-extensions/src/chromium
SAFARI_PROJECT_DIR=release/safari-web-extension
SAFARI_APP_NAME="GrxFirma Safari"
SAFARI_BUNDLE_ID=es.dipgra.grxfirma.safari
SAFARI_MIN_MACOS=12.3
SAFARI_DEFAULT_ENDPOINT=https://127.0.0.1:63118
```

`SAFARI_SKIP_XCODE_BUILD=1` omite la compilacion y solo debe usarse para
diagnostico del conversor. El valor por defecto compila todos los targets con
`CODE_SIGNING_ALLOWED=NO`. Una salida anterior no se mezcla con el conversor:
se conserva hasta que la nueva pasa todas las comprobaciones y entonces se
sustituye mediante renombrado en el mismo volumen. El directorio generado
contiene `.grxfirma-safari/integration-report.json` con rutas de ficheros y
entitlements, UUID/nombre/configuraciones de ambos targets, version de Xcode,
ruta del conversor y SHA-256 determinista del arbol fuente.

La fuente actual usa Manifest V3. WebKit incorporo ese modelo en
[Safari 15.4](https://webkit.org/blog/12445/new-webkit-features-in-safari-15-4/),
por lo que el script fija macOS 12.3 como minimo y rechaza un valor inferior.
Una fuente Manifest V2 puede declarar un minimo anterior, pero tendria que
validarse como artefacto distinto.

El postprocesador falla si encuentra mas de un proyecto, mas de un handler, un
layout de app desconocido, targets con `productType`/Bundle ID ambiguos, si
faltan Debug o Release, o si no puede enlazar los entitlements a todas las
configuraciones de ambos targets. Es idempotente: puede ejecutarse de nuevo sin
duplicar ajustes, valida todas las salidas antes de mutar, reemplaza cada fichero
de forma atomica y revierte el conjunto si una escritura posterior falla.

## Validacion disponible en Linux

Linux no puede ejecutar el SDK de Safari, pero permite verificar el shell, el
postprocesador y las invariantes de seguridad:

```bash
bash -n packaging/browser-extensions/build-safari.sh
shellcheck packaging/browser-extensions/build-safari.sh
python3 -m py_compile \
  packaging/browser-extensions/safari/postprocess.py \
  packaging/browser-extensions/safari/record_environment.py \
  packaging/browser-extensions/safari/validate_source.py
python3 -m unittest discover \
  -s packaging/browser-extensions/safari/tests \
  -p 'test_*.py' -v
node --test packaging/browser-extensions/safari/tests/native_only.test.mjs
```

La invocacion normal de `build-safari.sh` fuera de macOS termina inmediatamente
con un error explicito; no produce un proyecto que pueda confundirse con un
artefacto Apple validado.

## Activacion y prueba real

1. Abrir el `.xcodeproj` generado y seleccionar el mismo Team para la app y la
   extension.
2. Verificar App Sandbox y el mismo grupo Keychain en ambos perfiles, y
   `Outgoing Connections` solo en el perfil de la extension.
3. Ejecutar GrxFirma con REST TLS y Bearer habilitados, e instalar su CA
   local en el almacen de confianza de macOS. El handler exige simultaneamente
   confianza PKI valida y coincidencia del pin; no acepta certificados solo por
   estar fijados.
4. Calcular la huella de la hoja TLS que usa ese proceso. Para la ubicacion
   macOS predeterminada:

   ```bash
   openssl x509 \
     -in "$HOME/Library/Application Support/grxfirma/tls/websocket-localhost.crt.pem" \
     -outform DER | openssl dgst -sha256 -r | awk '{print $1}'
   ```

   Si GrxFirma informa otra ruta de certificado, usar esa ruta. Regenerar el
   certificado invalida el pin y obliga a actualizarlo de forma deliberada.
5. Ejecutar la app contenedora, introducir el Bearer emitido por GrxFirma y
   los 64 digitos de la huella SHA-256, y guardar. Si el servicio genera otro
   token al reiniciar, actualizarlo.
6. Habilitar la extension en Safari y comprobar `ping`, certificados,
   verificacion, cancelacion de firma y firma autorizada.
7. Repetir con servicio detenido, token incorrecto, pin TLS incorrecto, TLS no
   confiable, payload sobredimensionado y dos firmas concurrentes. Confirmar que
   la autorizacion muestra la misma huella de documento que la interfaz web.

Safari solo puede declararse soportado tras conservar evidencia de esas pruebas
en una maquina macOS limpia. El proyecto generado es solo para macOS; no declara
soporte iOS/iPadOS porque esos sistemas no pueden usar el firmador y almacenes
locales de macOS.

## Firma, notarizacion y distribucion

Para distribucion fuera de App Store se necesita Developer ID, Hardened Runtime,
notarizacion y stapling. Para App Store/TestFlight se necesitan los perfiles y
el canal de App Store Connect.

```bash
codesign --verify --deep --strict --verbose "GrxFirma Safari.app"
codesign -d --entitlements :- "GrxFirma Safari.app"
codesign -d --entitlements :- \
  "GrxFirma Safari.app/Contents/PlugIns/GrxFirma Safari Extension.appex"
spctl --assess --type execute --verbose "GrxFirma Safari.app"
xcrun notarytool submit "GrxFirma Safari.zip" \
  --keychain-profile "$MACOS_NOTARY_PROFILE" --wait
xcrun stapler staple "GrxFirma Safari.app"
```

Las dos salidas de entitlements deben mostrar exactamente el mismo access group
expandido, por ejemplo `TEAMID.es.dipgra.grxfirma.safari.shared`.

La entrega no se considera cerrada hasta verificar el artefacto firmado en otra
cuenta de usuario, con Safari estable, y registrar versiones de macOS, Safari,
Xcode, certificado, perfil y hash del binario probado.
