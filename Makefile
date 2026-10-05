# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

#
# Makefile para GrxFirma
#
# Targets principales:
#   make build              Compila ambos binarios en el directorio raiz
#   make install            Instala en el sistema (requiere sudo o root)
#   make install-user       Instala solo para el usuario actual
#   make bridge-user        Instala el bridge de navegadores para el usuario actual
#   make desktop            Instala el fichero .desktop y registra afirma://
#   make uninstall          Desinstala del sistema
#   make test               Ejecuta todos los tests
#   make clean              Elimina artefactos de compilacion

BINARY     := grxfirma
BINARY_GUI := grxfirma-desktop
BINARY_QT  := grxfirma-gui-qml
BINARY_URI := grxfirma-afirmauri
BRIDGE_BIN := grxfirma-nativehost
CMD        := ./cmd/grxfirma
CMD_URI    := ./cmd/grxfirmauri
CMD_BRIDGE := ./cmd/nativehost
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
FILE_VERSION := $(shell tr -d '\r\n' < VERSION.txt)
LDFLAGS    := -X main.version=$(VERSION) -s -w
WINARCH    ?= amd64
WINOUTDIR  ?= release/windows-cli
WINSTAGE   ?= $(WINOUTDIR)/GrxFirma-$(FILE_VERSION)-cli-windows-$(WINARCH)
WINZIP     ?= $(WINOUTDIR)/GrxFirma-$(FILE_VERSION)-cli-windows-$(WINARCH).zip
WINNSIS    ?= $(WINOUTDIR)/GrxFirma-$(FILE_VERSION)-cli-windows-$(WINARCH)-setup.exe
WINNHOUTDIR ?= release/windows-nativehost
WINNHSTAGE  ?= $(WINNHOUTDIR)/GrxFirma-$(FILE_VERSION)-nativehost-windows-$(WINARCH)
WINSUITEOUT ?= release/windows-suite
WINQMLOUT  ?= release/windows-desktop-qml
MACCLI_OUT ?= release/macos-cli
MACNH_OUT  ?= release/macos-nativehost
MACAF_OUT  ?= release/macos-afirmauri
MACSUITE_OUT ?= release/macos-suite
QTGUI_PATH := cmd/gui-qml/$(BINARY_QT)
MACQML_OUT ?= release/macos-desktop-qml

PREFIX     ?= /usr/local
BINDIR     ?= $(PREFIX)/bin
APPDIR     ?= $(PREFIX)/share/applications
ICONDIR    ?= $(PREFIX)/share/pixmaps
MANDIR     ?= $(PREFIX)/share/man/man1
MAN7DIR    ?= $(PREFIX)/share/man/man7

USERBIN    ?= $(HOME)/.local/bin
USERAPP    ?= $(HOME)/.local/share/applications
USERLIBDIR ?= $(HOME)/.local/lib/grxfirma/bin
USERCFGDIR ?= $(HOME)/.config/grxfirma/pkcs12
USERMANDIR ?= $(HOME)/.local/share/man/man1
USERMAN7DIR ?= $(HOME)/.local/share/man/man7

# Directorios de manifests Native Messaging por navegador
NM_CHROME   := $(HOME)/.config/google-chrome/NativeMessagingHosts
NM_CHROMIUM := $(HOME)/.config/chromium/NativeMessagingHosts
NM_EDGE     := $(HOME)/.config/microsoft-edge/NativeMessagingHosts
NM_BRAVE    := $(HOME)/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts
NM_VIVALDI  := $(HOME)/.config/vivaldi/NativeMessagingHosts
NM_OPERA    := $(HOME)/.config/opera/NativeMessagingHosts
NM_FIREFOX  := $(HOME)/.mozilla/native-messaging-hosts

# ID de la extension Chrome/Chromium y Firefox
CHROME_EXT_ID  ?= pkefjandjcgdmhoonmhnllikibobijgg
FIREFOX_EXT_ID ?= grxfirma@aavidad.github.io

.PHONY: all build build-gui build-uri build-bridge build-qt-bootstrap build-pkcs11-worker build-qt-qml windows-cli windows-cli-zip windows-cli-nsis \
        windows-nativehost windows-nativehost-zip windows-suite windows-desktop-qml windows-desktop-qml-nsis \
        macos-cli macos-nativehost macos-afirmauri macos-suite macos-desktop-qml \
        pades-cierre-local pades-validacion-externa \
        install install-user bridge-user desktop desktop-user desktop-user-debug uninstall \
        uninstall-user test test-conformance-dss test-conformance-dss-offline clean vet install-git-hooks

