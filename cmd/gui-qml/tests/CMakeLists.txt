# SPDX-License-Identifier: EUPL-1.2
# The product retains its qmake build. This project exposes its existing
# Python/QML suites and optional compiled qmake tests through CTest.
cmake_minimum_required(VERSION 3.20)
project(GrxFirmaQtTests NONE)
include(CTest)
find_package(Python3 REQUIRED COMPONENTS Interpreter)
find_program(QMLTESTRUNNER NAMES qmltestrunner6 qmltestrunner HINTS /usr/lib/qt6/bin REQUIRED)
add_test(NAME qml COMMAND "${QMLTESTRUNNER}" -input "${CMAKE_CURRENT_SOURCE_DIR}/qml")
add_test(NAME contracts COMMAND "${Python3_EXECUTABLE}" -m unittest discover -s "${CMAKE_CURRENT_SOURCE_DIR}" -p "test_*.py")
set_tests_properties(qml contracts PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QT_QUICK_BACKEND=software;QML_XHR_ALLOW_FILE_READ=1" TIMEOUT 180)
set(IPC_TEST_BINARY "" CACHE FILEPATH "Compiled ipcbridge_transientsecret_test binary")
set(TRANSLATOR_TEST_BINARY "" CACHE FILEPATH "Compiled translatorbridge_test binary")
foreach(pair IN ITEMS IPC TRANSLATOR)
    if(${pair}_TEST_BINARY)
        add_test(NAME ${pair} COMMAND "${${pair}_TEST_BINARY}")
        set_tests_properties(${pair} PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen" TIMEOUT 120)
    endif()
endforeach()
