# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

TARGET = grxfirma-gui-qml
QT += core gui qml quick network widgets
greaterThan(QT_MAJOR_VERSION, 5): QT += quickcontrols2

CONFIG += c++17

# Qt 6.7 conserva referencias al framework AGL que Xcode 26 ya no distribuye.
# OpenGL.framework es la unica biblioteca de esa lista usada por Qt Quick.
macx {
    QMAKE_LIBS_OPENGL = -framework OpenGL

    # Qt 6.7 consulta __has_builtin(__yield) antes de incluir la declaracion
    # ACLE. Xcode 26 lo reconoce como builtin en arm64, pero exige que
    # arm_acle.h declare el simbolo. La preinclusion queda limitada al kit
    # macOS ARM y no silencia ningun diagnostico del compilador.
    contains(QMAKE_APPLE_DEVICE_ARCHS, arm64) {
        QMAKE_CXXFLAGS += -include arm_acle.h
    }
}

# Evitar dependencia de widgets si no se usa
# QT -= widgets

SOURCES += \
        main.cpp \
        activediagnostics.cpp \
        backendbridge.cpp \
        incidentprivacy.cpp \
        ipcbridge.cpp \
        portalsealbridge.cpp \
        residentagent.cpp \
        transientsecret.cpp \
        translatorbridge.cpp \
        webcompatibilitylease.cpp

HEADERS += \
        activediagnostics.h \
        backendbridge.h \
        executablelocator.h \
        incidentprivacy.h \
        ipcbridge.h \
        portalsealbridge.h \
        ipcsocketpath.h \
        processarguments.h \
        processenvironment.h \
        residentagent.h \
        releasenotes.h \
        transientsecret.h \
        translatorbridge.h \
        webcompatibilitylease.h

RESOURCES += qml.qrc

# Configuración de despliegue básica
target.path = /usr/bin
INSTALLS += target
