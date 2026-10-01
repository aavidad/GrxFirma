<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Construcción y publicación

`VERSION.txt` contiene la versión de la aplicación. El script
`scripts/nueva-version.sh` prepara la siguiente versión y abre su sección en
[docs/NOVEDADES.md](NOVEDADES.md). Completa esa sección antes de construir un
candidato. Conserva cada candidato y sus sumas SHA-256 por separado.

## Comprobación del código fuente

```bash
go build ./...
go test ./...
```

El equipo de construcción necesita las dependencias de cada plataforma ya
instaladas. Los comandos anteriores cubren los paquetes Go disponibles en el
sistema actual; los componentes Qt, WinUI y móviles tienen sus propias puertas.

## Paquetes locales

| Plataforma | Construcción |
|---|---|
| Linux | `./packaging/linux/build-suite.sh` o `./packaging/linux/build-suite.sh --deb` |
| Windows | `./packaging/windows/build-suite.sh --with-winui --nsis` |
| macOS | `./packaging/macos/build-suite.sh --pkg` |

Las opciones de cada plataforma se explican en los README de `packaging/`.
Agrupa los artefactos y genera el inventario con:

```bash
./scripts/release/assemble_installers.sh
```

Verifica `SHA256SUMS.txt`, `manifest.json` y las versiones internas de los
binarios e instaladores resultantes. Los paquetes producidos localmente son
candidatos técnicos. La publicación oficial requiere las firmas y comprobaciones
descritas en [RELEASE_SIGNING.md](RELEASE_SIGNING.md).

## Publicación oficial

El workflow `.github/workflows/release.yml` valida la versión del tag, construye
los paquetes, firma los artefactos con las credenciales configuradas por el
responsable de la publicación, verifica su contenido y adjunta el inventario y
el SBOM al GitHub Release. Las credenciales se entregan al workflow mediante el
almacén de secretos del proveedor de CI; no forman parte del repositorio.

Antes de anunciar una versión, comprueba funcionalmente los paquetes finales
en las plataformas y canales que se vayan a distribuir. Las pruebas sobre otro
candidato no acreditan el contenido de uno nuevo.
