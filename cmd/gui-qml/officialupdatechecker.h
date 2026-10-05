// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef OFFICIALUPDATECHECKER_H
#define OFFICIALUPDATECHECKER_H

#include <QDateTime>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QObject>
#include <QVariantMap>

class OfficialUpdateChecker : public QObject {
  Q_OBJECT
public:
  explicit OfficialUpdateChecker(QNetworkAccessManager *network = nullptr,
                                 QObject *parent = nullptr);
  Q_INVOKABLE void check(const QString &currentVersion, int timeoutMs = 10000);
  Q_INVOKABLE bool savedCheckForUpdates() const;
  Q_INVOKABLE bool shouldCheck(const QString &lastIso) const {
    return isDue(QDateTime::fromString(lastIso, Qt::ISODateWithMs),
                 QDateTime::currentDateTimeUtc());
  }
  Q_INVOKABLE bool newer(const QString &current, const QString &latest) const {
    return isNewer(current, latest);
  }
  Q_INVOKABLE bool isOfficialReleaseUrl(const QString &url) const {
    return validReleaseUrl(url);
  }
  static bool isDue(const QDateTime &last, const QDateTime &now);
  static bool isNewer(const QString &current, const QString &latest);
  static bool validReleaseUrl(const QString &url);
  static QVariantMap parse(const QByteArray &json, const QString &current);
signals:
  void finished(bool ok, const QVariantMap &result);
private:
  QNetworkAccessManager *m_network;
  bool m_ownsNetwork = false;
};
#endif
