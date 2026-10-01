# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

QT += core gui widgets testlib
CONFIG += c++17 testcase console
CONFIG -= app_bundle

TARGET = residentagent_test

SOURCES += \
        residentagent.cpp \
        residentagent_test.cpp

HEADERS += residentagent.h
