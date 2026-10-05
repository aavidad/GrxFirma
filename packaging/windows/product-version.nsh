; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

!ifndef GRXFIRMA_PRODUCT_VERSION_NSH
!define GRXFIRMA_PRODUCT_VERSION_NSH

; VERSION conserva SemVer completo. El recurso fijo de Windows necesita cuatro
; enteros: prerelease/build metadata no forman parte de esos cuatro componentes.
; NSIS rechaza un núcleo no numérico o fuera de rango; no inventar otra versión.
!macro GrxFirmaProductVersion Description
  !if "${VERSION}" == "dev"
    !define AF2_VERSION_CORE "0.0.0"
  !else
    !searchparse "${VERSION}-" "" AF2_VERSION_WITH_BUILD "-"
    !searchparse "${AF2_VERSION_WITH_BUILD}+" "" AF2_VERSION_CORE "+"
    !undef AF2_VERSION_WITH_BUILD
  !endif
  VIProductVersion "${AF2_VERSION_CORE}.0"
  VIFileVersion "${AF2_VERSION_CORE}.0"
  VIAddVersionKey /LANG=0 "ProductName" "GrxFirma"
  VIAddVersionKey /LANG=0 "CompanyName" "Alberto Avidad Fernández"
  VIAddVersionKey /LANG=0 "FileDescription" "${Description}"
  VIAddVersionKey /LANG=0 "ProductVersion" "${VERSION}"
  VIAddVersionKey /LANG=0 "FileVersion" "${AF2_VERSION_CORE}.0"
  VIAddVersionKey /LANG=0 "LegalCopyright" "Alberto Avidad Fernández. EUPL-1.2."
  !undef AF2_VERSION_CORE
!macroend

!endif
