// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef ACTIVEDIAGNOSTICS_H
#define ACTIVEDIAGNOSTICS_H

#include <QObject>
#include <QStringList>
#include <QVariantMap>

class QHostInfo;
class QSslSocket;
class QTcpSocket;
class QTimer;

// Ejecuta un catálogo cerrado de comprobaciones posteriores a un fallo.
// Nunca hace tráfico al construirlo: start() exige consentimiento explícito y
// limita destino, número de conexiones y tiempo total de cada probe.
class ActiveDiagnosticsRunner final : public QObject {
  Q_OBJECT

public:
  explicit ActiveDiagnosticsRunner(QObject *parent = nullptr);
  ~ActiveDiagnosticsRunner() override;

  bool running() const { return m_running; }

  void start(const QString &localAddress, bool localTLS,
             const QStringList &expectedLocalTLSPins,
             bool localChannelAvailable, const QVariantMap &context,
             bool explicitConsent);
  void cancel();

signals:
  void runningChanged(bool running);
  void finished(bool completed, QVariantMap result);

private:
  void appendProbe(const QString &kind, const QString &state,
                   const QString &detailCode = QString(), qint64 durationMs = 0);
  void startDNSProbe();
  void startTCPProbe();
  void startTLSProbe();
  void finishCompleted();
  void finishRejected(const QString &errorCode);
  void resetNetworkObjects();

  bool m_running = false;
  bool m_targetTLS = false;
  bool m_tlsPinned = false;
  QString m_targetHost;
  QStringList m_expectedLocalTLSPins;
  quint16 m_targetPort = 0;
  QString m_passiveCategory;
  QVariantMap m_result;
  int m_dnsLookupId = -1;
  QTimer *m_timeout = nullptr;
  QTcpSocket *m_tcpSocket = nullptr;
  QSslSocket *m_tlsSocket = nullptr;
  qint64 m_probeStartedMs = 0;
};

#endif // ACTIVEDIAGNOSTICS_H
