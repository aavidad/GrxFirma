<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma para iOS y iPadOS

Shell nativo SwiftUI para iOS/iPadOS 16 o posterior. Comparte el núcleo Go con
el resto del producto mediante `Mobilebind.xcframework` y mantiene toda la
interacción de sistema en código Apple.

## Implementado

- interfaz adaptativa para iPhone y iPad con Dynamic Type, VoiceOver e
  identificadores de accesibilidad;
- importación desde Archivos, apertura documental y extensión Compartir con un
  documento por operación para mantener una confirmación inequívoca;
- almacenamiento temporal con `NSFileProtectionComplete`, exclusión de backup,
  rechazo de enlaces simbólicos, comprobación SHA-256 antes de firmar y velo
  opaco en la instantánea del selector de apps;
- firma, cofirma, contrafirma y verificación mediante el contrato JSON de
  `mobilebind`;
- confirmación explícita, de un solo uso y válida 60 segundos, ligada al hash,
  certificado, acción, formato y algoritmo solicitado visibles;
- importación PKCS#12 sin persistir la contraseña; la identidad del núcleo
  actual vive solo en memoria y se limpia en segundo plano; el control de
  replay usa Keychain `WhenUnlockedThisDeviceOnly`, sin sincronización iCloud;
- enlaces `afirma://` directos con límites y control de replay en Keychain; las
  sesiones remotas se validan con HTTPS estricto y se rechazan mientras el
  contrato anuncie `remote_exchange=false`;
- ATS sin excepciones, App Group compartido, privacy manifest y extensión con
  copia transaccional;
- proyecto Xcode reproducible con app, Share Extension y tests Swift;
- Release fail-closed: firma manual, núcleo production, XCFramework validado y
  perfiles separados de app/extensión.

## Árbol

```text
mobile/ios/
  GrxFirma/             app SwiftUI, adaptador gomobile y recursos
  ShareExtension/          recepción de documentos desde otras apps
  Tests/                   pruebas XCTest
  Config/                  xcconfig Debug/Release y ejemplo de firma
  Frameworks/              destino local ignorado del XCFramework
  GrxFirma.xcodeproj/   salida determinista del generador
scripts/mobile/ios/        toolchain, generación y validadores
packaging/mobile/ios/      archivo/exportación para App Store Connect
```

## Validación multiplataforma

Desde Linux o macOS:

```bash
scripts/mobile/ios/validate-project.sh
```

Comprueba scripts, plists, entitlements, ATS, privacidad, icono sin alfa,
fuentes incluidas, controles de seguridad, pruebas Python y regeneración byte a
byte del `.xcodeproj`. En Linux termina indicando que Xcode no se ha ejecutado.

En macOS, para regenerar el proyecto:

```bash
scripts/mobile/ios/install-toolchain.sh
ruby scripts/mobile/ios/generate-xcode-project.rb
RUN_XCODE_TESTS=1 scripts/mobile/ios/validate-project.sh
```

El script instala para el usuario `xcodeproj 1.28.1` y las herramientas
`gomobile/gobind` fijadas. No acepta silenciosamente otras versiones.

## Modos de compilación

`Debug` usa `GRXFIRMA_CORE_MODE=verification`. Puede compilar y ejecutar la
UI sin XCFramework, pero todas las operaciones criptográficas permanecen
deshabilitadas y el estado lo muestra explícitamente.

`Release` define `PRODUCTION_CORE` y `GRXFIRMA_PRODUCTION_CORE=1`. El
Objective-C incluye obligatoriamente `<Mobilebind/Mobilebind.h>`, enlaza
`-framework Mobilebind` y ejecuta `check-release-inputs.sh`. Framework, fábrica,
checksum, Team ID, perfiles o identidad ausentes provocan error.

El contrato binario completo está en [CORE_CONTRACT.md](CORE_CONTRACT.md).

## Límites conocidos

- No se ha compilado Swift ni ejecutado Xcode en Linux.
- La fábrica iOS concurrente ya existe, pero aún debe compilarse y validarse en
  Xcode; declara identidad en memoria y no implementa intercambio remoto.
- La firma/notarización de macOS no sustituye la firma de distribución iOS.
- Face ID y acceso a claves dependen de la implementación final del adaptador
  Go/Keychain y deben probarse en dispositivo físico.
- La idoneidad jurídica del certificado y la política de firma corresponden al
  núcleo y al despliegue, no a la shell visual.
