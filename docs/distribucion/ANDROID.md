<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Distribución Android

## APK en GitHub Releases

La variante distribuible es `productionRelease` de [mobile/android](../../mobile/android/README.md). El workflow [release.yml](../../.github/workflows/release.yml) compila el AAR Go con `scripts/mobile/android/build-core-aar.sh`, lo valida por SHA-256, construye el APK y el AAB con Gradle y exige una clave propia de distribución almacenada fuera del repositorio. La misma clave debe conservarse para las futuras actualizaciones de este canal. No se debe publicar el APK `verificationDebug`: no ofrece operaciones criptográficas.

Para comprobar un APK descargado, usar `apksigner` de Android Build Tools y cotejar la huella SHA-256 del certificado con la huella oficial publicada por un canal independiente:

```bash
apksigner verify --verbose --print-certs GrxFirma-version-android.apk
```

Comprobar que la verificación termina correctamente, que v2 y v3 figuran como válidas y que la huella coincide. Comparar también el SHA-256 del archivo con `SHA256SUMS.txt` de la release y verificar su firma OpenPGP según [RELEASE_SIGNING.md](../RELEASE_SIGNING.md). No publicar una clave privada ni usar la clave de QA.

## F-Droid

La receta [fdroiddata](../../packaging/fdroid/es.dipgra.grxfirma.yml) describe una compilación desde fuentes: JDK 17, Android SDK 36, Build Tools 36.0.0, NDK 28.2.13676358, Go 1.26.8 y la revisión fijada de `gomobile`; primero construye el AAR y luego `productionRelease`. `GRXFIRMA_FDROID_BUILD=1` permite generar un APK **sin firma de GrxFirma** para que F-Droid lo firme con su propia clave. Ese APK tendrá una identidad de firma diferente a la de GitHub Releases; Android no permitirá actualizar directamente entre los dos canales sin desinstalar la aplicación. Es necesario conservar y revisar el mismo `applicationId` y `versionCode` al proponer cada versión.

La receta apunta a la etiqueta `v0.0.105`, que debe crearse sobre el commit revisado antes de presentar la solicitud. F-Droid puede requerir ajustes de disponibilidad de dependencias Go/Android o de su entorno de construcción; la inclusión depende de su revisión. No se versiona ningún AAR, APK ni clave privada.

### Texto listo para pegar en Request For Packaging

> Solicito la inclusión de GrxFirma en F-Droid. Código fuente público: https://github.com/aavidad/GrxFirma. Licencia: EUPL-1.2. ID de paquete Android: `es.dipgra.grxfirma`. GrxFirma permite seleccionar documentos con el selector del sistema, firmarlos y verificarlos localmente con un núcleo Go compilado mediante gomobile. La app Android no solicita permiso de Internet. La receta propuesta está en `packaging/fdroid/es.dipgra.grxfirma.yml`: compila el AAR y el APK `productionRelease` desde el código y entrega el APK sin firma para la firma de F-Droid; no usa binarios precompilados del proyecto. La etiqueta de versión debe corresponder a `VERSION.txt`. Contacto: avidad@dipgra.es.
