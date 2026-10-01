<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Empaquetado Android

`build-release.sh` produce un APK y un Android App Bundle de la variante
`productionRelease`. El proceso se detiene si falta cualquiera de estos gates:

- AAR `gomobile` con contrato Android v1 y tres ABI;
- SHA-256 aprobado del AAR;
- almacen, alias y secretos de firma recibidos por variables de entorno;
- pruebas unitarias y Android Lint;
- verificacion `apksigner`, alineacion de paginas de 16 KiB y firma del AAB.

Variables obligatorias:

```text
GRXFIRMA_ANDROID_CORE_AAR
GRXFIRMA_ANDROID_CORE_SHA256
GRXFIRMA_ANDROID_KEYSTORE
GRXFIRMA_ANDROID_KEYSTORE_PASSWORD
GRXFIRMA_ANDROID_KEY_ALIAS
GRXFIRMA_ANDROID_KEY_PASSWORD
```

Los artefactos se escriben en `packaging/mobile/android/dist/`, que esta fuera
del control de versiones, como `GrxFirma-0.0.90-android.apk` y
`GrxFirma-0.0.90-android.aab` para la versión actual. Las credenciales nunca deben almacenarse en Gradle,
el repositorio ni los logs de CI.
