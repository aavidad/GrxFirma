// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "../releasenotes.h"
#include <QFile>
#include <cassert>

int main() {
  QFile file(QStringLiteral("docs/NOVEDADES.md"));
  assert(file.open(QIODevice::ReadOnly));
  const QString source = QString::fromUtf8(file.readAll());
  const QString selected = ReleaseNotes::select(source, QStringLiteral("0.0.101"),
                                                QStringLiteral("0.0.99"));
  assert(selected.contains(QStringLiteral("## 0.0.100")));
  assert(selected.contains(QStringLiteral("## 0.0.101")));
  assert(!selected.contains(QStringLiteral("## 0.0.99")));
  assert(!selected.contains(QStringLiteral("<!--")));
  assert(!selected.contains(QStringLiteral("Próxima versión")));
  assert(ReleaseNotes::select(source, QStringLiteral("0.0.101"),
                              QStringLiteral("0.0.101")).isEmpty());
  assert(ReleaseNotes::select(source, QStringLiteral("dev")).isEmpty());
  const QString unsafe = QStringLiteral(
      "<!-- licencia -->\n## 0.0.101 — 2026-10-01\n"
      "- [Ayuda](javascript:alert(1)) y <script>código</script>\n");
  const QString safe = ReleaseNotes::select(unsafe, QStringLiteral("0.0.101"));
  assert(safe.contains(QStringLiteral("Ayuda")));
  assert(!safe.contains(QStringLiteral("javascript:")));
  assert(!safe.contains(QLatin1Char('<')));
  return 0;
}