all: build build-gui build-uri build-bridge

## --------------------------------------------------------------------------
## Puertas locales
## --------------------------------------------------------------------------

# tests.yml corre con "on: push", o sea despues de publicar, y el proyecto
# integra con commits directos sobre main sin pull request. Este hook es el
# unico punto del flujo que puede bloquear una publicacion en rojo.
install-git-hooks:
	install -m 755 scripts/git-hooks/pre-push "$$(git rev-parse --git-path hooks)/pre-push"
	@echo "hook pre-push instalado; salto puntual con 'git push --no-verify'"

## --------------------------------------------------------------------------
## Compilacion
## --------------------------------------------------------------------------

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

build-gui:
	go build -tags fyne_gui -ldflags "$(LDFLAGS)" -o $(BINARY_GUI) $(CMD)

build-uri:
	go build -tags fyne_gui -ldflags "$(LDFLAGS)" -o $(BINARY_URI) $(CMD_URI)

build-bridge:
	go build -ldflags "$(LDFLAGS)" -o $(BRIDGE_BIN) $(CMD_BRIDGE)

build-qt-bootstrap:
	go build -tags production -ldflags "$(LDFLAGS)" -o grxfirma-gui ./cmd/grxfirma-gui

build-pkcs11-worker:
	CGO_ENABLED=1 go build -tags production -ldflags "$(LDFLAGS)" -o grxfirma-pkcs11-worker ./cmd/grxfirma-pkcs11-worker

build-qt-qml:
	cd cmd/gui-qml && qmake grxfirma_qt.pro
	$(MAKE) -C cmd/gui-qml
	mv cmd/gui-qml/grxfirma-gui-qml cmd/gui-qml/grxfirma-gui-qml

windows-cli:
	./packaging/windows/build-cli.sh

windows-cli-zip: windows-cli

windows-cli-nsis:
	./packaging/windows/build-cli.sh --nsis

windows-nativehost:
	./packaging/windows/build-nativehost.sh

windows-nativehost-zip: windows-nativehost

windows-suite:
	@echo "La suite Windows debe construirse desde Windows con PowerShell:"
	@echo "  .\\packaging\\windows\\build-suite.ps1"

windows-desktop-qml:
	@echo "El frontend desktop Qt/QML para Windows debe construirse desde Windows:"
	@echo "  .\\packaging\\windows\\build-desktop-qml.ps1"

windows-desktop-qml-nsis:
	@echo "El instalador NSIS del frontend desktop Qt/QML debe construirse desde Windows:"
	@echo "  .\\packaging\\windows\\build-desktop-qml.ps1 --nsis"

macos-cli:
	@echo "La CLI de macOS debe construirse desde macOS:"
	@echo "  ./packaging/macos/build-cli.sh"

macos-nativehost:
	@echo "El nativehost de macOS debe construirse desde macOS:"
	@echo "  ./packaging/macos/build-nativehost.sh"

macos-afirmauri:
	@echo "El handler afirma:// de macOS debe construirse desde macOS:"
	@echo "  ./packaging/macos/build-afirmauri.sh"

macos-suite:
	@echo "La suite de macOS debe construirse desde macOS:"
	@echo "  ./packaging/macos/build-suite.sh"

macos-desktop-qml:
	@echo "La app desktop Qt/QML de macOS debe construirse desde macOS:"
	@echo "  ./packaging/macos/build-desktop-qml.sh"

pades-cierre-local:
	bash scripts/cierre_pades_local.sh /tmp/grxfirma-pades-cierre

pades-validacion-externa:
	bash scripts/preparar_validacion_pades_externa.sh /tmp/grxfirma-pades-validacion-externa

## --------------------------------------------------------------------------
## Instalacion en el sistema (requiere permisos de root / sudo)
## --------------------------------------------------------------------------

