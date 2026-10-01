<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Build, firma y TestFlight de iOS

Este procedimiento produce un IPA para revisión. No publica automáticamente y
no permite construir un Release de verificación.

## Requisitos Apple

- Mac compatible con una versión soportada de Xcode 16 o posterior;
- Xcode y licencia inicializados, SDK de iPhoneOS y simuladores instalados;
- Apple Developer Program y rol suficiente en App Store Connect;
- identificadores explícitos para app y Share Extension;
- App Group común y Keychain Group de la app;
- dos perfiles App Store Distribution, uno por target;
- certificado `Apple Distribution` con su clave privada en el llavero;
- registro de la app, ficha de privacidad, clasificación, export compliance,
  textos, capturas iPhone/iPad y política de soporte.

Convención exigida por el gate:

```text
app bundle:       es.organismo.grxfirma
share bundle:     es.organismo.grxfirma.share
app group:        group.es.organismo.grxfirma
keychain group:   es.organismo.grxfirma.keys
```

## 1. Toolchain y proyecto

```bash
scripts/mobile/ios/install-toolchain.sh
scripts/mobile/ios/validate-project.sh
```

Para ejecutar XCTest en simulador:

```bash
RUN_XCODE_TESTS=1 scripts/mobile/ios/validate-project.sh
```

## 2. Núcleo XCFramework

Con la fábrica descrita en `mobile/ios/CORE_CONTRACT.md` integrada y commiteada:

```bash
scripts/mobile/ios/build-core-xcframework.sh
```

Genera, valida e instala localmente:

```text
mobile/ios/Frameworks/Mobilebind.xcframework
mobile/ios/Frameworks/Mobilebind.xcframework.sha256
```

Ambos se ignoran en Git. El checksum es canónico sobre rutas y contenido, no
un hash dependiente del ZIP. Debe conservarse junto a la evidencia de build.

## 3. Configuración privada

```bash
cp mobile/ios/Config/Signing.example.xcconfig \
   mobile/ios/Config/Signing.local.xcconfig
cp packaging/mobile/ios/ExportOptions.example.plist \
   packaging/mobile/ios/ExportOptions.plist
```

Sustituya todos los ejemplos. Los dos ficheros están ignorados y nunca deben
contener claves privadas ni contraseñas. Los certificados se importan al
llavero mediante los procedimientos de la organización.

Compruebe el gate antes de archivar:

```bash
scripts/mobile/ios/check-release-inputs.sh \
  --xcconfig mobile/ios/Config/Signing.local.xcconfig
```

## 4. Archivo e IPA

```bash
SIGNING_XCCONFIG="$PWD/mobile/ios/Config/Signing.local.xcconfig" \
EXPORT_OPTIONS_PLIST="$PWD/packaging/mobile/ios/ExportOptions.plist" \
packaging/mobile/ios/build-release.sh "$PWD/release/ios"
```

El proceso:

1. valida el XCFramework y su checksum;
2. rechaza fuentes iOS sin commit y placeholders de firma;
3. crea un `.xcarchive` para dispositivo genérico;
4. verifica recursivamente app y extensión con `codesign`;
5. decodifica ambos perfiles y compara Team ID, Bundle IDs y App Group;
6. rechaza `get-task-allow` y ausencia de protección completa;
7. exporta un único IPA, rechaza rutas ZIP inseguras y vuelve a validar su
   firma, perfiles, ATS, App Group, Keychain y privacy manifests;
8. emite `SHA256SUMS` únicamente para el IPA final validado.

## 5. TestFlight

La subida debe ser una acción deliberada del responsable de release:

1. abra el `.xcarchive` en Xcode Organizer;
2. ejecute `Validate App` y resuelva todos los avisos;
3. distribuya con `App Store Connect > Upload` o use Transporter;
4. confirme que símbolos y build aparecen procesados;
5. asigne primero un grupo interno de TestFlight;
6. pruebe en iPhone y iPad físicos antes de revisión externa.

No se automatiza la subida porque requiere credenciales App Store Connect,
trazabilidad del operador y aceptación explícita de metadatos/export compliance.
Consulte también la [guía oficial de carga de builds](https://developer.apple.com/help/app-store-connect/manage-builds/upload-builds)
y el [flujo oficial de TestFlight](https://developer.apple.com/help/app-store-connect/test-a-beta-version/testflight-overview/).

## Matriz mínima en Apple

- simulador iPhone e iPad en la versión mínima y la última versión de iOS;
- dispositivo físico con bloqueo y Data Protection activo;
- importación desde iCloud Drive, proveedor de ficheros y Share Sheet;
- PKCS#12 correcto, contraseña errónea, certificado caducado y cancelación;
- PAdES, CAdES y XAdES con documentos en límites y por encima de límites;
- `afirma://` directo, remoto, repetido, manipulado, HTTP, IP y hosts distintos;
- segundo plano/bloqueo durante firma, memoria limitada y cancelación;
- VoiceOver, tamaños de texto de accesibilidad, contraste y teclado iPad;
- instalación limpia, actualización, restauración y borrado de la app;
- validación del IPA exportado y prueba TestFlight fuera de la red corporativa.

La validación Apple real, la firma y TestFlight quedan pendientes hasta disponer
de Mac, perfiles, certificados, Team ID y la fábrica iOS del núcleo.
