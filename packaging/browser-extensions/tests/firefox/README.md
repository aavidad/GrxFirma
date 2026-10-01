<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Firefox Native Messaging E2E (Linux)

Esta prueba usa Firefox y geckodriver reales. No simula `browser.runtime` ni el
protocolo Native Messaging.

Recorrido verificado:

1. compila `cmd/nativehost` con Go y la etiqueta cerrada `production`, o
   valida y usa el host precompilado indicado para probar un paquete instalado;
2. genera un XPI temporal con el mismo empaquetador de produccion;
3. registra `com.dipgra.grxfirma` en
   `.mozilla/native-messaging-hosts` del home real de la cuenta del sistema;
4. instala temporalmente `extension@dipgra.es` en un perfil nuevo;
5. abre el `popup.html` real desde el contexto privilegiado de automatizacion y
   vuelve al contexto de contenido para inspeccionar el DOM;
6. exige respuesta de `ping` y de `getCertificates`;
7. verifica en `/proc` el ejecutable Go lanzado y los argumentos que Firefox
   obtiene del manifiesto.

## Ejecucion

Requisitos: Linux, Go compatible con `go.mod`, Firefox >= 140 y geckodriver.

```bash
packaging/browser-extensions/tests/firefox/run-e2e.sh
```

Se pueden seleccionar binarios concretos:

```bash
FIREFOX_E2E_GO="$HOME/go/bin/go1.26.5" \
FIREFOX_E2E_GECKODRIVER=/ruta/geckodriver \
packaging/browser-extensions/tests/firefox/run-e2e.sh
```

Para recorrer exactamente el host instalado, el arnés copia el binario a su
directorio temporal y rechaza la ejecución si su metainformación Go no contiene
la etiqueta `production`:

```bash
FIREFOX_E2E_NATIVE_HOST=/usr/lib/grxfirma/bin/grxfirma-nativehost \
packaging/browser-extensions/tests/firefox/run-e2e.sh
```

La prueba es `headless` por defecto. `--headed` conserva la misma validacion con
ventana visible y `--keep-workdir` mantiene los artefactos temporales para
diagnostico.

Firefox actual impide navegar directamente a una URL `moz-extension://` desde
el endpoint WebDriver `/url`. El arnes inicia su perfil desechable con
`-remote-allow-system-access`, cambia brevemente al contexto `chrome`, abre la
URL con `openTrustedLinkIn` y vuelve siempre a `content`, incluso si la
navegacion falla. Este acceso privilegiado solo se habilita en la instancia
temporal de la prueba.

Las pruebas unitarias del registro y de esta secuencia se ejecutan con:

```bash
python3 -m unittest discover \
  -s packaging/browser-extensions/tests/firefox \
  -p 'test_*.py'
```

## Firefox Snap

Firefox confinado usa el portal XDG WebExtensions. En modo automatizado, Ubuntu
identifica geckodriver como el subprograma `snap.firefox_geckodriver`; no como
`snap.firefox`. La prueba concede temporalmente ambos permisos mediante
`flatpak permission-set` y restaura el valor anterior al terminar.

El manifiesto de usuario tambien se sustituye de forma atomica bajo bloqueo y
se restaura byte a byte, incluidos sus permisos. No se toca el perfil habitual
de Firefox. El arnes obtiene el home real mediante la base de cuentas del
sistema: asi sigue registrando el host donde Firefox lo busca aunque el proceso
que lanza la prueba tenga `HOME` redefinido por un contenedor o un runner.

Para una instalacion de usuario normal, la primera conexion muestra un dialogo
del portal. Si el operador ha autorizado expresamente la integracion y el
dialogo no aparece, la autorizacion equivalente es:

```bash
flatpak permission-set webextensions com.dipgra.grxfirma snap.firefox yes
```

Puede auditarse con `flatpak permissions webextensions` y revocarse con:

```bash
flatpak permission-remove webextensions com.dipgra.grxfirma snap.firefox
```

## Limites

La carga temporal acepta un XPI sin firma AMO. Esta prueba valida el codigo y el
canal Native Messaging, no la firma de distribucion, la publicacion en AMO ni
una operacion criptografica de firma. `getCertificates` si atraviesa el host Go
y el almacen de certificados real del usuario, pero el resultado solo publica
el numero de certificados para no exponer identidades en los logs.