install: build build-gui build-uri build-bridge
	install -D -m 755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -D -m 755 $(BINARY_GUI) $(DESTDIR)$(BINDIR)/$(BINARY_GUI)
	@if [ -f "$(QTGUI_PATH)" ]; then install -D -m 755 $(QTGUI_PATH) $(DESTDIR)$(BINDIR)/$(BINARY_QT); fi
	install -D -m 755 $(BINARY_URI) $(DESTDIR)$(BINDIR)/$(BINARY_URI)
	install -D -m 755 $(BRIDGE_BIN) $(DESTDIR)$(BINDIR)/$(BRIDGE_BIN)
	install -D -m 644 packaging/linux/man/grxfirma.1 $(DESTDIR)$(MANDIR)/grxfirma.1
	install -D -m 644 packaging/linux/man/grxfirma-firmas.7 $(DESTDIR)$(MAN7DIR)/grxfirma-firmas.7
	install -D -m 644 packaging/linux/man/grxfirma-confianza.7 $(DESTDIR)$(MAN7DIR)/grxfirma-confianza.7
	install -D -m 644 packaging/linux/man/grxfirma-tls-local.7 $(DESTDIR)$(MAN7DIR)/grxfirma-tls-local.7
	install -D -m 644 packaging/linux/man/grxfirma-puente-navegador.7 $(DESTDIR)$(MAN7DIR)/grxfirma-puente-navegador.7
	install -D -m 644 packaging/linux/man/grxfirma-verificacion.7 $(DESTDIR)$(MAN7DIR)/grxfirma-verificacion.7
	install -D -m 644 packaging/linux/man/grxfirma-formatos.7 $(DESTDIR)$(MAN7DIR)/grxfirma-formatos.7
	install -D -m 644 packaging/linux/man/grxfirma-portales.7 $(DESTDIR)$(MAN7DIR)/grxfirma-portales.7
	install -D -m 644 packaging/linux/man/grxfirma-recetas.7 $(DESTDIR)$(MAN7DIR)/grxfirma-recetas.7
	install -D -m 644 packaging/linux/man/grxfirma-diagnostico.7 $(DESTDIR)$(MAN7DIR)/grxfirma-diagnostico.7
	install -D -m 644 packaging/linux/man/grxfirma-certificados.7 $(DESTDIR)$(MAN7DIR)/grxfirma-certificados.7
	install -D -m 644 packaging/linux/man/grxfirma-servicios-locales.7 $(DESTDIR)$(MAN7DIR)/grxfirma-servicios-locales.7
	install -D -m 644 packaging/linux/man/grxfirma-integracion-web.7 $(DESTDIR)$(MAN7DIR)/grxfirma-integracion-web.7
	install -D -m 644 packaging/linux/man/grxfirma-lotes.7 $(DESTDIR)$(MAN7DIR)/grxfirma-lotes.7
	install -D -m 644 packaging/linux/man/grxfirma-protocolos-web.7 $(DESTDIR)$(MAN7DIR)/grxfirma-protocolos-web.7
	install -D -m 644 packaging/linux/man/grxfirma-errores-comunes.7 $(DESTDIR)$(MAN7DIR)/grxfirma-errores-comunes.7
	install -D -m 644 packaging/linux/man/grxfirma-operacion.7 $(DESTDIR)$(MAN7DIR)/grxfirma-operacion.7
	install -D -m 644 packaging/linux/man/grxfirma-autenticacion-y-certificados.7 $(DESTDIR)$(MAN7DIR)/grxfirma-autenticacion-y-certificados.7
	install -D -m 644 packaging/linux/man/grxfirma-mejoras.7 $(DESTDIR)$(MAN7DIR)/grxfirma-mejoras.7
	install -D -m 644 packaging/linux/man/grxfirma-frontend-qt.7 $(DESTDIR)$(MAN7DIR)/grxfirma-frontend-qt.7
	@echo "Instalado en $(DESTDIR)$(BINDIR)/$(BINARY), $(DESTDIR)$(BINDIR)/$(BINARY_GUI) y $(DESTDIR)$(BINDIR)/$(BINARY_URI)"
	@if [ -f "$(QTGUI_PATH)" ]; then echo "Frontend Qt instalado en $(DESTDIR)$(BINDIR)/$(BINARY_QT)"; else echo "Frontend Qt no instalado (compila antes con make build-qt-qml si lo necesitas)."; fi

