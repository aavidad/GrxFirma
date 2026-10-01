<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Política de autoría y licencia

La licencia del código propio de GrxFirma es la EUPL 1.2. Conforme a su
artículo 5, esta licencia permite distribuir bajo la versión 1.2 o una versión
posterior salvo que se declare expresamente «EUPL v. 1.2 solamente». El
identificador SPDX correcto es `EUPL-1.2`.

El fichero raíz `LICENSE` reproduce el anexo oficial español de la
[Decisión de Ejecución (UE) 2017/863](https://eur-lex.europa.eu/legal-content/ES/TXT/?uri=CELEX:32017D0863).

La cabecera canónica, adaptada a la sintaxis de comentarios de cada formato,
contiene exactamente:

```text
Derechos de autor (C) 2026 Alberto Avidad Fernández.
Autoría: Alberto Avidad Fernández
Licencia: EUPL 1.2 o posterior
SPDX-License-Identifier: EUPL-1.2
```

La clasificación exhaustiva está en `legal/licensing.toml`. Los JSON,
binarios, recursos, resultados generados y formatos sin comentarios reciben
la misma autoría y licencia mediante la cobertura central declarada allí. No
se modifica su contenido.

El código vendorizado conserva sus titulares y licencias:

- `third_party/pdfsign/**`: BSD-2-Clause, copyright de Digitorus.
- Gradle Wrapper de Android: Apache-2.0, copyright de sus autores originales.

## Comprobación

La migración reproducible se puede previsualizar sin escribir:

```sh
python3 scripts/compliance/migrate_legal_headers.py --check
```

Para normalizar las cabeceras de todas las rutas propias versionadas que
admiten comentarios:

```sh
python3 scripts/compliance/migrate_legal_headers.py
```

El migrador conserva el BOM y los saltos de línea, mantiene en primera
posición los *shebangs*, declaraciones XML, `DOCTYPE`, restricciones de
compilación Go y *front matter*, y no modifica ninguna categoría cubierta
centralmente ni código de terceros. Una segunda ejecución no produce cambios.

Durante la migración de cabeceras se puede validar el inventario, la cobertura
central, la licencia raíz y los avisos de terceros:

```sh
python3 scripts/compliance/check_legal_headers.py --inventory-only
```

El modo normal es estricto y exige la cabecera canónica en cada fichero propio
que admite comentarios. También comprueba que la sección de licencia del
`README.md` declara EUPL 1.2 o posterior y no reproduce la antigua declaración
contradictoria GPLv3. El mismo gate cubre la ayuda CLI y los manuales de
distribución de Linux, Windows y macOS:

```sh
python3 scripts/compliance/check_legal_headers.py
```

Una extensión o ruta nueva que no encaje en una categoría conocida hace fallar
la comprobación. El inventario procede exclusivamente de `git ls-files -z`;
los artefactos ignorados y los ficheros todavía no añadidos al índice no
forman parte de la auditoría.
