<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Firma Windows con SignPath Foundation

SignPath Foundation ofrece firma de código sin coste a proyectos de software libre aceptados en su programa. La firma identifica al editor de los binarios Windows y reduce las advertencias al instalar. La aceptación y la emisión del certificado corresponden a SignPath; no las concede este repositorio.

## Requisitos y preparación

- Proyecto de código abierto, repositorio público y licencia aprobada por la OSI. GrxFirma publica su código bajo EUPL-1.2.
- Compilación verificable y repetible en CI desde el commit publicado. El flujo de GitHub Actions construye la suite Windows con Go, Qt, WinUI y NSIS, y comprueba los artefactos en otro runner. Hay que facilitar a SignPath el workflow y las instrucciones de reproducción.
- Publicar la [política de firma de código](../sitio/firma-de-codigo.html) del sitio estático. SignPath exige esa página pública con su mención, los roles y la nota de privacidad.
- Crear la organización y el proyecto en SignPath Foundation, aceptar las condiciones y configurar la integración con GitHub. El proyecto necesita el slug `grxfirma`, la política `release-signing` y dos configuraciones de artefacto: `windows-payload` y `windows-installers`. Si SignPath asigna otros nombres, hay que cambiarlos en el workflow. Un ZIP no recibe una firma Authenticode como archivo: se firman los ejecutables que contiene y se publica su hash.
- Configurar `SIGNPATH_API_TOKEN` y `SIGNPATH_ORGANIZATION_ID` como secretos del entorno protegido `official-release` y fijar en la variable `WINDOWS_SIGNING_CERT_THUMBPRINT` la huella del certificado de SignPath Foundation.

Los pasos detallados, las configuraciones de artefacto listas para pegar y los secretos están en [CERTIFICADOS.md](CERTIFICADOS.md#a-windows-con-signpath-foundation).

## Cómo firma el workflow

[release.yml](../../.github/workflows/release.yml) usa SignPath en las etiquetas sin sufijo de prueba cuando encuentra los dos secretos. Si no están, usa el PFX propio descrito en [RELEASE_SIGNING.md](../RELEASE_SIGNING.md). Las etiquetas `vX.Y.Z-algo` no usan SignPath. El job `release-policy` elige la vía y la anota en su resumen.

Con SignPath, el job `build-windows` hace dos rondas con la acción oficial `signpath/github-action-submit-signing-request`, fijada por SHA. Cada ronda sube un artefacto, espera a que se apruebe la firma y descarga el resultado con `output-artifact-directory`:

1. Ronda 1, configuración `windows-payload`: los ejecutables y bibliotecas de las etapas Qt y Suite que hay que firmar (los mismos que con PFX) y los dos desinstaladores NSIS. El desinstalador no contiene la carga útil, así que se puede generar y firmar antes que el instalador.
2. Con lo devuelto, el workflow coloca las firmas en las etapas, replica el backend en los payloads, regenera los ZIP y genera los instaladores. NSIS vuelve a crear el desinstalador y el workflow solo lo sustituye por el firmado si es idéntico byte a byte al que se envió.
3. Ronda 2, configuración `windows-installers`: los dos instaladores.
4. Se publican los ZIP y los instaladores firmados en `release/windows-official`, y se regeneran `WINDOWS-SIGNATURES.json` y `SHA256SUMS-windows.txt`.

Cada fichero devuelto se compara con el enviado. Solo se admite que SignPath haya añadido la tabla de certificados al final; cualquier otro cambio, un fichero de más o uno de menos paran la release. Después se verifica con `signtool` y `Get-AuthenticodeSignature` que la firma es válida, del certificado fijado, con SHA-256 y sello de tiempo RFC3161. El job `verify-windows` repite toda la verificación en otro runner sin credenciales antes de publicar.

Cada release necesita dos aprobaciones en SignPath; cada ronda espera hasta una hora.

## Texto listo para pegar en la solicitud

> GrxFirma es una aplicación libre de firma y verificación de documentos para escritorio y Android, acompañada por una extensión de navegador que conecta portales autorizados con el firmador local. Se publica bajo la licencia EUPL-1.2 en https://github.com/aavidad/GrxFirma. Solicitamos acceso a SignPath Foundation para firmar nuestros artefactos Windows y permitir que las personas comprueben su origen e integridad al instalarlos. Los artefactos son el instalador NSIS de la suite Windows (`*-setup.exe`) y los ejecutables incluidos en el paquete ZIP de Windows; el ZIP se identifica además con SHA-256. El código se compila desde un commit y una etiqueta de versión en `.github/workflows/release.yml` mediante GitHub Actions: un runner Windows instala las herramientas fijadas, construye Go, Qt y WinUI, genera el instalador con NSIS y sube los artefactos; otro runner los verifica antes de publicar. El repositorio, la licencia, el flujo de construcción y las instrucciones de reproducción son públicos. Contacto del proyecto: avidad@dipgra.es.

La [política actual de publicación](../RELEASE_SIGNING.md) explica los demás controles y el estado de las releases oficiales.