desktop: install
	install -D -m 644 packaging/linux/grxfirma.desktop $(DESTDIR)$(APPDIR)/$(BINARY).desktop
	install -D -m 644 packaging/linux/grxfirma-manual.desktop $(DESTDIR)$(APPDIR)/$(BINARY)-manual.desktop
	rm -f $(DESTDIR)$(APPDIR)/$(BINARY)-manual-debug.desktop
	rm -f $(DESTDIR)$(APPDIR)/$(BINARY)-debug.desktop
	@if command -v update-desktop-database >/dev/null 2>&1; then \
	    update-desktop-database $(DESTDIR)$(APPDIR); \
	fi
	@if command -v xdg-mime >/dev/null 2>&1; then \
	    xdg-mime default $(BINARY).desktop x-scheme-handler/afirma; \
	fi
	@if command -v update-mime-database >/dev/null 2>&1; then \
	    update-mime-database $(DESTDIR)$(PREFIX)/share/mime || true; \
	fi
	@echo "Esquema afirma:// registrado"

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY)
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_GUI)
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_QT)
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_URI)
	rm -f $(DESTDIR)$(MANDIR)/grxfirma.1
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-firmas.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-confianza.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-tls-local.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-puente-navegador.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-verificacion.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-formatos.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-portales.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-recetas.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-diagnostico.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-certificados.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-servicios-locales.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-integracion-web.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-lotes.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-protocolos-web.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-errores-comunes.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-operacion.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-autenticacion-y-certificados.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-mejoras.7
	rm -f $(DESTDIR)$(MAN7DIR)/grxfirma-frontend-qt.7
	rm -f $(DESTDIR)$(APPDIR)/$(BINARY).desktop
	rm -f $(DESTDIR)$(APPDIR)/$(BINARY)-manual.desktop
	rm -f $(DESTDIR)$(APPDIR)/$(BINARY)-manual-debug.desktop
	@if command -v update-desktop-database >/dev/null 2>&1; then \
	    update-desktop-database $(DESTDIR)$(APPDIR) 2>/dev/null || true; \
	fi
	@echo "Desinstalado"

## --------------------------------------------------------------------------
## Instalacion para el usuario actual (sin privilegios)
## --------------------------------------------------------------------------

