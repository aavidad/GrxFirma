// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef TRANSLATORBRIDGE_H
#define TRANSLATORBRIDGE_H

#include <QHash>
#include <QJsonObject>
#include <QObject>
#include <QString>
#include <QVariantList>

class TranslatorBridge : public QObject {
  Q_OBJECT
  Q_PROPERTY(QString locale READ locale WRITE setLocale NOTIFY localeChanged)
  Q_PROPERTY(QVariantList languages READ languages NOTIFY localeChanged)

public:
  explicit TranslatorBridge(QObject *parent = nullptr);

  QString locale() const { return m_locale; }
  void setLocale(const QString &locale);

  Q_INVOKABLE QString t(const QString &key) const;
  Q_INVOKABLE QString format(const QString &key,
                             const QVariantList &args = QVariantList()) const;
  Q_INVOKABLE QString displayName(const QString &code) const;
  Q_INVOKABLE QString nativeName(const QString &code) const;
  Q_INVOKABLE QString helpHtml() const;
  QVariantList languages() const;

  static QString detectLocale();
  static QString normalizeLocale(const QString &locale);
  static TranslatorBridge *shared();

signals:
  void localeChanged();

private:
  void reload();
  static QString resourcePathForLocale(const QString &locale);

  QString m_locale;
  QHash<QString, QString> m_messages;
  mutable QHash<QString, QString> m_nativeNames;
  static TranslatorBridge *s_shared;
};

#endif
