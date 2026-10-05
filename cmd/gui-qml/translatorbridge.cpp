// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "translatorbridge.h"

#include <QFile>
#include <QJsonDocument>
#include <QJsonValue>
#include <QProcessEnvironment>

TranslatorBridge *TranslatorBridge::s_shared = nullptr;

TranslatorBridge::TranslatorBridge(QObject *parent) : QObject(parent) {
  s_shared = this;
  m_locale = normalizeLocale(detectLocale());
  reload();
}

TranslatorBridge *TranslatorBridge::shared() { return s_shared; }

QString TranslatorBridge::detectLocale() {
  const auto env = QProcessEnvironment::systemEnvironment();
  for (const QString &name :
       {QStringLiteral("LANGUAGE"), QStringLiteral("LC_ALL"),
        QStringLiteral("LC_MESSAGES"), QStringLiteral("LANG")}) {
    const QString value = env.value(name).trimmed();
    if (!value.isEmpty())
      return value;
  }
  return QStringLiteral("es");
}

QString TranslatorBridge::normalizeLocale(const QString &locale) {
  QString value = locale.trimmed().toLower();
  value.replace('_', '-');
  const int dot = value.indexOf('.');
  if (dot >= 0)
    value = value.left(dot);

  if (value.startsWith("en"))
    return QStringLiteral("en");
  if (value.startsWith("fr"))
    return QStringLiteral("fr");
  if (value.startsWith("de"))
    return QStringLiteral("de");
  if (value.startsWith("it"))
    return QStringLiteral("it");
  if (value.startsWith("pt"))
    return QStringLiteral("pt");
  if (value.startsWith("zh"))
    return QStringLiteral("zh");
  if (value.startsWith("gl"))
    return QStringLiteral("gl");
  if (value.startsWith("eu"))
    return QStringLiteral("eu");
  if (value.startsWith("ca-valencia") || value.startsWith("val") ||
      value.startsWith("va"))
    return QStringLiteral("va");
  if (value.startsWith("ca"))
    return QStringLiteral("ca");
  return QStringLiteral("es");
}

QString TranslatorBridge::resourcePathForLocale(const QString &locale) {
  return QStringLiteral(":/i18n/") + normalizeLocale(locale) +
         QStringLiteral(".json");
}

void TranslatorBridge::setLocale(const QString &locale) {
  const QString normalized = normalizeLocale(locale);
  if (normalized == m_locale)
    return;
  m_locale = normalized;
  reload();
  emit localeChanged();
}

void TranslatorBridge::reload() {
  auto loadJson = [](const QString &path) {
    QHash<QString, QString> out;
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
      return out;
    const QJsonDocument doc = QJsonDocument::fromJson(file.readAll());
    if (!doc.isObject())
      return out;
    const QJsonObject obj = doc.object();
    for (auto it = obj.begin(); it != obj.end(); ++it) {
      out.insert(it.key(), it.value().toString());
    }
    return out;
  };

  m_messages = loadJson(QStringLiteral(":/i18n/es.json"));
  const auto localeMessages = loadJson(resourcePathForLocale(m_locale));
  for (auto it = localeMessages.begin(); it != localeMessages.end(); ++it)
    m_messages.insert(it.key(), it.value());
}

QString TranslatorBridge::t(const QString &key) const {
  const auto it = m_messages.constFind(key);
  if (it == m_messages.constEnd())
    return key;
  return it.value();
}

QString TranslatorBridge::format(const QString &key,
                                 const QVariantList &args) const {
  QString value = t(key);
  for (int i = 0; i < args.size(); ++i) {
    value = value.arg(args.at(i).toString());
  }
  return value;
}

QString TranslatorBridge::displayName(const QString &code) const {
  return t(QStringLiteral("language.") + normalizeLocale(code));
}

// Nombre de cada idioma en su propio idioma («English», «Català»…), sacado de
// su catálogo para que cada persona encuentre el suyo aunque la interfaz esté
// en otro. Solo se lee la línea de la clave, sin analizar todo el catálogo.
QString TranslatorBridge::nativeName(const QString &code) const {
  const QString normalized = normalizeLocale(code);
  const auto cached = m_nativeNames.constFind(normalized);
  if (cached != m_nativeNames.constEnd())
    return cached.value();
  QString name = displayName(normalized);
  QFile file(resourcePathForLocale(normalized));
  if (file.open(QIODevice::ReadOnly)) {
    const QByteArray data = file.readAll();
    const QByteArray key = QByteArrayLiteral("\"language.") +
                           normalized.toUtf8() + QByteArrayLiteral("\"");
    const qsizetype start = data.indexOf(key);
    if (start >= 0) {
      qsizetype end = data.indexOf('\n', start);
      if (end < 0)
        end = data.size();
      QByteArray line = data.mid(start, end - start).trimmed();
      if (line.endsWith(','))
        line.chop(1);
      const QJsonDocument doc =
          QJsonDocument::fromJson(QByteArrayLiteral("{") + line + QByteArrayLiteral("}"));
      const QString value =
          doc.object().value(QStringLiteral("language.") + normalized).toString();
      if (!value.isEmpty())
        name = value;
    }
  }
  m_nativeNames.insert(normalized, name);
  return name;
}

