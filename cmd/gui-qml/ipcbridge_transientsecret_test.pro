# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

QT += core gui network testlib
CONFIG += console c++17 testcase
CONFIG -= app_bundle

SOURCES += \
    activediagnostics.cpp \
    incidentprivacy.cpp \
    ipcbridge.cpp \
    ipcpeer.cpp \
    ipcbridge_transientsecret_test.cpp \
    transientsecret.cpp \
    translatorbridge.cpp \
    webcompatibilitylease.cpp

HEADERS += \
    activediagnostics.h \
    incidentprivacy.h \
    ipcbridge.h \
    ipcpeer.h \
    transientsecret.h \
    translatorbridge.h \
    webcompatibilitylease.h

# Lote 39a: el catálogo de la interfaz para pintar el motivo de un bloqueo.
RESOURCES += ipcbridge_transientsecret_test.qrc
