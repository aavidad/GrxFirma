<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Publicar la extensión en Chrome, Edge y Firefox

La extensión necesita GrxFirma de escritorio y su host de mensajería nativa. Esta guía sirve para la primera publicación y para las actualizaciones. Todo lo que hay que subir o pegar ya está en el repositorio; lo único que falta es crear las cuentas.

## Qué está preparado

| Material | Dónde |
|---|---|
| Paquetes | Se generan con `build.py` (ver abajo). No se guardan en Git. |
| Textos de las fichas (es/en), justificación de permisos y uso de datos | [store/FICHAS.md](../../packaging/browser-extensions/store/FICHAS.md) |
| Notas para revisores (es/en) | [store/NOTAS_REVISORES.md](../../packaging/browser-extensions/store/NOTAS_REVISORES.md) |
| Capturas de 1280×800 (es/en) | `packaging/browser-extensions/store/capturas/{es,en}/` |
| Icono de tienda de 128 px | `packaging/browser-extensions/store/graficos/icono-128.png` |
| Mosaico promocional de 440×280 (es/en) | `packaging/browser-extensions/store/graficos/mosaico-440x280-{es,en}.png` |
| Política de privacidad pública | https://aavidad.github.io/GrxFirma/privacidad.html (fuente: [docs/sitio/privacidad.html](../sitio/privacidad.html)) |

Las capturas son reales: el script [store/generar_capturas.py](../../packaging/browser-extensions/store/generar_capturas.py) carga el ZIP en un perfil temporal de Chrome for Testing, conecta el host nativo compilado desde `cmd/nativehost` y abre un PDF ficticio. No contacta con ningún portal.

## 1. Generar y comprobar los paquetes

Desde el commit que se va a publicar:

```bash
python3 packaging/browser-extensions/build.py --output-dir /tmp/grxfirma-ext
python3 packaging/browser-extensions/verify_package.py /tmp/grxfirma-ext
python3 -m unittest discover -s packaging/browser-extensions/tests -p 'test_*.py'
node --test packaging/browser-extensions/tests/*.test.mjs
```

Salen cuatro archivos:

- `grxfirma-extension-chromium.zip`: el que se sube a Chrome Web Store y a Edge Add-ons, sin descomprimir.
- `grxfirma-extension-firefox-unsigned.xpi`: el que se sube a AMO. Mozilla lo revisa y lo firma.
- `grxfirma-extension-firefox.xpi` y su `.metadata.json`: sin credenciales de Mozilla es una copia sin firmar para desarrollo. No se ofrece como descarga.

Opcional, para ver lo que dirá el validador de Mozilla antes de subir:

```bash
mkdir /tmp/grxfirma-ext/ff && cd /tmp/grxfirma-ext/ff && unzip -q ../grxfirma-extension-firefox-unsigned.xpi
npx web-ext lint --source-dir .
```

Hoy da 0 errores y un aviso sobre Firefox para Android, que se resuelve no marcando Android al subir (no admite mensajería nativa).

## 2. El ID de la extensión en Chrome y Edge

El host nativo solo acepta extensiones cuyo ID figure en `allowed_origins`. Hoy el código trae fijo `pkefjandjcgdmhoonmhnllikibobijgg`, pero esa clave privada no está en el repositorio ni en ninguna parte conocida, y el manifiesto Chromium no lleva campo `key`. Por eso Chrome Web Store asignará un ID nuevo en la primera subida, y Edge Add-ons asignará otro distinto (Edge siempre pone el suyo).

En Firefox no hay problema: el ID lo fija el manifiesto (`grxfirma@aavidad.github.io`) y AMO lo respeta.

Qué hacer:

1. Subir a Chrome Web Store y anotar el ID que aparece en el panel. Hacer lo mismo en Edge.
2. En el panel de Chrome, pestaña «Package», pulsar «View public key». Guardar esa clave pública (no es secreta). Si se añade como `"key"` en una copia de desarrollo del manifiesto, la extensión cargada sin empaquetar tendrá el mismo ID que la de la tienda. La clave privada la guarda Google; no hay que generar ni custodiar ninguna.
3. Abrir una tarea de código para fijar los dos IDs publicados donde hoy aparece `pkefjandjcgdmhoonmhnllikibobijgg`:
   - `cmd/nativehost/caller.go` (`officialChromiumExtensionID` y `builtinChromiumExtensionIDs`, añadir el de Edge);
   - `packaging/windows/install-nativehost.ps1` (orígenes por defecto; hoy los IDs de tienda solo entran por `GRXFIRMA_CHROMIUM_EXTENSION_ID` y `GRXFIRMA_EDGE_EXTENSION_ID` en el momento de instalar);
   - `packaging/linux/build-suite.sh` y `packaging/linux/configure-browsers.sh` (hoy solo admiten un ID adicional);
   - `packaging/macos/install-nativehost.sh` y el `Makefile` (`CHROME_EXT_ID`).
   Mantener `pkefjandjcgdmhoonmhnllikibobijgg` mientras haya instalaciones de prueba que lo usen.
