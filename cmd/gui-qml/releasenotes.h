// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef GRXFIRMA_RELEASENOTES_H
#define GRXFIRMA_RELEASENOTES_H

#include <QRegularExpression>
#include <QObject>
#include <QString>
#include <QVersionNumber>
#include <algorithm>
#include <utility>

namespace ReleaseNotes {

struct Section {
  QVersionNumber version;
  QString markdown;
};

inline QVersionNumber version(const QString &value) {
  static const QRegularExpression valid(QStringLiteral("^[0-9]+\\.[0-9]+\\.[0-9]+$"));
  return valid.match(value).hasMatch() ? QVersionNumber::fromString(value)
                                         : QVersionNumber();
}

inline QString safeMarkdown(QString text) {
  // El fichero instalado es contenido, nunca código HTML ni una fuente de URLs.
  static const QRegularExpression comments(QStringLiteral("<!--[\\s\\S]*?-->"));
  static const QRegularExpression links(QStringLiteral("!?\\[([^\\x5D\\r\\n]*)\\]\\([^)]*\\)"));
  static const QRegularExpression tags(QStringLiteral("<[^>\\n]*>"));
  text.remove(comments);
  text.replace(links, QStringLiteral("\\1"));
  text.remove(tags);
  return text.trimmed();
}

inline QList<Section> sections(const QString &source) {
  static const QRegularExpression heading(
      QStringLiteral("^## ([0-9]+\\.[0-9]+\\.[0-9]+) — [0-9]{4}-[0-9]{2}-[0-9]{2}\\s*$"));
  QList<Section> result;
  if (source.size() > 64 * 1024)
    return result;
  QString current;
  QVersionNumber currentVersion;
  const auto flush = [&]() {
    if (!currentVersion.isNull() && result.size() < 64) {
      const QString clean = safeMarkdown(current);
      if (!clean.isEmpty())
        result.append({currentVersion, clean});
    }
    current.clear();
    currentVersion = QVersionNumber();
  };
  for (const QString &line : source.split(QLatin1Char('\n'))) {
    if (line.startsWith(QStringLiteral("## "))) {
      flush();
      const auto match = heading.match(line.trimmed());
      if (match.hasMatch()) {
        currentVersion = version(match.captured(1));
        current = line.trimmed() + QLatin1Char('\n');
      }
    } else if (!currentVersion.isNull() && current.size() < 48 * 1024) {
      current += line + QLatin1Char('\n');
    }
  }
  flush();
  std::sort(result.begin(), result.end(), [](const Section &a, const Section &b) {
    return QVersionNumber::compare(a.version, b.version) > 0;
  });
  return result;
}

inline QString select(const QString &source, const QString &installed,
                      const QString &lastSeen = QString()) {
  const auto current = version(installed);
  if (current.isNull())
    return QString();
  const auto previous = version(lastSeen);
  QStringList selected;
  for (const auto &section : sections(source)) {
    if (QVersionNumber::compare(section.version, current) <= 0 &&
        (lastSeen.isEmpty() || (!previous.isNull() &&
                                QVersionNumber::compare(section.version, previous) > 0)))
      selected.append(section.markdown);
  }
  return selected.join(QStringLiteral("\n\n"));
}
} // namespace ReleaseNotes

class ReleaseNotesBridge final : public QObject {
  Q_OBJECT
public:
  explicit ReleaseNotesBridge(QString source, QObject *parent = nullptr)
      : QObject(parent), m_source(std::move(source)) {}
  Q_INVOKABLE QString since(const QString &installed,
                            const QString &lastSeen) const {
    return ReleaseNotes::select(m_source, installed, lastSeen);
  }
private:
  QString m_source;
};

#endif