install-user: build build-gui build-uri build-bridge
	install -D -m 755 $(BINARY) $(USERBIN)/$(BINARY)
	install -D -m 755 $(BINARY_GUI) $(USERBIN)/$(BINARY_GUI)
	@if [ -f "$(QTGUI_PATH)" ]; then install -D -m 755 $(QTGUI_PATH) $(USERBIN)/$(BINARY_QT); fi
	install -D -m 755 $(BINARY_URI) $(USERBIN)/$(BINARY_URI)
	install -D -m 755 $(BRIDGE_BIN) $(USERLIBDIR)/$(BRIDGE_BIN)
	install -D -m 644 packaging/linux/man/grxfirma.1 $(USERMANDIR)/grxfirma.1
	install -D -m 644 packaging/linux/man/grxfirma-firmas.7 $(USERMAN7DIR)/grxfirma-firmas.7
	install -D -m 644 packaging/linux/man/grxfirma-confianza.7 $(USERMAN7DIR)/grxfirma-confianza.7
	install -D -m 644 packaging/linux/man/grxfirma-tls-local.7 $(USERMAN7DIR)/grxfirma-tls-local.7
	install -D -m 644 packaging/linux/man/grxfirma-puente-navegador.7 $(USERMAN7DIR)/grxfirma-puente-navegador.7
	install -D -m 644 packaging/linux/man/grxfirma-verificacion.7 $(USERMAN7DIR)/grxfirma-verificacion.7
	install -D -m 644 packaging/linux/man/grxfirma-formatos.7 $(USERMAN7DIR)/grxfirma-formatos.7
	install -D -m 644 packaging/linux/man/grxfirma-portales.7 $(USERMAN7DIR)/grxfirma-portales.7
	install -D -m 644 packaging/linux/man/grxfirma-recetas.7 $(USERMAN7DIR)/grxfirma-recetas.7
	install -D -m 644 packaging/linux/man/grxfirma-diagnostico.7 $(USERMAN7DIR)/grxfirma-diagnostico.7
	install -D -m 644 packaging/linux/man/grxfirma-certificados.7 $(USERMAN7DIR)/grxfirma-certificados.7
	install -D -m 644 packaging/linux/man/grxfirma-servicios-locales.7 $(USERMAN7DIR)/grxfirma-servicios-locales.7
	install -D -m 644 packaging/linux/man/grxfirma-integracion-web.7 $(USERMAN7DIR)/grxfirma-integracion-web.7
	install -D -m 644 packaging/linux/man/grxfirma-lotes.7 $(USERMAN7DIR)/grxfirma-lotes.7
	install -D -m 644 packaging/linux/man/grxfirma-protocolos-web.7 $(USERMAN7DIR)/grxfirma-protocolos-web.7
	install -D -m 644 packaging/linux/man/grxfirma-errores-comunes.7 $(USERMAN7DIR)/grxfirma-errores-comunes.7
	install -D -m 644 packaging/linux/man/grxfirma-operacion.7 $(USERMAN7DIR)/grxfirma-operacion.7
	install -D -m 644 packaging/linux/man/grxfirma-autenticacion-y-certificados.7 $(USERMAN7DIR)/grxfirma-autenticacion-y-certificados.7
	install -D -m 644 packaging/linux/man/grxfirma-mejoras.7 $(USERMAN7DIR)/grxfirma-mejoras.7
	install -D -m 644 packaging/linux/man/grxfirma-frontend-qt.7 $(USERMAN7DIR)/grxfirma-frontend-qt.7
	@mkdir -p "$(HOME)/.local/lib/grxfirma/config"
	@chmod 700 "$(HOME)/.local/lib/grxfirma/config"
	@if [ ! -f "$(HOME)/.local/lib/grxfirma/config/env.sh" ]; then \
	    printf '#!/usr/bin/env bash\n# Ajusta estas variables a tu entorno local antes de usar GrxFirma.\nexport GRXFIRMA_CERTS_DIR="$${GRXFIRMA_CERTS_DIR:-$$HOME/.config/grxfirma/certs}"\nexport GRXFIRMA_PKCS12_DIR="$${GRXFIRMA_PKCS12_DIR:-$$HOME/.config/grxfirma/pkcs12}"\nexport GRXFIRMA_PKCS12_PASSWORD="$${GRXFIRMA_PKCS12_PASSWORD:-}"\n' \
	        > "$(HOME)/.local/lib/grxfirma/config/env.sh"; \
	    chmod 600 "$(HOME)/.local/lib/grxfirma/config/env.sh"; \
	fi
	@chmod 600 "$(HOME)/.local/lib/grxfirma/config/env.sh"
	@sed -i '/^export GRXFIRMA_DEBUG_ENABLED=/d; /^export GRXFIRMA_DEBUG_LOG_FILE=/d' \
	    "$(HOME)/.local/lib/grxfirma/config/env.sh"
	@# Actualizar el bridge script para apuntar al nuevo binario
	@printf '#!/usr/bin/env bash\nset -euo pipefail\nsource "%s/config/env.sh" 2>/dev/null || true\nexec "%s/%s"\n' \
	    "$(HOME)/.local/lib/grxfirma" "$(USERLIBDIR)" "$(BRIDGE_BIN)" \
	    > $(USERLIBDIR)/browser-bridge.sh
	@chmod 755 $(USERLIBDIR)/browser-bridge.sh
	@printf '#!/usr/bin/env bash\nset -euo pipefail\nsource "%s/config/env.sh" 2>/dev/null || true\nuri="$${1:-}"\nexport GRXFIRMA_PROTOCOL_UI=1\nexec "%s/%s" "$$uri"\n' \
	    "$(HOME)/.local/lib/grxfirma" "$(USERBIN)" "$(BINARY_URI)" \
	    > $(USERLIBDIR)/afirmauri-handler.sh
	@chmod 755 $(USERLIBDIR)/afirmauri-handler.sh
	@mkdir -p $(USERCFGDIR)
	@echo "Instalado en $(USERBIN)/$(BINARY), $(USERBIN)/$(BINARY_GUI), $(USERBIN)/$(BINARY_URI) y $(USERLIBDIR)/$(BRIDGE_BIN)"
	@if [ -f "$(QTGUI_PATH)" ]; then echo "Frontend Qt instalado en $(USERBIN)/$(BINARY_QT)"; else echo "Frontend Qt no instalado (compila antes con make build-qt-qml si lo necesitas)."; fi

## --------------------------------------------------------------------------
## Bridge de navegadores (instala manifests Native Messaging)
## --------------------------------------------------------------------------

