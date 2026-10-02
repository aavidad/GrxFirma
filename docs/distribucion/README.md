<!-- Derechos de autor (C) 2026 Diputación de Granada. -->
<!-- Autoría: Oficina de Software Libre de la Diputación de Granada. -->
<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Distribución de GrxFirma

Orden recomendado para las vías de menor coste:

1. Publicar el [sitio estático](../sitio/index.html) con GitHub Pages y comprobar su [política de privacidad](../sitio/privacidad.html). En Settings → Pages, seleccionar «GitHub Actions» como fuente. Cada push a `main` que modifique el sitio lo despliega.
2. Preparar las fichas de [Edge y Firefox](TIENDAS-EXTENSION.md), gratuitas, con los paquetes de `build.py`, la política pública y las notas para revisores. Chrome Web Store requiere una cuota única de 5 USD.
3. Solicitar [SignPath Foundation](SIGNPATH.md) para la firma Windows. La integración en CI es opcional hasta disponer de aprobación y secretos; el flujo oficial mantiene sus requisitos actuales de firma.
4. Publicar el [APK Android](ANDROID.md) firmado en GitHub Releases cuando se hayan superado los controles de la release y solicitar su inclusión en F-Droid con la receta desde fuentes.

Las publicaciones externas y la activación de cuentas requieren las credenciales y la revisión del responsable. Contacto único del proyecto: avidad@dipgra.es.
