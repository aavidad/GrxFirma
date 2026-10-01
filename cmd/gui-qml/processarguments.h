// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef PROCESSARGUMENTS_H
#define PROCESSARGUMENTS_H

#include <QChar>
#include <QString>
#include <QStringList>

namespace ProcessArguments {

inline QString optionName(const QString &argument) {
  QString name = argument.trimmed();
  while (name.startsWith(QLatin1Char('-')) ||
         name.startsWith(QLatin1Char('/'))) {
    name.remove(0, 1);
  }
  const qsizetype equals = name.indexOf(QLatin1Char('='));
  if (equals >= 0)
    name.truncate(equals);
  return name;
}

inline bool looksLikeOptionForPlatform(const QString &argument,
                                       bool windowsPlatform) {
  if (argument.startsWith(QLatin1Char('-')))
    return true;
  if (!windowsPlatform || !argument.startsWith(QLatin1Char('/')))
    return false;
  const QString slashName = argument.mid(1);
  return !slashName.contains(QLatin1Char('/')) &&
         !slashName.contains(QLatin1Char('\\'));
}

inline bool looksLikeOption(const QString &argument) {
#ifdef Q_OS_WIN
  return looksLikeOptionForPlatform(argument, true);
#else
  return looksLikeOptionForPlatform(argument, false);
#endif
}

inline QString foldedName(const QString &name) {
  const QString decomposed =
      name.normalized(QString::NormalizationForm_D).toLower();
  QString folded;
  folded.reserve(decomposed.size());
  for (const QChar character : decomposed) {
    const QChar::Category category = character.category();
    if (category == QChar::Mark_NonSpacing ||
        category == QChar::Mark_SpacingCombining ||
        category == QChar::Mark_Enclosing) {
      continue;
    }
    if (character.isLetterOrNumber())
      folded.append(character);
  }
  return folded;
}

inline bool isSensitiveName(const QString &name) {
  const QString folded = foldedName(optionName(name));
  if (folded.isEmpty())
    return false;

  static const QStringList sensitiveMarkers = {
      QStringLiteral("password"),       QStringLiteral("passwd"),
      QStringLiteral("passphrase"),     QStringLiteral("contrasena"),
      QStringLiteral("secret"),         QStringLiteral("token"),
      QStringLiteral("authorization"),  QStringLiteral("autorizacion"),
      QStringLiteral("bearer"),         QStringLiteral("credential"),
      QStringLiteral("credencial"),     QStringLiteral("apikey"),
      QStringLiteral("claveapi"),       QStringLiteral("privatekey"),
      QStringLiteral("claveprivada"),   QStringLiteral("protectionkey"),
      QStringLiteral("claveproteccion"),
      QStringLiteral("clavedeproteccion"),
      QStringLiteral("encryptionkey"),  QStringLiteral("signingkey"),
      QStringLiteral("secretb64"),      QStringLiteral("clavesecreta"),
  };
  for (const QString &marker : sensitiveMarkers) {
    if (folded.contains(marker))
      return true;
  }
  return folded == QStringLiteral("pin") ||
         folded.endsWith(QStringLiteral("pin")) ||
         folded == QStringLiteral("key") ||
         folded == QStringLiteral("clave");
}

inline bool isGenericOptionContainer(const QString &argument) {
  const QString folded = foldedName(optionName(argument));
  return folded == QStringLiteral("opcion") ||
         folded == QStringLiteral("option");
}

inline bool isSafeSourceOption(const QString &argument) {
  const QString normalized =
      optionName(argument).normalized(QString::NormalizationForm_C).toLower();
  for (const QString &suffix :
       {QStringLiteral("-stdin"), QStringLiteral("-file"),
        QStringLiteral("-fichero"), QStringLiteral("-fd")}) {
    if (normalized.endsWith(suffix))
      return true;
  }
  return false;
}

inline QString inlineOptionValue(const QString &argument) {
  const qsizetype equals = argument.indexOf(QLatin1Char('='));
  if (equals < 0 || equals + 1 >= argument.size())
    return QString();
  return argument.mid(equals + 1);
}

inline bool containsSensitiveOptionNameForPlatform(
    const QStringList &arguments, bool windowsPlatform) {
  for (qsizetype index = 0; index < arguments.size(); ++index) {
    const QString &argument = arguments.at(index);
    if (looksLikeOptionForPlatform(argument, windowsPlatform) &&
        isSensitiveName(argument)) {
      const bool hasInlineValue = argument.contains(QLatin1Char('='));
      if (!isSafeSourceOption(argument) || hasInlineValue)
        return true;
    }
    if (!looksLikeOptionForPlatform(argument, windowsPlatform) ||
        !isGenericOptionContainer(argument))
      continue;

    const QString inlineValue = inlineOptionValue(argument);
    if (!inlineValue.isEmpty() && isSensitiveName(inlineValue))
      return true;
    if (index + 1 < arguments.size() &&
        isSensitiveName(arguments.at(index + 1))) {
      return true;
    }
  }
  return false;
}

inline bool containsSensitiveOptionName(const QStringList &arguments) {
#ifdef Q_OS_WIN
  return containsSensitiveOptionNameForPlatform(arguments, true);
#else
  return containsSensitiveOptionNameForPlatform(arguments, false);
#endif
}

} // namespace ProcessArguments

#endif // PROCESSARGUMENTS_H
