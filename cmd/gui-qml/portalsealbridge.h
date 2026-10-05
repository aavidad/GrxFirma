// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#pragma once

#include <QObject>
#include <QVariantList>
#include <QVariantMap>

class PortalSealBridge final : public QObject {
  Q_OBJECT
  Q_PROPERTY(bool active READ active CONSTANT)
  Q_PROPERTY(QString documentPath READ documentPath CONSTANT)
  Q_PROPERTY(QString signerName READ signerName CONSTANT)

public:
  explicit PortalSealBridge(QObject *parent = nullptr);
  bool load(const QString &requestPath);
  bool active() const { return m_active; }
  QString documentPath() const { return m_documentPath; }
  QString signerName() const { return m_signerName; }
  Q_INVOKABLE QString normalizeVerificationUrl(const QString &raw) const;
  Q_INVOKABLE bool hasControlOrFormat(const QString &value) const;
  Q_INVOKABLE bool submit(const QString &action, const QVariantList &placements,
                          const QVariantMap &appearance);

private:
  bool m_active = false;
  QString m_documentPath;
  QString m_resultPath;
  QString m_signerName;
};
