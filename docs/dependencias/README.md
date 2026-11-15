<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Expediente técnico por versión

El [expediente 0.0.127](0.0.127/README.md) relaciona el inventario mecánico con
las licencias acreditadas, el uso, la procedencia, las incidencias revisadas y
los límites de interoperabilidad. La [próxima versión](proxima/README.md) tiene
una plantilla de cierre. La aceptación y los compromisos corresponden a Alberto.

## Regenerar y comprobar sin red

```sh
export GOCACHE="$HOME/.cache/grxfirma-gocache" GOPROXY=off GOFLAGS=-mod=readonly
python3 scripts/compliance/dependency_dossier.py --version 0.0.127 \
  --output /tmp/expediente-0.0.127.json
python3 -m unittest discover -s scripts/compliance/tests -v
```

Se leen los `go.mod` y `go.sum`, los lockfiles npm, Gradle y NuGet, las
referencias de proyectos WinUI, los módulos Qt de los `.pro`, el wrapper y el
inventario `legal/licensing.toml`. Se reutilizan las configuraciones Android de
`prepare_runtime_sbom_scope.py`, el filtro de lockfiles y las bases de terceros
de `third_party_bases.py`. Las declaraciones Gradle/Qt/WinUI tienen un hash
revisado: cambiarlas exige revisar su clasificación aunque aún no cambie el
lockfile. Una identidad o versión nueva sin revisión, una licencia ausente sin
pendiente inicial explícito, o un uso sin clasificar hacen fallar el control.
La prueba lo comprueba con cambios deliberados, incluida una versión nueva.

La lista congelada `legal/dependency-license-pending-0.0.127.json` impide que
el generador extienda esta deuda a identidades nuevas.

`legal/dependency-review.json` conserva la revisión técnica exacta por identidad
y versión. Las 38 licencias pendientes iniciales son deuda visible, acotada a
esas identidades, sin aceptación de riesgo ni autorización de distribución.
`--strict` exige cerrarlas y hoy falla. El control ordinario permite conservar
esa deuda inicial para no bloquear usos existentes; no extiende la salvedad a
nuevas dependencias. No hay evaluación automática de compatibilidad jurídica.

El inventario fuente es deliberadamente conservador: Go indirecto se cuenta
como ejecución salvo los auxiliares de pruebas identificados y PKCS#11
experimental, marcado opcional. La ocurrencia de `third_party/pdfsign/go.mod`
conserva sus versiones propias; al construir la aplicación prevalecen las de
`go.mod` raíz por MVS. `go.sum` acredita hashes, incluye entradas históricas y
no demuestra por sí solo que un módulo se use. `go list -m all` no pudo
resolver el grafo completo sin red en este equipo (`module lookup disabled
by GOPROXY=off`); por eso no se presenta este complemento de manifiestos
como cierre del grafo entero ni se usa el export antiguo de `docs/generated`
como evidencia actual. Los `replace` apuntan a copias
locales cuyo aviso y base upstream se conservan por separado. Qt no fija el kit:
no se atribuye la versión de este equipo a los binarios publicados. NuGet usa
la versión resuelta del lockfile y su `contentHash`.

## Actualizar evidencia de licencias

Se complementa Syft, que ya usa CI, con metadatos de las cachés locales. Esta
orden no consulta servicios ni ejecuta Gradle/npm:

```sh
SYFT_CHECK_FOR_APP_UPDATE=false \
SYFT_GOLANG_SEARCH_LOCAL_MOD_CACHE_LICENSES=true \
SYFT_GOLANG_SEARCH_REMOTE_LICENSES=false \
  syft dir:. -o syft-json=/tmp/syft-local.json -q
python3 scripts/compliance/license_candidates.py \
  --syft-json /tmp/syft-local.json \
  --gradle-cache "$HOME/.gradle/caches/modules-2/files-2.1" \
  --nuget-cache "$HOME/.nuget/packages" \
  --go-root "$(go env GOROOT)" \
  --output /tmp/revision-propuesta.json
```

Hay que revisar la propuesta y su diferencia antes de actualizar el registro.
No usarla para aceptar automáticamente pendientes nuevos. Los POM se leen con
sus padres locales cuando heredan licencia; se conservan nombre/URL declarados
y SHA-256 de cada evidencia, sin deducir SPDX por el prefijo del proveedor.
Las licencias de npm vienen del lockfile y las Go de Syft/caché o del aviso
local en los reemplazos. Las expresiones `sha256:…` de Syft identifican textos
sin SPDX reconocido (por ejemplo, recursos de fuente de Fyne): su interpretación
y los avisos de redistribución siguen pendientes de revisión humana.
Los hashes de POM y las rutas normalizadas permiten cotejar evidencias; los POM
no se copian al expediente. Conservarlos en el archivo de construcción antes
de purgar la caché. `syft-local.json.gz` contiene la salida nativa completa;
el hash que citan las licencias corresponde al JSON descomprimido.

Este complemento no sustituye SPDX, el gate del runtime ni la enumeración de
CI/release. Acciones GitHub, requisitos Python auxiliares y SDK de plataforma
que detecte la SBOM completa siguen en ella; no se declaran cubiertos por la
clasificación de los ecosistemas de este lote. El SDK Android/.NET, el kit Qt,
las bibliotecas gráficas nativas de Fyne y las herramientas instaladas deben
atestarse también sobre cada artefacto y plataforma distribuida.

## Escaneos locales

Usar únicamente bases ya presentes. No ejecutar opciones de descarga.

```sh
python3 scripts/compliance/prepare_govulncheck_db.py \
  --archive "$HOME/.cache/osv-scanner/Go/all.zip" \
  --output /tmp/govulndb-release
# govulncheck 1.6.0 admite un directorio plano de entradas GO oficiales.
govulncheck -db file:///tmp/govulndb-release -json ./... > /tmp/govulncheck.json
(cd third_party/pdfsign && \
  govulncheck -db file:///tmp/govulndb-release -json ./...) > /tmp/govulncheck-pdfsign.json
osv-scanner scan source --offline --no-call-analysis go --format json \
  -L go.mod -L third_party/pdfsign/go.mod \
  -L packaging/browser-extensions/tools/package-lock.json > /tmp/osv.json
GRYPE_CHECK_FOR_APP_UPDATE=false GRYPE_DB_AUTO_UPDATE=false \
  grype sbom:/tmp/syft-local.json -o json > /tmp/grype.json
```

El exportador rechaza una base vacía o un destino no vacío y conserva intactas
las entradas GO oficiales. No convierte los GHSA a avisos Go ni fabrica fechas.
Registrar fecha, SHA-256 de bases e informes, alcance, código de salida y
revisión. Si una herramienta falla, su resultado es «no comprobado», nunca
«sin vulnerabilidades». La orden Grype anterior explora el árbol fuente; el
gate de distribución existente usa una SBOM de runtime separada. Añadir OSV
Maven/NuGet solo con sus bases locales disponibles, sin caer a consultas online.
