<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma CLI para Windows

Este paquete contiene la versión de consola de GrxFirma para Windows.

## Qué incluye

- `grxfirma.exe`
- `README_CLI_WINDOWS.md`
- `VERSION.txt`

Si se compila el instalador NSIS, la entrega puede incluir ademas:

- `GrxFirma-0.0.90-cli-windows-amd64-setup.exe`

El ZIP portable es `GrxFirma-0.0.90-cli-windows-amd64.zip`.

## Qué permite

- Firmar desde línea de comandos.
- Verificar firmas.
- Procesar lotes.
- Listar y comprobar certificados.
- Gestionar dominios de confianza.
- Gestionar TLS local desde CLI cuando aplique.

## Ejemplos

Mostrar ayuda:

```powershell
.\grxfirma.exe -ayuda
```

Listar certificados:

```powershell
.\grxfirma.exe -modo-cli -listar-certificados -salida-json
```

Informe de diagnóstico:

```powershell
.\grxfirma.exe -modo-cli -operacion informe-diagnostico -salida-json
```

Firmar un documento:

```powershell
.\grxfirma.exe -contenedor-p12 C:\ruta\certificado.p12 -contrasena-stdin -entrada C:\ruta\documento.pdf -salida C:\ruta\firma.csig -formato cades
```

La aplicación solicita la contraseña sin eco. No la escriba como argumento:
quedaría expuesta en el historial y en la lista de procesos.

Verificar:

```powershell
.\grxfirma.exe -modo-cli -operacion verificar -entrada C:\ruta\firma.csig -documento-original C:\ruta\documento.pdf -salida-json
```

## Observaciones

- Esta entrega es la CLI de Windows, no la app desktop completa.
- Las cabeceras y errores base de la ayuda respetan el idioma del entorno; los
  cuerpos descriptivos todavía conservan texto en castellano.
- El soporte Windows usa el almacén de certificados del sistema cuando procede.
- El instalador NSIS no requiere elevación: copia la CLI en
  `%LOCALAPPDATA%\Programs\GrxFirma\CLI` y registra la desinstalación en
  `HKCU`.
- Las versiones antiguas del NSIS escribían la CLI o la Suite en `HKLM`. El
  instalador actual revisa las vistas de registro de 32 y 64 bits y, si
  encuentra una de esas instalaciones por equipo, se detiene antes de cambiar
  nada. Debe desinstalarse primero desde **Configuración > Aplicaciones**
  aceptando UAC, o con su `uninstall.exe` ejecutado como administrador; después
  se instala la edición por usuario.

## Construcción del paquete

Desde Linux:

```bash
./packaging/windows/build-cli.sh
./packaging/windows/build-cli.sh --nsis
```

Desde Windows con PowerShell:

```powershell
.\packaging\windows\build-cli.ps1
.\packaging\windows\build-cli.ps1 --nsis
```

La ruta shell actual soporta hoy:

- `GOARCH=amd64`

## Documentación relacionada

En Linux existen páginas `man` más extensas. Como referencia temática:

- `grxfirma-mejoras`
- `grxfirma-certificados`
- `grxfirma-operacion`
- `grxfirma-protocolos-web`

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autoría: Alberto Avidad Fernández

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
