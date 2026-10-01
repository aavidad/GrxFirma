<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Chromium Native Messaging E2E (Linux)

Esta prueba usa navegadores Chromium reales mediante Playwright. No simula
`chrome.runtime`, `runtime.connectNative` ni el protocolo Native Messaging.

Recorrido verificado:

1. compila `cmd/nativehost` con Go y la etiqueta cerrada `production`, o
   valida y usa el host precompilado indicado para probar un paquete instalado;
2. valida y descomprime el ZIP Chromium generado por produccion;
3. carga la extension MV3 en un perfil temporal;
4. registra `com.dipgra.grxfirma` dentro del perfil y de un
   `XDG_CONFIG_HOME` aislado;
5. exige que un host inexistente produzca el error esperado;
6. ejecuta `runtime.connectNative`, `ping` y `getCertificates` reales;
7. verifica en `/proc` el binario Go y el origen `chrome-extension://.../`;
8. abre el `popup.html` real y comprueba su estado y certificados;
9. cierra procesos, elimina perfiles y confirma que los manifiestos habituales
   del usuario no cambiaron.

Los resultados solo publican el numero de certificados, nunca sus identidades.

## Requisitos

- Linux y Go compatible con `go.mod`.
- Python 3.11 o posterior.
- Playwright para Python 1.55.0 con su driver Node de la misma version. La
  instalacion aislada y reproducible del arnes se prepara con:

```bash
packaging/browser-extensions/tests/chromium/install-playwright.sh
```

`run-e2e.sh` usa primero un Playwright 1.55.0 completo del sistema y despues
el entorno cacheado por el instalador. Esto evita la instalacion global y
tambien detecta paquetes de distribucion incompletos, por ejemplo un binding
Python que apunte a un `cli.js` ausente. `CHROMIUM_E2E_PYTHON` permite fijar
otro entorno ya validado. El instalador exige versiones y hashes fijados en
`requirements.txt`; ese manifiesto forma parte del inventario y del gate OSV
de tooling de CI.

Google Chrome estable bloquea por diseno la carga automatizada con
`--load-extension`. Para probar el mismo canal Chrome de forma reproducible se
usa Chrome for Testing oficial:

```bash
npx --yes @puppeteer/browsers install chrome@stable \
  --path "$HOME/.cache/grxfirma-browsers"
```

El arnes descubre automaticamente la version mas reciente instalada en esa
ruta. Chromium y Brave se detectan desde `PATH`.

## Ejecucion

Validacion local del arnes, sin abrir navegador:

```bash
python3 -m unittest discover \
  -s packaging/browser-extensions/tests/chromium \
  -p 'test_*.py'
```

Prueba todos los navegadores disponibles, en este orden: Chrome estable como
sonda de la restriccion, Chrome for Testing, Chromium y Brave.

```bash
packaging/browser-extensions/tests/chromium/run-e2e.sh --browser all
```

Seleccion individual:

```bash
packaging/browser-extensions/tests/chromium/run-e2e.sh --browser chrome
packaging/browser-extensions/tests/chromium/run-e2e.sh --browser chromium
packaging/browser-extensions/tests/chromium/run-e2e.sh --browser brave
```

Se pueden fijar ejecutables y el paquete:

```bash
CHROMIUM_E2E_CHROME=/ruta/chrome-for-testing \
CHROMIUM_E2E_CHROMIUM=/ruta/chromium \
CHROMIUM_E2E_BRAVE=/ruta/brave \
CHROMIUM_E2E_GO="$HOME/go/bin/go1.26.5" \
CHROMIUM_E2E_PACKAGE=/ruta/extension.zip \
packaging/browser-extensions/tests/chromium/run-e2e.sh --browser all
```

Para probar exactamente el host de una instalación, el arnés copia el binario
a su directorio temporal y comprueba con la metainformación Go que fue
compilado con `production`:

```bash
CHROMIUM_E2E_NATIVE_HOST=/usr/lib/grxfirma/bin/grxfirma-nativehost \
packaging/browser-extensions/tests/chromium/run-e2e.sh --browser all
```

Para cambiar el cache sin alterar `HOME`:

```bash
CHROMIUM_E2E_VENV=/ruta/absoluta/playwright-venv \
packaging/browser-extensions/tests/chromium/install-playwright.sh
```

La prueba es `headless` por defecto. `--headed` abre ventanas reales y
`--keep-workdir` conserva los temporales para diagnostico.

## Aislamiento y limites

El ZIP se carga descomprimido porque Chrome estable solo admite una instalacion
automatizada publicada o gestionada por politica. Chrome for Testing existe
precisamente para automatizacion y conserva el motor Chrome oficial. La sonda
de Chrome estable aparece como `limited`, no como un falso fallo de la app.
El Chromium distribuido como Snap tambien puede impedir la carga de una
extension desempaquetada o la ejecucion de un host nativo fuera de su
confinamiento. El arnes lo identifica como `limited`; usa el paquete Chromium
de la distribucion, Brave o Chrome for Testing para completar el E2E. Una
ejecucion que solo encuentre navegadores limitados termina con codigo `77`.

El arnes valida enumeracion de certificados, no realiza una firma documental.
La distribucion final en Chrome Web Store y las firmas del paquete quedan fuera
de esta prueba.

Referencias de Chromium:

- [restriccion de `--load-extension`](https://chromium.googlesource.com/chromium/src/+/04f6233ce5be7e5e420418b5286f3b0f87ffc28f%5E%21/)
- [API Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging)