QVariantList TranslatorBridge::languages() const {
  QVariantList out;
  const QStringList codes = {QStringLiteral("es"), QStringLiteral("ca"),
                             QStringLiteral("va"), QStringLiteral("eu"),
                             QStringLiteral("gl"), QStringLiteral("en"),
                             QStringLiteral("de"), QStringLiteral("fr"),
                             QStringLiteral("pt"), QStringLiteral("it"),
                             QStringLiteral("zh")};
  for (const auto &code : codes) {
    QVariantMap item;
    item.insert(QStringLiteral("code"), code);
    item.insert(QStringLiteral("name"), nativeName(code));
    item.insert(QStringLiteral("localName"), displayName(code));
    out.push_back(item);
  }
  return out;
}

QString TranslatorBridge::helpHtml() const {
  auto esc = [](QString value) {
    value.replace("&", "&amp;");
    value.replace("<", "&lt;");
    value.replace(">", "&gt;");
    return value;
  };

  const QString title = esc(t(QStringLiteral("help.title")));
  const QString subtitle = esc(t(QStringLiteral("help.subtitle")));
  const QString signTitle = esc(t(QStringLiteral("help.sign.title")));
  const QString signBody = esc(t(QStringLiteral("help.sign.body")));
  const QString verifyTitle = esc(t(QStringLiteral("help.verify.title")));
  const QString verifyBody = esc(t(QStringLiteral("help.verify.body")));
  const QString certsTitle = esc(t(QStringLiteral("help.certs.title")));
  const QString certsBody = esc(t(QStringLiteral("help.certs.body")));
  const QString webTitle = esc(t(QStringLiteral("help.web.title")));
  const QString webBody = esc(t(QStringLiteral("help.web.body")));
  const QString securityTitle = esc(t(QStringLiteral("help.security.title")));
  const QString securityBody = esc(t(QStringLiteral("help.security.body")));
  const QString updatesTitle = esc(t(QStringLiteral("help.updates.title")));
  const QString updatesBody = esc(t(QStringLiteral("help.updates.body")));
  const QString moreTitle = esc(t(QStringLiteral("help.more.title")));
  const QString moreBody = esc(t(QStringLiteral("help.more.body")));

  return QStringLiteral(R"HTML(
<!doctype html>
<html lang="%1">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%2</title>
  <style>
    :root { color-scheme: light; }
    body {
      margin: 0;
      padding: 32px;
      font-family: "Noto Sans", "Segoe UI", sans-serif;
      background: linear-gradient(180deg, #f5f8fc 0%%, #edf2f8 100%%);
      color: #17324d;
    }
    .wrap {
      max-width: 920px;
      margin: 0 auto;
      background: #ffffff;
      border: 1px solid #dbe5ef;
      border-radius: 18px;
      box-shadow: 0 18px 42px rgba(18, 48, 84, 0.08);
      overflow: hidden;
    }
    header {
      padding: 28px 32px 20px;
      background: linear-gradient(135deg, #0c5ea8 0%%, #1f7dd1 100%%);
      color: white;
    }
    header h1 {
      margin: 0 0 8px 0;
      font-size: 30px;
      line-height: 1.1;
    }
    header p {
      margin: 0;
      font-size: 15px;
      opacity: 0.92;
    }
    .content {
      padding: 28px 32px 32px;
      display: grid;
      gap: 16px;
    }
    section {
      border: 1px solid #e2eaf2;
      border-radius: 14px;
      padding: 18px 20px;
      background: #fbfdff;
    }
    h2 {
      margin: 0 0 8px 0;
      color: #0c5ea8;
      font-size: 18px;
    }
    p {
      margin: 0;
      line-height: 1.55;
      font-size: 14px;
    }
  </style>
</head>
<body>
  <div class="wrap">
    <header>
      <h1>%2</h1>
      <p>%3</p>
    </header>
    <div class="content">
      <section><h2>%4</h2><p>%5</p></section>
      <section><h2>%6</h2><p>%7</p></section>
      <section><h2>%8</h2><p>%9</p></section>
      <section><h2>%10</h2><p>%11</p></section>
      <section><h2>%12</h2><p>%13</p></section>
      <section><h2>%14</h2><p>%15</p></section>
      <section><h2>%16</h2><p>%17</p></section>
    </div>
  </div>
</body>
</html>
)HTML")
      .arg(m_locale, title, subtitle, signTitle, signBody, verifyTitle, verifyBody,
           certsTitle, certsBody, webTitle, webBody, securityTitle, securityBody,
           updatesTitle, updatesBody, moreTitle, moreBody);
}
