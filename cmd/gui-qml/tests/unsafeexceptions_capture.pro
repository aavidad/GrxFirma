# SPDX-License-Identifier: EUPL-1.2
QT += core gui qml quick quickcontrols2
CONFIG += console c++17
CONFIG -= app_bundle
SOURCES += unsafeexceptions_capture.cpp ../translatorbridge.cpp
HEADERS += ../translatorbridge.h
RESOURCES += ../qml.qrc
