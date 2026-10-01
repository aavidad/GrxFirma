<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

<!--
Derechos de autor (C) 2026 Alberto Avidad Fernández.
Autoría: Alberto Avidad Fernández.
Licencia: EUPL 1.2 o posterior.
SPDX-License-Identifier: EUPL-1.2
-->

# Automatización del repositorio

Esta carpeta es parte intencionada de GrxFirma. GitHub utiliza
`.github/workflows/` para ejecutar compilaciones, pruebas y controles de
seguridad de forma reproducible después de cada cambio.

Los ficheros de esta carpeta son configuración YAML y documentación. **No
contienen certificados, claves privadas ni contraseñas.** Las expresiones
`${{ secrets.NOMBRE }}` solo nombran credenciales custodiadas por GitHub fuera
del repositorio y entregadas temporalmente al proceso autorizado.

## Controles incluidos

- `tests.yml`: compilación y pruebas generales.
- `compatibility.yml`: compatibilidad entre sistemas y herramientas.
- `conformance.yml`: conformidad criptográfica y DSS.
- `security-sast.yml`: análisis estático.
- `security-govulncheck.yml`: vulnerabilidades alcanzables en Go.
- `security-dependencies.yml`: dependencias y ficheros de bloqueo.
- `security-secrets.yml`: detección de secretos actuales e históricos.
- `security-sbom.yml`: inventario SBOM y análisis del runtime distribuido.
- `msix.yml`: construcción técnica del paquete MSIX.
- `release.yml`: publicación oficial firmada y protegida.

## Gestión segura

Las credenciales de firma y publicación deben configurarse únicamente en el
entorno protegido `official-release`, con acceso mínimo y aprobación manual.
Nunca deben copiarse a un workflow, un fichero del proyecto, una incidencia,
un log o un commit.

Un cambio en esta carpeta modifica controles de la cadena de suministro. Debe
revisarse como código de seguridad y no debe reducir versiones fijadas,
permisos mínimos, verificaciones de firma ni análisis sin una justificación
documentada.