4. Generar un instalador nuevo con esos IDs. Hasta entonces, la extensión de la tienda se instala pero el popup dirá que no puede conectar con GrxFirma.

Si se quiere conocer el ID antes de subir, la alternativa es crear ahora un par de claves propio, poner la clave pública en `"key"` y subir la primera versión con la privada como `key.pem` en la raíz del ZIP. Google lo ha admitido en el pasado, pero hay que comprobar en el panel que lo sigue aceptando y custodiar esa clave como material de firma (nunca en Git). La opción recomendada es la anterior.

## 3. Firefox Add-ons (AMO)

Coste: gratis.

1. Crear una cuenta Mozilla en https://addons.mozilla.org/developers/ y activar la verificación en dos pasos (AMO la exige para publicar). Aceptar el acuerdo de distribución.
2. «Submit a New Add-on». Elegir el canal:
   - «On this site» (listado): aparece en AMO y Mozilla lo revisa a mano. Recomendado para el público.
   - «On your own» (no listado): solo firma el XPI para distribuirlo con el instalador. Sirve para reservar el ID ya y publicar la ficha después.
   La primera subida, en cualquiera de los dos canales, reserva `grxfirma@aavidad.github.io` para esta cuenta.
3. Subir `grxfirma-extension-firefox-unsigned.xpi`. Plataforma: solo escritorio (desmarcar Android).
4. A «Do you use minified, concatenated or machine-generated code?» responder No. Explicación en las notas para revisores.
5. Ficha (solo canal listado): nombre, resumen de AMO y descripción de [FICHAS.md](../../packaging/browser-extensions/store/FICHAS.md), en español y, con «Manage localizations», en inglés. Categoría, correo de soporte, sitio web, licencia «Other» con enlace a `LICENSE`, URL de privacidad, icono (lo toma del XPI) y las tres capturas.
6. Recolección de datos: AMO la lee del manifiesto (`personallyIdentifyingInfo`, `websiteContent`). No hay que añadir nada.
7. «Notes to Reviewer»: pegar [NOTAS_REVISORES.md](../../packaging/browser-extensions/store/NOTAS_REVISORES.md) (basta la parte en inglés).
8. Enviar. Cuando Mozilla apruebe la versión, descargar el XPI firmado desde «Manage Status & Versions». Ese es el que deben llevar los instaladores (variable `GRXFIRMA_FIREFOX_SIGNED_XPI` de `build.py`).

## 4. Chrome Web Store

Coste: 5 USD, pago único por cuenta.

1. Entrar en https://chrome.google.com/webstore/devconsole con la cuenta de Google que será la propietaria. Pagar la cuota, aceptar el acuerdo y activar la verificación en dos pasos de la cuenta.
2. En «Account», completar correo de contacto verificado y la declaración de comerciante (trader) que pide la UE. Para una publicación personal y gratuita, lo normal es «non-trader»; si se publica en nombre de un organismo, consultar antes quién figura como editor.
3. «New item» y subir `grxfirma-extension-chromium.zip`.
4. Pestaña «Store listing»:
   - descripción de [FICHAS.md](../../packaging/browser-extensions/store/FICHAS.md) (el resumen y el nombre vienen del manifiesto);
   - categoría Productividad › Herramientas, idioma español;
   - icono de tienda `store/graficos/icono-128.png`;
   - capturas `store/capturas/es/01…03` (y las de `en/` en la ficha inglesa);
   - mosaico pequeño `store/graficos/mosaico-440x280-es.png`;
   - sitio web y URL de soporte.
5. Pestaña «Privacy practices»: copiar campo a campo la tabla de [FICHAS.md](../../packaging/browser-extensions/store/FICHAS.md) (función única, cada permiso, código remoto «No»), marcar las casillas de uso de datos y las tres certificaciones, y pegar la URL de privacidad.
6. Pestaña «Distribution»: gratuita, visibilidad pública o no listada, todas las regiones o solo España.
7. Pestaña «Test instructions»: pegar las notas para revisores en inglés.
8. «Submit for review». Se puede marcar «Defer publish» para publicar a mano tras la aprobación. El permiso opcional `https://*/*` puede alargar la revisión; la justificación ya explica que se pide sitio a sitio.
9. Anotar el ID asignado y seguir el apartado 2.

## 5. Microsoft Edge Add-ons

Coste: gratis.

1. Registrarse en el programa Microsoft Edge en Partner Center: https://partner.microsoft.com/dashboard/microsoftedge/ . Cuenta personal (individual); Microsoft verifica la identidad y puede tardar unos días.
2. «Create new extension» y subir el mismo `grxfirma-extension-chromium.zip`.
3. «Availability»: pública u oculta, y mercados.
4. «Properties»: categoría Productividad, URL de privacidad, sitio web, correo de soporte `avidad@dipgra.es`. A «¿accede a datos personales?» responder Sí.
5. «Store listings»: añadir español e inglés; descripción, resumen corto, logotipo 128 (el mismo icono), mosaico 440×280 y capturas.
6. «Submit». En «Notes for certification» pegar las notas para revisores en inglés.
7. Anotar el ID que asigna Edge y seguir el apartado 2.

