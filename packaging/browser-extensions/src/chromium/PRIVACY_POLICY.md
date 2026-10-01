<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Política de privacidad de la extensión GrxFirma

**Actualizada el 30 de septiembre de 2026.**

GrxFirma es una aplicación de firma de escritorio. Su extensión de navegador conecta los sitios permitidos con esa aplicación. No tiene cuentas propias ni envía documentos o datos de uso a servidores de GrxFirma.

## Qué datos trata

Cuando el usuario pulsa «Firmar» sobre un PDF en un sitio permitido, la extensión intenta descargarlo usando la sesión activa de ese portal. Guarda la precarga temporalmente en memoria o en `storage.session` y la entrega al firmador local mediante un token de un solo uso. La precarga deja de poder consumirse a los cinco minutos. Se borra al consumirla, en la siguiente limpieza de solicitudes caducadas o al cerrar la sesión del navegador. Si no puede precargarlo, abre el firmador sin documento para que el usuario lo elija. El archivo que el usuario firma queda bajo el control de la aplicación local y del destino que elija.

En los dominios de fábrica y en los sitios fijados por la organización, el portal puede solicitar una prueba de identidad. GrxFirma pide seleccionar un certificado y aprobar la operación. Solo después, la extensión devuelve al mismo portal la firma de la prueba, el certificado público y su cadena. El portal puede tratar esos datos según su propia política. La extensión no entrega la clave privada ni ofrece esta función a sitios añadidos por el usuario.

La extensión conserva en `storage.local` la lista de sitios que el usuario ha añadido. Puede leer una lista de sitios fijados por el administrador en `storage.managed`. Esas listas contienen orígenes o patrones de sitios, no documentos ni credenciales. Un sitio fijado necesita también el permiso de host concedido por la organización para que funcione. La extensión no incluye bóveda de contraseñas, inicio de sesión automático ni sincronización de credenciales.

## Permisos y conexiones

`nativeMessaging` comunica la extensión con el host instalado por GrxFirma. `storage` guarda la lista de sitios y las precargas temporales. `scripting` registra el detector de PDF únicamente en los sitios HTTPS concedidos. El acceso permanente a hosts cubre `dipgra.es`, `savia.net` y el servicio local `https://127.0.0.1`; los sitios nuevos requieren un permiso opcional que el usuario concede y puede revocar.

La descarga de un PDF contacta al portal donde se encuentra el archivo, con las cookies de esa sesión. La extensión se comunica con GrxFirma en el equipo; no envía el PDF, el certificado ni la firma a servidores de GrxFirma. La aplicación de escritorio puede consultar servicios de sellado de tiempo o revocación de certificados según su configuración. Los registros de diagnóstico de la extensión quedan en la consola local del navegador y no incluyen el contenido del PDF.

## Contacto

Oficina de Software Libre de la Diputación de Granada.
