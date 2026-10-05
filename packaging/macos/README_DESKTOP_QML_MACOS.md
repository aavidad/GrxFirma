<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma Desktop Qt/QML para macOS

Este paquete contiene la app desktop manual de `GrxFirma` basada en Qt/QML para macOS.

## Qué incluye

- `GrxFirma Desktop Qt.app`
- `install-desktop-qml.sh`
- `README_DESKTOP_QML_MACOS.md`
- `VERSION.txt` bajo `Contents/Resources`

Y dentro de la app:

- frontend Qt/QML;
- bootstrap IPC `grxfirma-gui`;
- backend `grxfirma`;
- directorios `qml/` y `assets/` bajo `Contents/Resources`;
- frameworks y plugins desplegados por `macdeployqt`.

En la suite oficial esos Mach-O y bundles Qt se firman de dentro hacia fuera:
primero ejecutables, frameworks y plugins anidados, y por último la `.app`.
Este orden mantiene válido el sello del contenedor para Gatekeeper y
notarización.

## Para qué sirve

Es la app manual de escritorio para macOS:

- abrir documentos;
- seleccionar certificado;
- firmar;
- verificar;
- operar principalmente por `IPC` local con el backend de `GrxFirma`;
- usar opcionalmente la REST local como superficie técnica y de soporte.

Si el backend REST local está activo, expone también:

- `https://127.0.0.1:63118/` como consola técnica local;
- `https://127.0.0.1:63118/signer` como firmador web local.

Capacidades visibles actuales de esas superficies web:

- firma de uno o varios ficheros con `/sign-batch`;
- sello visible `PAdES`;
- sello en una página concreta, en rangos como `1,3-5` o en todas las páginas con `all`.

## Requisitos de construcción

Debe construirse desde un macOS real. Una toolchain Darwin cruzada puede
compilar partes Go, pero no sustituye el despliegue final de la `.app` con:

- Qt instalado;
- `qmake` en `PATH`;
- `macdeployqt` en `PATH`;
- compilador compatible con el kit Qt usado.

## Construcción

```bash
./packaging/macos/build-desktop-qml.sh
```

Si lo ejecutas fuera de macOS, el script aborta antes de empaquetar porque
`macdeployqt` no ofrece aquí una ruta de cross-build completa. Sin toolchain
Darwin, el primer diagnóstico es:

```text
error: el build macOS requiere macOS real o una toolchain Darwin cruzada configurada en CC/CXX.
       En Linux, exporta una toolchain tipo osxcross antes de ejecutar este script.
```

## Instalación manual

```bash
chmod +x install-desktop-qml.sh
./install-desktop-qml.sh
```

El script valida que la aplicación contenga el frontend, el bootstrap IPC y el
backend, rechaza
orígenes simbólicos y sustituye la instalación mediante un directorio temporal
con restauración de la copia anterior si falla el cambio. En concreto:

- copia la app a `~/Applications`;
- reemplaza de forma atómica una copia previa del mismo nombre;
- exige un `HOME` absoluto y no simbólico;
- no sigue enlaces simbólicos en la aplicación de destino ni en sus directorios
  antecesores.

## Observaciones

- Este paquete no registra por sí solo `afirma://`.
- La suite y el PKG finales incluyen esta aplicación; este paquete separado se
  conserva para pruebas o despliegues manuales del componente.
- Está orientado a escritorio manual, no a portales web legacy.
- El bundle mantiene juntos `grxfirma-gui-qml`, `grxfirma-gui` y
  `grxfirma`. Finder abre `grxfirma-gui-qml` porque es el
  `CFBundleExecutable`; el frontend invoca después `grxfirma-gui --server`
  para prestar IPC. Si se ejecuta el bootstrap manualmente sin `--server`, este
  sí arranca el frontend Qt. El backend REST `grxfirma` permanece disponible
  en el mismo directorio interno.
- En escritorio manual el modo preferente es `IPC`; la REST local queda como vía experta, consola web local y soporte.

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autoría: Alberto Avidad Fernández

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