## 6. Publicar una actualización

1. Subir la versión en los dos manifiestos (`src/chromium/manifest.json` y `src/firefox/manifest.json`) y la versión esperada en `verify_package.py`. Las tres deben coincidir. La versión de la extensión es independiente de la de la aplicación.
2. Si cambian textos visibles, actualizar los dos catálogos `_locales` y regenerar las capturas:

   ```bash
   npx --yes @puppeteer/browsers install chrome@stable --path /tmp/cft
   python3 packaging/browser-extensions/store/generar_capturas.py --chrome /tmp/cft/chrome/linux-*/chrome-linux64/chrome
   ```

   Chrome estable de marca ya no acepta `--load-extension`; por eso se usa Chrome for Testing.
3. Ejecutar las pruebas del apartado 1 y generar los paquetes.
4. Chrome y Edge: subir el ZIP nuevo en «Package» y enviar a revisión. El ID no cambia.
5. Firefox: el workflow `release.yml` (trabajo `build-firefox-extension`) firma el XPI con `web-ext sign --channel unlisted` usando los secretos `WEB_EXT_API_KEY` y `WEB_EXT_API_SECRET` del entorno `official-release`. Las claves se crean en AMO, «Tools › Manage API Keys».

   AMO no deja repetir un número de versión en ningún canal. Si la extensión está listada, firmar 1.2.0 como no listada en el workflow impide después subir 1.2.0 a la ficha pública. Hay que elegir una de estas formas de trabajo:
   - solo no listada: el workflow firma y los instaladores la llevan; AMO no muestra ficha;
   - listada: subir el XPI sin firmar a AMO a mano, esperar la aprobación, descargar el firmado y pasarlo al workflow o a `build.py` con `GRXFIRMA_FIREFOX_SIGNED_XPI`. Para esto el workflow necesitaría un ajuste (hoy siempre firma como no listada).

## 7. Revisión de permisos

Cada permiso se usa en el código:

| Permiso | Uso |
|---|---|
| `storage` | Sitios añadidos (`storage.local`), sitios fijados por la organización (`storage.managed`) y precarga del PDF durante cinco minutos (`storage.session`). |
| `nativeMessaging` | `ping` del popup y prueba de identidad con el host `io.github.aavidad.grxfirma`. |
| `scripting` | `registerContentScripts` y `unregisterContentScripts` del detector de PDF en los sitios que añade la persona usuaria. |
| `https://127.0.0.1/*` | Página del firmador local (`/signer`), su script puente y las llamadas del service worker al servicio local. |
| `https://*.dipgra.es/*`, `https://*.savia.net/*` | Portales de fábrica del botón de PDF y de la prueba de identidad. |
| `https://*/*` opcional | Solo se pide al añadir un sitio y se retira al quitarlo. |

No se declara `tabs` (abrir una pestaña con `tabs.create` no lo necesita), ni `externally_connectable`, ni `web_accessible_resources`.

Propuestas que no se han aplicado:

- En Chrome, los dominios de fábrica de `host_permissions` repiten los de `content_scripts`, que ya conceden ese acceso. Quitarlos no cambia el aviso de instalación y obligaría a repetir las pruebas en Firefox, donde los permisos de sitio se muestran y se revocan por separado. Se mantienen.
- Con `all_frames: true`, un PDF incrustado en una página de un dominio de fábrica muestra dos botones «Firmar PDF» (uno de la página y otro del marco del PDF). Conviene corregirlo en el detector en otra tarea.
- `src/*/icons/icon256.png` e `icon512.png` siguen siendo el logotipo antiguo. No entran en el paquete, pero conviene sustituirlos por el GRX de `assets/branding/` o borrarlos.

## Lista final para el responsable

1. Crear la cuenta de AMO (gratis, con verificación en dos pasos) y subir el XPI sin firmar: no listado para reservar el ID o listado para la ficha pública.
2. Crear la cuenta de Chrome Web Store (5 USD), completar la declaración de comerciante y subir el ZIP con la ficha y la pestaña de privacidad de `FICHAS.md`.
3. Registrarse en Partner Center para Edge (gratis, con verificación de identidad) y subir el mismo ZIP.
4. Comprobar que https://aavidad.github.io/GrxFirma/privacidad.html está publicada con la versión de este commit.
5. Anotar los IDs de Chrome y Edge y pedir la tarea de código del apartado 2.
6. Si se quiere que el workflow firme Firefox, crear las claves API de AMO y guardarlas como secretos del entorno `official-release`, y decidir entre listado y no listado (apartado 6).
