<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Privacidad y revisión App Store

La declaración técnica incluida indica que la shell no rastrea ni recopila
datos para analítica. Antes de cada envío, el responsable debe contrastarla con
el comportamiento real del núcleo, endpoints institucionales, SDK incorporados
y política de soporte.

Revisión obligatoria:

- datos de documentos y certificados tratados en el dispositivo;
- servidores de recuperación/subida configurados por el organismo;
- consultas OCSP, CRL, TSA o validación remota realizadas por el núcleo;
- diagnósticos, logs y canal de soporte activados en la distribución;
- motivos de Required Reason APIs detectados por el informe de privacidad de
  Xcode;
- declaración de cifrado/export compliance en App Store Connect;
- política de retención, borrado y ejercicio de derechos del despliegue.

`PrivacyInfo.xcprivacy` se incluye en la app y en la Share Extension. Declara
acceso a metadatos de ficheros del contenedor con motivo `C617.1` y de ficheros
elegidos por el usuario con motivo `3B52.1`, sin tracking, dominios de tracking
ni categorías de datos recogidos por la shell. Un cambio en dependencias o
telemetría obliga a revisar la declaración y la ficha antes de publicar.

Referencias Apple: [motivos de APIs de metadatos de fichero](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)
y [flujo de builds en App Store Connect](https://developer.apple.com/help/app-store-connect/manage-builds/upload-builds).
