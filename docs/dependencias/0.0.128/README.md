<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Expediente de dependencias 0.0.128

Preparación del 15/11/2026 sobre `b1987f3a7717701c4e23029cd0081421270d9033`.
La versión está fijada en fuentes. Tag, commit de publicación, artefactos,
hashes y aceptación de Alberto: pendientes.

## Generación bloqueada por revisiones previas pendientes

La [plantilla de cierre](../proxima/README.md) pide abrir un expediente al fijar
la versión. Se ha intentado generar el inventario sin red:

```sh
GOPROXY=off GOSUMDB=off python3 scripts/compliance/dependency_dossier.py \
  --version 0.0.128 --output docs/dependencias/0.0.128/inventario.json
```

Resultado: salida 1, `Expediente incompleto: declaración cambiada: revisar
licencia y uso de cmd/gui-qml/translatorbridge_test.pro`.
No se ha creado `inventario.json` ni se ha saltado la validación.

La [comparación local](evidencias/preparacion.json) recoge los hashes de las
cuatro fuentes distintas del expediente anterior, las dos declaraciones sin
revisión vigente y las identidades añadidas. La recogida mecánica cuenta
702 identidades y versiones en 39 fuentes, frente a 690 en 38 fuentes.
De las 12 altas, la copia legal de NSIS ya tiene revisión; las 11 de Maven
siguen sin entrada en `legal/dependency-review.json`. La prueba
`test_current_inventory_and_baseline_debt_are_visible` falla por la primera:
`maven:androidx.test.espresso:espresso-accessibility@3.7.0`.
Son diferencias ya presentes en la base de este lote. El cambio de versión
no modifica dependencias ni declaraciones Gradle/Qt/WinUI.

El generador sigue usando la lista congelada
`legal/dependency-license-pending-0.0.127.json`: identifica deuda inicial,
no la versión actual del producto. No se crea una lista 0.0.128 que pueda
confundirse con una nueva exención ni se amplían las 38 identidades pendientes.
Antes de generar el inventario hay que revisar las 11 identidades Maven y
las dos declaraciones, incorporar su evidencia al registro y repetir la orden.

## Publicación y comprobaciones pendientes

`scripts/release` y `.github/workflows/release.yml` no exigen un fichero
`dependency-license-pending-<versión>.json` ni consumen este expediente por
ruta. El workflow exige su propia SBOM SPDX mediante Syft, además de los
controles de firma, artefactos y tag oficial que coincide con `VERSION.txt`.
Este expediente abierto no acredita una publicación ni sustituye esos controles.

No se han ejecutado Syft, OSV, govulncheck ni Grype en este lote, ni descargado
bases. Los informes del [expediente anterior](../0.0.127/README.md) conservan
su fecha y alcance; no se presentan como resultados de la 0.0.128.
Pendiente adjuntar SPDX de árbol y runtime, identificar bases locales y guardar
los nuevos informes con sus hashes y códigos de salida. Si una base falta,
ese ecosistema debe figurar como no comprobado.

También quedan pendientes el triage de `GO-2026-5932`, los módulos OpenPGP,
los alcances por plataforma y la excepción `GHSA-gcqx-w48m-w2h4`, cuya fecha
de caducidad sigue siendo 2027-02-14. No se prorroga ni se acepta ningún riesgo.
Faltan los resultados de interoperabilidad de esta versión y DSS local.
Alberto debe determinar responsables, soporte, plazos, backports y aceptación.
El expediente permanece abierto hasta completar esas revisiones.
