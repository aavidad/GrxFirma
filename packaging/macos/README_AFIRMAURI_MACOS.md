<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma handler afirma:// para macOS

Este paquete contiene el manejador protocolario `afirma://` para macOS.

## Qué incluye

- `GrxFirma AfirmaURI.app`
- `install-afirmauri.sh`
- `README_AFIRMAURI_MACOS.md`
- `VERSION.txt`

## Para qué sirve

Permite registrar en macOS el esquema `afirma://` para que portales y sedes electrónicas puedan invocar `GrxFirma`.

En la práctica:

- el navegador o el sistema abre una URI `afirma://...`;
- LaunchServices entrega la URI a la aplicación registrada;
- `GrxFirma AfirmaURI.app` ejecuta el flujo protocolario y dialoga con el portal remoto.

## Instalación

```bash
chmod +x install-afirmauri.sh
./install-afirmauri.sh
```

El script:

- copia `GrxFirma AfirmaURI.app` a `~/Applications`;
- fuerza el registro de la app en LaunchServices;
- deja activado el esquema `afirma://` para el usuario.

## Diagnóstico

El paquete se compila con la etiqueta `production` y no acepta la activación
de `DEBUG` por variables de entorno. Para soporte debe usarse la incidencia
saneada de la aplicación. Las trazas detalladas quedan reservadas a una build
de desarrollo separada, con datos sintéticos y borrado al terminar.

## Construcción

Debe construirse desde un macOS real o desde Linux con una toolchain Darwin
cruzada configurada en `CC` y `CXX` (por ejemplo `osxcross`):

```bash
./packaging/macos/build-afirmauri.sh
```

Si lo ejecutas en Linux sin toolchain Darwin, el script aborta con:

```text
error: el build macOS requiere macOS real o una toolchain Darwin cruzada configurada en CC/CXX.
       En Linux, exporta una toolchain tipo osxcross antes de ejecutar este script.
```

## Observaciones

- Este paquete no instala el `nativehost` del navegador.
- El registro del protocolo se hace a través de una `.app` con `Info.plist`, no mediante registro tipo Windows.

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autor: Oficina de Software Libre de la Diputacion de Granada.
- Alberto Avidad Fernandez
- Oficina de Software Libre - Diputación de Granada

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
