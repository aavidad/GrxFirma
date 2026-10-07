<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Extensiones de navegador de GrxFirma

La extensión acompaña a la aplicación de escritorio GrxFirma. Sin GrxFirma instalada, no puede abrir el firmador ni realizar una prueba de identidad. Sirve para cualquier organismo que use los protocolos admitidos.

## Funciones

En los sitios permitidos, la extensión detecta PDF y muestra el botón «Firmar». Al pulsarlo, intenta descargar el PDF con la sesión del portal y entregarlo al firmador local mediante un token temporal. Si la descarga o la precarga fallan, abre el firmador sin documento y avisa de que hay que elegirlo allí. La firma requiere la aprobación prevista por GrxFirma.

`dipgra.es` y `savia.net` están permitidos de fábrica. En «Sitios de confianza», el usuario puede conceder acceso HTTPS a otros sitios y retirarlo después. Un administrador también puede fijar sitios mediante `storage.managed`; el usuario puede verlos, pero no eliminarlos. Para que funcionen, la política o el despliegue empresarial también debe conceder el permiso de host correspondiente. Los sitios añadidos por el usuario solo reciben el botón de PDF.

La prueba de identidad funciona únicamente en los dominios de fábrica y en los sitios fijados por la organización. El portal solicita un reto; GrxFirma pide seleccionar un certificado y aprobar la operación. Después, la extensión devuelve al mismo portal la firma, el certificado público y su cadena. No entrega la clave privada ni enumera certificados al portal.

El paquete no incluye bóveda de contraseñas, autologin, sincronización de sesiones ni el firmador antiguo de `src/*/signer/`. `config.js` se distribuye con `LOCAL_REST_BEARER` vacío; `build.py` rechaza un bearer incrustado. Los archivos de referencia heredados permanecen en la fuente, fuera del ZIP/XPI.

La [política de privacidad](src/chromium/PRIVACY_POLICY.md) explica los datos tratados y su conservación. Las [notas para revisores](store/NOTAS_REVISORES.md) describen permisos y pasos de prueba en español e inglés. Los textos de las fichas, las capturas y los gráficos de tienda están en `store/`; la guía de publicación es [TIENDAS-EXTENSION.md](../../docs/distribucion/TIENDAS-EXTENSION.md).

## Fuente y paquetes

- `src/chromium/`: Chrome, Edge, Brave y otros navegadores Chromium.
- `src/firefox/`: Firefox de escritorio.
- `build.py`: genera un ZIP Chromium y un XPI Firefox desde estas fuentes.
- `SAFARI.md`: flujo independiente para Safari, sin cierre de distribución en esta entrega.

Para generar paquetes en un directorio externo al repositorio:

```bash
python3 packaging/browser-extensions/build.py --output-dir /ruta/de/salida
```

Sin `--output-dir`, los artefactos se escriben en `packaging/browser-extensions/`. El comando genera `grxfirma-extension-chromium.zip`, `grxfirma-extension-firefox-unsigned.xpi`, `grxfirma-extension-firefox.xpi` y `grxfirma-extension-firefox.metadata.json`. Si no se proporciona una firma Mozilla, el último XPI es una copia de desarrollo sin firmar: Firefox Release/ESR exige uno firmado. Para exigirlo en una compilación de distribución, define `GRXFIRMA_REQUIRE_SIGNED_FIREFOX_XPI=1` y proporciona `GRXFIRMA_FIREFOX_SIGNED_XPI` o las credenciales de `web-ext` previstas por `build.py`.

Los instaladores de GrxFirma deben tomar los artefactos recién generados desde la misma fuente. En Windows, el instalador registra la ficha de tienda para Chrome y Edge si están configurados sus IDs publicados (`GRXFIRMA_CHROMIUM_EXTENSION_ID` y `GRXFIRMA_EDGE_EXTENSION_ID`). Ambos navegadores solicitan confirmación al usuario; el instalador no coloca una copia privada de esas extensiones. No se deben reutilizar los ZIP/XPI antiguos de agosto: contienen el nombre del host previo.

## Pruebas locales

```bash
python3 -m unittest discover -s packaging/browser-extensions/tests -p 'test_build.py'
node --test packaging/browser-extensions/tests/*.test.mjs
```

El paquete debe usar el host `io.github.aavidad.grxfirma` y los prefijos `grxfirma-`. La extensión usa `storage` para la configuración y una precarga temporal del PDF, `nativeMessaging` para la aplicación local y `scripting` para registrar el detector en sitios que el usuario concede. `host_permissions` cubre los dominios de fábrica y el servicio local `https://127.0.0.1`; `optional_host_permissions` permite pedir acceso HTTPS a un sitio nuevo solo cuando el usuario lo añade. La identidad no se activa por ese permiso opcional.

Los textos visibles están en `_locales/es` y `_locales/en`. Al modificarlos, actualiza ambos catálogos y ejecuta `tests/i18n.test.mjs`.

Chromium declara `managed_schema.json` en el manifiesto. Firefox lee `storage.managed` cuando la política empresarial lo proporciona, aunque su manifiesto no utiliza ese esquema.
