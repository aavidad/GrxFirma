// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
#include "officialupdatechecker.h"
#include <QJsonDocument>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonParseError>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QRegularExpression>
#include <QTimer>
#include <QUrl>

namespace {
constexpr int maxBytes = 1024 * 1024;
const QString endpoint = QStringLiteral("https://api.github.com/repos/aavidad/GrxFirma/releases/latest");
const QString prefix = QStringLiteral("https://github.com/aavidad/GrxFirma/releases/tag/");
const QRegularExpression versionPattern(QStringLiteral(
    "^v?(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*))?(?:\\+([0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*))?$"));

bool numbers(const QString &raw, QList<int> &out, bool &prerelease, bool allowShort) {
  QString value = raw;
  if (allowShort && QRegularExpression(QStringLiteral("^v?(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)$")).match(value).hasMatch())
    value += QStringLiteral(".0");
  const auto match = versionPattern.match(value);
  if (!match.hasMatch()) return false;
  for (int i = 1; i <= 3; ++i) {
    bool ok = false;
    const int n = match.captured(i).toInt(&ok);
    if (!ok) return false;
    out.append(n);
  }
  prerelease = !match.captured(4).isEmpty();
  if (prerelease) {
    for (const QString &identifier : match.captured(4).split('.')) {
      if (identifier.size() > 1 && identifier.startsWith('0') &&
          QRegularExpression(QStringLiteral("^[0-9]+$")).match(identifier).hasMatch())
        return false;
    }
  }
  return true;
}
}

OfficialUpdateChecker::OfficialUpdateChecker(QNetworkAccessManager *network, QObject *parent)
    : QObject(parent), m_network(network ? network : new QNetworkAccessManager(this)),
      m_ownsNetwork(!network) {}

bool OfficialUpdateChecker::savedCheckForUpdates() const {
#ifdef Q_OS_WIN
  QString base = qEnvironmentVariable("APPDATA");
  if (base.isEmpty()) base = QDir::homePath() + QStringLiteral("/AppData/Roaming");
  const QString path = QDir(base).filePath(QStringLiteral("GrxFirma/settings.json"));
#else
  const QString path = QDir::home().filePath(QStringLiteral(".config/grxfirma/settings.json"));
#endif
  const QFileInfo info(path);
  if (!info.isFile() || info.isSymLink() || info.size() < 1 || info.size() > 4 * 1024 * 1024)
    return true;
  QFile file(path);
  if (!file.open(QIODevice::ReadOnly)) return true;
  const auto document = QJsonDocument::fromJson(file.readAll());
  const auto value = document.object().value(QStringLiteral("checkForUpdates"));
  return !value.isBool() || value.toBool();
}

bool OfficialUpdateChecker::isDue(const QDateTime &last, const QDateTime &now) {
  return !last.isValid() || last.secsTo(now) >= 5 * 60 * 60;
}

bool OfficialUpdateChecker::isNewer(const QString &current, const QString &latest) {
  QList<int> a, b;
  bool preA = false, preB = false;
  if (!numbers(current, a, preA, true) || !numbers(latest, b, preB, false) || preB) return false;
  for (int i = 0; i < 3; ++i)
    if (a[i] != b[i]) return b[i] > a[i];
  return preA && !preB;
}

bool OfficialUpdateChecker::validReleaseUrl(const QString &url) {
  if (!url.startsWith(prefix) || url.size() <= prefix.size()) return false;
  const QUrl parsed(url, QUrl::StrictMode);
  return parsed.isValid() && parsed.scheme() == QStringLiteral("https") &&
         parsed.host() == QStringLiteral("github.com") && parsed.port() == -1 &&
         parsed.userInfo().isEmpty() && !parsed.hasQuery() && !parsed.hasFragment() &&
         parsed.path().startsWith(QStringLiteral("/aavidad/GrxFirma/releases/tag/")) &&
         !parsed.path().mid(QStringLiteral("/aavidad/GrxFirma/releases/tag/").size()).contains('/') &&
         parsed.toString(QUrl::FullyEncoded) == url;
}

QVariantMap OfficialUpdateChecker::parse(const QByteArray &json, const QString &current) {
  if (json.size() > maxBytes) return {};
  QJsonParseError error;
  const auto document = QJsonDocument::fromJson(json, &error);
  if (error.error != QJsonParseError::NoError || !document.isObject()) return {};
  const QJsonObject object = document.object();
  const auto draft = object.value(QStringLiteral("draft"));
  const auto prerelease = object.value(QStringLiteral("prerelease"));
  if ((!draft.isUndefined() && !draft.isBool()) || (!prerelease.isUndefined() && !prerelease.isBool())) return {};
  if (draft.toBool() || prerelease.toBool()) return {{QStringLiteral("ignored"), true}};
  const QString tag = object.value(QStringLiteral("tag_name")).toString();
  const QString url = object.value(QStringLiteral("html_url")).toString();
  QList<int> ignored;
  bool pre = false;
  if (!numbers(tag, ignored, pre, false) || pre || !validReleaseUrl(url) ||
      url.mid(prefix.size()) != tag) return {};
  return {{QStringLiteral("version"), tag}, {QStringLiteral("url"), url},
          {QStringLiteral("newer"), isNewer(current, tag)}};
}

void OfficialUpdateChecker::check(const QString &currentVersion, int timeoutMs) {
  QNetworkRequest request{QUrl(endpoint)};
  request.setRawHeader("Accept", "application/vnd.github+json");
  request.setRawHeader("User-Agent", "GrxFirma-UpdateCheck");
  request.setRawHeader("X-GitHub-Api-Version", "2022-11-28");
  request.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                       QNetworkRequest::ManualRedirectPolicy);
  // Qt usa la validación TLS del sistema; no se acepta ninguna excepción SSL.
  auto *reply = m_network->get(request);
  auto *timer = new QTimer(reply);
  timer->setSingleShot(true);
  timer->start(qBound(1, timeoutMs, 10000));
  connect(timer, &QTimer::timeout, reply, &QNetworkReply::abort);
  connect(reply, &QNetworkReply::readyRead, this, [reply]() {
    if (reply->bytesAvailable() > maxBytes) reply->abort();
  });
  connect(reply, &QNetworkReply::finished, this, [this, reply, currentVersion]() {
    const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    if (reply->error() == QNetworkReply::ContentNotFoundError &&
        status == 404 && reply->request().url().toString() == endpoint) {
      emit finished(true, {{QStringLiteral("estado"), QStringLiteral("sin_publicaciones")}});
      reply->deleteLater();
      return;
    }
    const bool ok = reply->error() == QNetworkReply::NoError && status == 200 &&
        reply->request().url().toString() == endpoint &&
        reply->bytesAvailable() <= maxBytes;
    const QVariantMap result = ok ? parse(reply->readAll(), currentVersion) : QVariantMap{};
    emit finished(ok && !result.isEmpty(), result);
    reply->deleteLater();
  });
}
