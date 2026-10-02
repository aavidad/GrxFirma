<!-- Derechos de autor (C) 2026 Diputación de Granada. -->
<!-- Autoría: Oficina de Software Libre de la Diputación de Granada. -->
<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Firma Windows con SignPath Foundation

SignPath Foundation ofrece firma de código sin coste a proyectos de software libre aceptados en su programa. La firma identifica al editor de los binarios Windows y reduce las advertencias al instalar. La aceptación y la emisión del certificado corresponden a SignPath; no las concede este repositorio.

## Requisitos y preparación

- Proyecto de código abierto, repositorio público y licencia aprobada por la OSI. GrxFirma publica su código bajo EUPL-1.2.
- Compilación verificable y repetible en CI desde el commit publicado. El flujo de GitHub Actions construye la suite Windows con Go, Qt, WinUI y NSIS, y comprueba los artefactos en otro runner. Hay que facilitar a SignPath el workflow y las instrucciones de reproducción.
- Crear la organización y el proyecto en SignPath Foundation, aceptar las condiciones y configurar la integración con GitHub. Solicitar que el perfil `windows-installers` firme el instalador NSIS y los ejecutables incluidos en el ZIP. Un ZIP no recibe una firma Authenticode como archivo: se firman los ejecutables que contiene y se publica su hash.
- Configurar `SIGNPATH_API_TOKEN` y `SIGNPATH_ORGANIZATION_ID` como secretos del entorno protegido `official-release`. El proyecto debe tener slug `grxfirma`, política `release-signing` y configuración de artefacto `windows-installers`, o habrá que adaptar esos tres valores en el workflow a los otorgados por SignPath.

El paso opcional de [release.yml](../../.github/workflows/release.yml) envía a la acción oficial `signpath/github-action-submit-signing-request` el identificador del artefacto Windows generado por GitHub Actions. Espera el resultado y falla si la solicitud falla. Sin ambos secretos, se omite. Los tags con sufijo de prueba (`-...`) no solicitan SignPath. La release oficial sigue exigiendo el certificado Authenticode actual y su verificación independiente: **esta integración no reemplaza todavía ese control ni publica automáticamente el resultado de SignPath**. Antes de cambiar el paquete público habrá que integrar el artefacto devuelto, regenerar hashes y evidencias y verificar la nueva huella en un runner independiente.

## Texto listo para pegar en la solicitud

> GrxFirma es una aplicación libre de firma y verificación de documentos para escritorio y Android, acompañada por una extensión de navegador que conecta portales autorizados con el firmador local. Se publica bajo la licencia EUPL-1.2 en https://github.com/aavidad/GrxFirma. Solicitamos acceso a SignPath Foundation para firmar nuestros artefactos Windows y permitir que las personas comprueben su origen e integridad al instalarlos. Los artefactos son el instalador NSIS de la suite Windows (`*-setup.exe`) y los ejecutables incluidos en el paquete ZIP de Windows; el ZIP se identifica además con SHA-256. El código se compila desde un commit y una etiqueta de versión en `.github/workflows/release.yml` mediante GitHub Actions: un runner Windows instala las herramientas fijadas, construye Go, Qt y WinUI, genera el instalador con NSIS y sube los artefactos; otro runner los verifica antes de publicar. El repositorio, la licencia, el flujo de construcción y las instrucciones de reproducción son públicos. Contacto del proyecto: avidad@dipgra.es.

La [política actual de publicación](../RELEASE_SIGNING.md) explica los demás controles y el estado de las releases oficiales.