bridge-user: install-user
	@# Familia Chromium - formato allowed_origins
	@for dir in "$(NM_CHROME)" "$(NM_CHROMIUM)" "$(NM_EDGE)" "$(NM_BRAVE)" "$(NM_VIVALDI)" "$(NM_OPERA)"; do \
	    if [ -d "$$dir" ]; then \
	        printf '{\n  "name": "com.grxfirma.native",\n  "description": "GrxFirma Native Messaging Host",\n  "path": "%s/browser-bridge.sh",\n  "type": "stdio",\n  "allowed_origins": ["chrome-extension://$(CHROME_EXT_ID)/"]\n}\n' \
	            "$(USERLIBDIR)" > "$$dir/com.grxfirma.native.json"; \
	        printf '{\n  "name": "io.github.aavidad.grxfirma",\n  "description": "GrxFirma Native Messaging Host",\n  "path": "%s/browser-bridge.sh",\n  "type": "stdio",\n  "allowed_origins": ["chrome-extension://$(CHROME_EXT_ID)/"]\n}\n' \
	            "$(USERLIBDIR)" > "$$dir/io.github.aavidad.grxfirma.json"; \
	        printf '{\n  "name": "io.github.aavidad.portafirmas",\n  "description": "GrxFirma Native Messaging Host",\n  "path": "%s/browser-bridge.sh",\n  "type": "stdio",\n  "allowed_origins": ["chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/"]\n}\n' \
	            "$(USERLIBDIR)" > "$$dir/io.github.aavidad.portafirmas.json"; \
	        rm -f "$$dir/com.dipgra.grxfirma.json" "$$dir/com.dipgra.portafirmas.json"; \
	        echo "  Manifest instalado en $$dir"; \
	    fi \
	done
	@# Firefox — formato allowed_extensions
	@mkdir -p "$(NM_FIREFOX)"
	@printf '{\n  "name": "com.grxfirma.native",\n  "description": "GrxFirma Native Messaging Host",\n  "path": "%s/browser-bridge.sh",\n  "type": "stdio",\n  "allowed_extensions": ["$(FIREFOX_EXT_ID)"]\n}\n' \
	    "$(USERLIBDIR)" > "$(NM_FIREFOX)/com.grxfirma.native.json"
	@printf '{\n  "name": "io.github.aavidad.grxfirma",\n  "description": "GrxFirma Native Messaging Host",\n  "path": "%s/browser-bridge.sh",\n  "type": "stdio",\n  "allowed_extensions": ["$(FIREFOX_EXT_ID)"]\n}\n' \
	    "$(USERLIBDIR)" > "$(NM_FIREFOX)/io.github.aavidad.grxfirma.json"
	@printf '{\n  "name": "io.github.aavidad.portafirmas",\n  "description": "GrxFirma Native Messaging Host",\n  "path": "%s/browser-bridge.sh",\n  "type": "stdio",\n  "allowed_extensions": ["portafirmas@dipgra.es"]\n}\n' \
	    "$(USERLIBDIR)" > "$(NM_FIREFOX)/io.github.aavidad.portafirmas.json"
	@rm -f "$(NM_FIREFOX)/com.dipgra.grxfirma.json" "$(NM_FIREFOX)/com.dipgra.portafirmas.json"
	@echo "  Manifest instalado en $(NM_FIREFOX)"
	@echo "Bridge de navegadores instalado."
	@echo "  Certificados: coloca tus .p12 en $(USERCFGDIR)"

desktop-user: install-user
	@mkdir -p "$(USERAPP)"
	@sed 's|^Exec=.*|Exec=$(USERLIBDIR)/afirmauri-handler.sh %u|' packaging/linux/grxfirma.desktop > $(USERAPP)/$(BINARY).desktop
	@chmod 644 $(USERAPP)/$(BINARY).desktop
	install -D -m 644 packaging/linux/grxfirma-manual.desktop $(USERAPP)/$(BINARY)-manual.desktop
	rm -f $(USERAPP)/$(BINARY)-manual-debug.desktop
	rm -f $(USERAPP)/$(BINARY)-debug.desktop
	@if command -v update-desktop-database >/dev/null 2>&1; then \
	    update-desktop-database $(USERAPP); \
	fi
	@if command -v xdg-mime >/dev/null 2>&1; then \
	    xdg-mime default $(BINARY).desktop x-scheme-handler/afirma; \
	fi
	@echo "Esquema afirma:// registrado para el usuario"

desktop-user-debug: install-user
	install -D -m 644 packaging/linux/grxfirma-debug.desktop $(USERAPP)/$(BINARY)-debug.desktop
	@if command -v update-desktop-database >/dev/null 2>&1; then \
	    update-desktop-database $(USERAPP); \
	fi
	@if command -v xdg-mime >/dev/null 2>&1; then \
	    xdg-mime default $(BINARY)-debug.desktop x-scheme-handler/afirma; \
	fi
	@echo "Esquema afirma:// registrado en modo debug para el usuario"

