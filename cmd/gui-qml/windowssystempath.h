// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef WINDOWSSYSTEMPATH_H
#define WINDOWSSYSTEMPATH_H

#ifdef _WIN32

#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>

#include <string>

// D-05: los programas del sistema se lanzan por ruta absoluta, construida con
// el directorio de sistema de Windows. Así no dependen del directorio de
// trabajo ni de PATH, donde alguien podría dejar un explorer.exe o un
// rundll32.exe falsos. Si Windows no da el directorio, la ruta sale vacía y
// quien llama no lanza nada.
namespace WindowsSystemPath {

inline std::wstring join(UINT(WINAPI *directoryGetter)(LPWSTR, UINT),
                         const wchar_t *executable) {
  wchar_t buffer[MAX_PATH];
  const UINT length = directoryGetter(buffer, MAX_PATH);
  if (length == 0 || length >= MAX_PATH)
    return std::wstring();
  std::wstring path(buffer, length);
  if (path.back() != L'\\')
    path.push_back(L'\\');
  path += executable;
  return path;
}

// %SystemRoot%\System32\rundll32.exe
inline std::wstring rundll32() {
  return join(&GetSystemDirectoryW, L"rundll32.exe");
}

// %SystemRoot%\explorer.exe
inline std::wstring explorer() {
  return join(&GetWindowsDirectoryW, L"explorer.exe");
}

} // namespace WindowsSystemPath

#endif // _WIN32

#endif // WINDOWSSYSTEMPATH_H
