; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

; Firma el uninstaller temporal despues de generarlo y antes de que NSIS lo
; comprima dentro del setup. El comando solo se define en la release oficial.
!ifdef GRXFIRMA_UNINSTALL_SIGNER
  !uninstfinalize '"${GRXFIRMA_UNINSTALL_SIGNER}" "%1"'
!endif