uninstall-user:
	rm -f $(USERBIN)/$(BINARY)
	rm -f $(USERBIN)/$(BINARY_GUI)
	rm -f $(USERBIN)/$(BINARY_QT)
	rm -f $(USERBIN)/$(BINARY_URI)
	rm -f $(USERMANDIR)/grxfirma.1
	rm -f $(USERMAN7DIR)/grxfirma-firmas.7
	rm -f $(USERMAN7DIR)/grxfirma-confianza.7
	rm -f $(USERMAN7DIR)/grxfirma-tls-local.7
	rm -f $(USERMAN7DIR)/grxfirma-puente-navegador.7
	rm -f $(USERMAN7DIR)/grxfirma-verificacion.7
	rm -f $(USERMAN7DIR)/grxfirma-formatos.7
	rm -f $(USERMAN7DIR)/grxfirma-portales.7
	rm -f $(USERMAN7DIR)/grxfirma-recetas.7
	rm -f $(USERMAN7DIR)/grxfirma-diagnostico.7
	rm -f $(USERMAN7DIR)/grxfirma-certificados.7
	rm -f $(USERMAN7DIR)/grxfirma-servicios-locales.7
	rm -f $(USERMAN7DIR)/grxfirma-integracion-web.7
	rm -f $(USERMAN7DIR)/grxfirma-lotes.7
	rm -f $(USERMAN7DIR)/grxfirma-protocolos-web.7
	rm -f $(USERMAN7DIR)/grxfirma-errores-comunes.7
	rm -f $(USERMAN7DIR)/grxfirma-operacion.7
	rm -f $(USERMAN7DIR)/grxfirma-autenticacion-y-certificados.7
	rm -f $(USERMAN7DIR)/grxfirma-mejoras.7
	rm -f $(USERMAN7DIR)/grxfirma-frontend-qt.7
	rm -f $(USERMAN7DIR)/grxfirma-recetas.7
	rm -f $(USERMAN7DIR)/grxfirma-diagnostico.7
	rm -f $(USERAPP)/$(BINARY).desktop
	rm -f $(USERAPP)/$(BINARY)-debug.desktop
	rm -f $(USERAPP)/$(BINARY)-manual.desktop
	rm -f $(USERAPP)/$(BINARY)-manual-debug.desktop
	@if command -v update-desktop-database >/dev/null 2>&1; then \
	    update-desktop-database $(USERAPP) 2>/dev/null || true; \
	fi
	@echo "Desinstalado del usuario"

## --------------------------------------------------------------------------
## Calidad
## --------------------------------------------------------------------------

test:
	go test ./...

test-conformance-dss:
	@test -n "$${GRXFIRMA_DSS_RUNNER:-}" || { \
	    echo "ERROR: GRXFIRMA_DSS_RUNNER no configurado; use test-conformance-dss-offline para validar solo el harness" >&2; \
	    exit 1; \
	}
	@test "$${GRXFIRMA_DSS_EXPECTED_RESULT:-}" = "TOTAL_PASSED" || { \
	    echo "ERROR: GRXFIRMA_DSS_EXPECTED_RESULT debe ser exactamente TOTAL_PASSED" >&2; \
	    exit 1; \
	}
	go test ./test/conformance/...

test-conformance-dss-offline:
	@echo "AVISO: gate offline; no constituye evidencia de conformidad DSS/ETSI"
	env -u GRXFIRMA_DSS_RUNNER -u GRXFIRMA_DSS_EXPECTED_RESULT go test ./test/conformance/...

vet:
	go vet ./...

## --------------------------------------------------------------------------
## Limpieza
## --------------------------------------------------------------------------

clean:
	rm -f $(BINARY) $(BINARY_GUI) $(BINARY_URI) $(BRIDGE_BIN)
	rm -f cmd/gui-qml/grxfirma-gui-qml cmd/gui-qml/grxfirma-gui-qml.exe
	rm -rf $(WINOUTDIR)
	rm -rf $(WINNHOUTDIR)
	rm -rf $(WINSUITEOUT)
	rm -rf $(WINQMLOUT)
	rm -rf $(MACCLI_OUT)
	rm -rf $(MACNH_OUT)
	rm -rf $(MACAF_OUT)
	rm -rf $(MACSUITE_OUT)
	rm -rf $(MACQML_OUT)
