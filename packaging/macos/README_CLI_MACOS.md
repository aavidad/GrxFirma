<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma CLI para macOS

Este paquete contiene la versión de consola de `GrxFirma` para macOS.

## Qué incluye

- `grxfirma`
- `README_CLI_MACOS.md`
- `VERSION.txt`

## Qué permite

- firmar desde terminal;
- verificar firmas;
- procesar lotes;
- listar y comprobar certificados;
- gestionar dominios de confianza;
- operar con certificados del Keychain cuando corresponda.

## Construcción

Debe construirse desde un macOS real o desde Linux con una toolchain Darwin
cruzada configurada en `CC` y `CXX` (por ejemplo `osxcross`). Sin eso, el
script aborta antes de compilar.

```bash
./packaging/macos/build-cli.sh
```

Si lo ejecutas en Linux sin toolchain Darwin, el bloqueo real es este:

```text
error: el build macOS requiere macOS real o una toolchain Darwin cruzada configurada en CC/CXX.
       En Linux, exporta una toolchain tipo osxcross antes de ejecutar este script.
```

## Instalación manual

```bash
chmod +x grxfirma
./grxfirma -ayuda
```

## Observaciones

- Esta entrega es la CLI, no la app desktop completa.
- La ayuda visible del binario está en castellano.
- El acceso al Keychain depende de los permisos del usuario y del propio macOS.

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autoría: Alberto Avidad Fernández

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
