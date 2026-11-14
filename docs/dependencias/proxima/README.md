<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Expediente de la próxima versión — pendiente de abrir

Copiar esta plantilla a `docs/dependencias/<versión>/` al fijar la versión.
No sobrescribir la revisión ni los informes de 0.0.127.

- Versión, tag, commit y hashes de artefactos: pendientes.
- Generar `inventario.json` con `dependency_dossier.py --version <versión>`.
- Revisar diferencias de dependencias, usos, licencias, declaraciones y avisos.
  Resolver las identidades nuevas antes de incorporar su propuesta al registro.
- Adjuntar SPDX de árbol y runtime generados por los controles existentes.
- Guardar bases locales identificadas, informes y códigos de salida de
  govulncheck (aplicación y pdfsign), OSV (todos los ecosistemas cubiertos) y
  Grype; declarar los que falten o no sean actuales.
- Revisar `GO-2026-5932`, módulos OpenPGP y cambios de alcance por plataforma.
- Revisar `GHSA-gcqx-w48m-w2h4`: caduca el **2027-02-14**, sin prórroga automática.
- Registrar correcciones, regresiones y backports con versión e incidencia.
- Ejecutar [interoperabilidad local](../../../test/conformance/README.md) y
  adjuntar resultados; DSS sigue pendiente hasta disponer de instalación local.
- Alberto: asignar responsable de triage, aceptar/rechazar cada riesgo,
  determinar soporte, plazos de respuesta y backports. Todo pendiente hasta
  decisión expresa fechada; no se hereda una aceptación de otra versión.
- Cierre técnico, aceptación humana y fecha: pendientes.
