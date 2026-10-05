// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#pragma once

#include <QRegularExpression>
#include <QString>
#include <QUrl>

namespace VerificationUrl {
// RFC 5893: QUrl convierte ACE sin aplicar todas las reglas Bidi.
// Comprobar las etiquetas Unicode devueltas por Qt, también si llegaron en ACE.
inline bool validBidi(const QString &domain) {
  bool bidiDomain = false;
  for (const auto c : domain.toUcs4()) {
    if (QChar::direction(c) == QChar::DirR || QChar::direction(c) == QChar::DirAL ||
        QChar::direction(c) == QChar::DirAN)
      bidiDomain = true;
  }
  if (!bidiDomain)
    return true;
  for (const QString &label : domain.split(u'.')) {
    if (label.isEmpty())
      return false;
    const auto codepoints = label.toUcs4();
    const auto first = QChar::direction(codepoints.front());
    const bool rtl = first == QChar::DirR || first == QChar::DirAL;
    if (!rtl && first != QChar::DirL)
      return false;
    auto last = first;
    bool europeanNumber = false, arabicNumber = false;
    for (const auto c : codepoints) {
      const auto d = QChar::direction(c);
      const bool neutral = d == QChar::DirES || d == QChar::DirCS ||
          d == QChar::DirET || d == QChar::DirON || d == QChar::DirBN || d == QChar::DirNSM;
      if (!neutral && d != QChar::DirEN &&
          !(rtl ? (d == QChar::DirR || d == QChar::DirAL || d == QChar::DirAN)
                : d == QChar::DirL))
        return false;
      europeanNumber |= d == QChar::DirEN;
      arabicNumber |= d == QChar::DirAN;
      if (d != QChar::DirNSM)
        last = d;
    }
    if (rtl) {
      if ((europeanNumber && arabicNumber) ||
          (last != QChar::DirR && last != QChar::DirAL && last != QChar::DirEN && last != QChar::DirAN))
        return false;
    } else if (last != QChar::DirL && last != QChar::DirEN) {
      return false;
    }
  }
  return true;
}

// Caracteres de control (Cc) o de formato (Cf): marcas Bidi, anchura cero,
// U+FEFF... No se ven, pero alteran lo que se muestra frente a lo que se abre
// o se estampa. Se recorre por puntos de código para cubrir los de fuera del
// plano básico; un sustituto suelto también se rechaza.
inline bool hasControlOrFormat(const QString &value) {
  if (!value.isValidUtf16())
    return true;
  for (const char32_t c : value.toUcs4()) {
    const auto category = QChar::category(c);
    if (category == QChar::Other_Control || category == QChar::Other_Format)
      return true;
  }
  return false;
}

inline QString normalize(const QString &raw) {
  QString value = raw.trimmed();
  if (value.isEmpty() || value.size() > 2048)
    return {};
  if (hasControlOrFormat(value))
    return {};
  for (const QChar c : value) {
    if (c.isSpace() || c == u'\\')
      return {};
  }
  if (!value.contains(QStringLiteral("://"))) {
    if (value.contains(u':'))
      return {};
    value.prepend(QStringLiteral("https://"));
  }
  if (!value.startsWith(QStringLiteral("https://"), Qt::CaseInsensitive))
    return {};
  const auto tail = value.indexOf(QRegularExpression(QStringLiteral("[/?#]")), 8);
  const auto end = tail < 0 ? value.size() : tail;
  const QString authority = value.mid(8, end - 8);
  if (authority.isEmpty() || authority.contains(u'@') || authority.contains(u'%'))
    return {};
  QString host = authority;
  QString port;
  if (authority.startsWith(u'[')) {
    const auto close = authority.indexOf(u']');
    if (close < 0)
      return {};
    host = authority.left(close + 1);
    port = authority.mid(close + 1);
  } else {
    const auto colon = authority.lastIndexOf(u':');
    if (colon >= 0) {
      host = authority.left(colon);
      port = authority.mid(colon);
    }
  }
  if (!port.isEmpty()) {
    static const QRegularExpression validPort(QStringLiteral("^:[0-9]{1,5}$"));
    bool ok = false;
    const int number = port.mid(1).toInt(&ok);
    if (!validPort.match(port).hasMatch() || !ok || number < 1 || number > 65535)
      return {};
  }
  // Convertir el host original: QUrl puede normalizar caracteres antes de
  // exponer host(). No depender de la lista de TLD permitidos para mostrar IDN.
  if (!host.startsWith(u'[')) {
    const QByteArray ace = QUrl::toAce(host, QUrl::IgnoreIDNWhitelist);
    if (ace.isEmpty() || ace.size() > 253)
      return {};
    host = QString::fromLatin1(ace).toLower();
    const QString unicode = QUrl::fromAce(ace, QUrl::IgnoreIDNWhitelist);
    if (QUrl::toAce(unicode, QUrl::IgnoreIDNWhitelist) != ace || !validBidi(unicode))
      return {};
    static const QRegularExpression labelPattern(
        QStringLiteral("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$"));
    for (const QString &label : host.split(u'.')) {
      if (!labelPattern.match(label).hasMatch() ||
          (label.startsWith(QStringLiteral("xn--")) &&
           QUrl::fromAce(label.toLatin1(), QUrl::IgnoreIDNWhitelist) == label))
        return {};
    }
  }
  const QString result = QStringLiteral("https://") + host + port + value.mid(end);
  QString probe = result;
  probe.replace(QStringLiteral("{csv}"), QStringLiteral("__csv__"));
  const QUrl parsed(probe, QUrl::StrictMode);
  if (result.size() > 2048 || !parsed.isValid() || parsed.host().isEmpty() ||
      parsed.scheme() != QStringLiteral("https") || !parsed.userInfo().isEmpty())
    return {};
  return result;
}
} // namespace VerificationUrl
