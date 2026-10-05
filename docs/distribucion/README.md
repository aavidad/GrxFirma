<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Distribución de GrxFirma

Orden recomendado para las vías de menor coste:

1. Publicar el [sitio estático](../sitio/index.html) con GitHub Pages y comprobar su [política de privacidad](../sitio/privacidad.html). En Settings → Pages, seleccionar «GitHub Actions» como fuente. Cada push a `main` que modifique el sitio lo despliega.
2. Preparar las fichas de [Edge y Firefox](TIENDAS-EXTENSION.md), gratuitas, con los paquetes de `build.py`, la política pública y las notas para revisores. Chrome Web Store requiere una cuota única de 5 USD.
3. Solicitar [SignPath Foundation](SIGNPATH.md) para la firma Windows, o usar un certificado Authenticode propio. La [política de firma de código](../sitio/firma-de-codigo.html) del sitio es la página pública que pide SignPath.
4. Publicar el [APK Android](ANDROID.md) firmado en GitHub Releases cuando se hayan superado los controles de la release y solicitar su inclusión en F-Droid con la receta desde fuentes.

La guía [CERTIFICADOS.md](CERTIFICADOS.md) explica paso a paso cómo crear los certificados de Windows y la clave de Android, qué secretos dar de alta en GitHub y cómo lanzar y comprobar una release oficial.

Las publicaciones externas y la activación de cuentas requieren las credenciales y la revisión del responsable. Contacto único del proyecto: avidad@dipgra.es.
