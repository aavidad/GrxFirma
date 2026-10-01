// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "activediagnostics.h"

#include <QAbstractSocket>
#include <QCryptographicHash>
#include <QDateTime>
#include <QHostAddress>
#include <QHostInfo>
#include <QNetworkProxyFactory>
#include <QProcessEnvironment>
#include <QRegularExpression>
#include <QSslCertificate>
#include <QSslError>
#include <QSslSocket>
#include <QTcpSocket>
#include <QTimer>

namespace {

constexpr int kDNSProbeTimeoutMs = 1500;
constexpr int kTCPProbeTimeoutMs = 2000;
constexpr int kTLSProbeTimeoutMs = 3000;

QString closedFailureCategory(const QVariantMap &context) {
  const QString raw =
      context.value(QStringLiteral("failureCategory")).toString().trimmed();
  static const QStringList allowed = {
      QStringLiteral("app_local"),          QStringLiteral("certificate_store"),
      QStringLiteral("certificate_or_device"),
      QStringLiteral("network_proxy"),      QStringLiteral("environment"),
      QStringLiteral("local_web_service"),  QStringLiteral("remote_service"),
      QStringLiteral("government_afirma"),  QStringLiteral("unknown")};
  return allowed.contains(raw) ? raw : QStringLiteral("unknown");
}

bool parseLocalAddress(const QString &address, QString *host, quint16 *port) {
  if (!host || !port)
    return false;
  const QString value = address.trimmed();
  const int separator = value.lastIndexOf(QLatin1Char(':'));
  if (separator <= 0 || separator == value.size() - 1 ||
      value.contains(QLatin1Char('/')) || value.contains(QLatin1Char('\\')))
    return false;
  bool portOK = false;
  const int parsedPort = value.mid(separator + 1).toInt(&portOK);
  const QString parsedHost = value.left(separator).trimmed();
  if (!portOK || parsedPort < 1 || parsedPort > 65535 ||
      parsedHost.isEmpty() || parsedHost.size() > 253)
    return false;
  if (parsedHost != QStringLiteral("127.0.0.1") &&
      parsedHost.compare(QStringLiteral("localhost"),
                         Qt::CaseInsensitive) != 0)
    return false;
  *host = parsedHost;
  *port = static_cast<quint16>(parsedPort);
  return true;
}

bool proxyEnvironmentConfigured() {
  const QProcessEnvironment environment =
      QProcessEnvironment::systemEnvironment();
  for (const QString &name :
       {QStringLiteral("HTTPS_PROXY"), QStringLiteral("https_proxy"),
        QStringLiteral("HTTP_PROXY"), QStringLiteral("http_proxy"),
        QStringLiteral("ALL_PROXY"), QStringLiteral("all_proxy")}) {
    if (!environment.value(name).trimmed().isEmpty())
      return true;
  }
  return false;
}

qint64 monotonicNowMs() {
  return QDateTime::currentMSecsSinceEpoch();
}

} // namespace

ActiveDiagnosticsRunner::ActiveDiagnosticsRunner(QObject *parent)
    : QObject(parent), m_timeout(new QTimer(this)) {
  m_timeout->setSingleShot(true);
}

ActiveDiagnosticsRunner::~ActiveDiagnosticsRunner() { cancel(); }

void ActiveDiagnosticsRunner::start(const QString &localAddress, bool localTLS,
                                    const QStringList &expectedLocalTLSPins,
                                    bool localChannelAvailable,
                                    const QVariantMap &context,
                                    bool explicitConsent) {
  if (m_running) {
    QVariantMap rejected{
        {QStringLiteral("schema"),
         QStringLiteral("grxfirma-active-diagnostic-v1")},
        {QStringLiteral("trigger"), QStringLiteral("post_failure")},
        {QStringLiteral("completed"), false},
        {QStringLiteral("errorCode"), QStringLiteral("already_running")}};
    emit finished(false, rejected);
    return;
  }
  if (!explicitConsent) {
    finishRejected(QStringLiteral("consent_required"));
    return;
  }
  if (!context.value(QStringLiteral("postFailure"), false).toBool()) {
    finishRejected(QStringLiteral("post_failure_required"));
    return;
  }

  m_running = true;
  emit runningChanged(true);
  m_passiveCategory = closedFailureCategory(context);
  m_expectedLocalTLSPins.clear();
  static const QRegularExpression validPin(
      QStringLiteral("^[0-9a-f]{64}$"));
  for (QString pin : expectedLocalTLSPins) {
    pin = pin.trimmed().toLower();
    if (validPin.match(pin).hasMatch() &&
        !m_expectedLocalTLSPins.contains(pin))
      m_expectedLocalTLSPins.append(pin);
  }
  m_result = {
      {QStringLiteral("schema"),
       QStringLiteral("grxfirma-active-diagnostic-v1")},
      {QStringLiteral("trigger"), QStringLiteral("post_failure")},
      {QStringLiteral("completed"), false},
      {QStringLiteral("consent"), true},
      {QStringLiteral("conclusive"), false},
      {QStringLiteral("confidence"), QStringLiteral("limited")},
      {QStringLiteral("passiveCategory"), m_passiveCategory},
      {QStringLiteral("probes"), QVariantList()}};

  appendProbe(
      QStringLiteral("proxy_configuration"), QStringLiteral("observed"),
      proxyEnvironmentConfigured()
          ? QStringLiteral("environment_configuration")
          : (QNetworkProxyFactory::usesSystemConfiguration()
                 ? QStringLiteral("system_configuration")
                 : QStringLiteral("direct_or_unknown")));

  m_targetTLS = localTLS;
  m_result.insert(QStringLiteral("scope"), QStringLiteral("local_backend"));
  appendProbe(QStringLiteral("local_channel"),
              localChannelAvailable ? QStringLiteral("ok")
                                    : QStringLiteral("failed"),
              localChannelAvailable ? QStringLiteral("available")
                                    : QStringLiteral("unavailable"));
  if (!parseLocalAddress(localAddress, &m_targetHost, &m_targetPort)) {
    finishCompleted();
    return;
  }
  startDNSProbe();
}

void ActiveDiagnosticsRunner::cancel() {
  if (!m_running)
    return;
  m_running = false;
  resetNetworkObjects();
  m_result.clear();
  emit runningChanged(false);
}

void ActiveDiagnosticsRunner::appendProbe(const QString &kind,
                                          const QString &state,
                                          const QString &detailCode,
                                          qint64 durationMs) {
  QVariantList probes = m_result.value(QStringLiteral("probes")).toList();
  QVariantMap probe{{QStringLiteral("kind"), kind},
                    {QStringLiteral("state"), state},
                    {QStringLiteral("durationMs"),
                     qBound(qint64{0}, durationMs, qint64{10000})}};
  if (!detailCode.isEmpty())
    probe.insert(QStringLiteral("detailCode"), detailCode);
  probes.append(probe);
  m_result.insert(QStringLiteral("probes"), probes);
}

void ActiveDiagnosticsRunner::startDNSProbe() {
  QHostAddress literalAddress;
  if (literalAddress.setAddress(m_targetHost)) {
    appendProbe(QStringLiteral("dns_resolution"),
                QStringLiteral("not_applicable"),
                QStringLiteral("numeric_address"));
    startTCPProbe();
    return;
  }

  m_probeStartedMs = monotonicNowMs();
  disconnect(m_timeout, nullptr, this, nullptr);
  connect(m_timeout, &QTimer::timeout, this, [this]() {
    const int lookupId = m_dnsLookupId;
    m_dnsLookupId = -1;
    if (lookupId >= 0)
      QHostInfo::abortHostLookup(lookupId);
    appendProbe(QStringLiteral("dns_resolution"), QStringLiteral("failed"),
                QStringLiteral("timeout"),
                monotonicNowMs() - m_probeStartedMs);
    finishCompleted();
  });
  m_timeout->start(kDNSProbeTimeoutMs);
  m_dnsLookupId =
      QHostInfo::lookupHost(m_targetHost, this, [this](const QHostInfo &info) {
        if (!m_running || m_dnsLookupId < 0)
          return;
        m_dnsLookupId = -1;
        m_timeout->stop();
        QHostAddress loopbackAddress;
        if (info.error() == QHostInfo::NoError) {
          for (const QHostAddress &address : info.addresses()) {
            if (address.isLoopback()) {
              loopbackAddress = address;
              break;
            }
          }
        }
        const bool ok = !loopbackAddress.isNull();
        appendProbe(QStringLiteral("dns_resolution"),
                    ok ? QStringLiteral("ok") : QStringLiteral("failed"),
                    ok ? QStringLiteral("resolved_loopback")
                       : QStringLiteral("not_resolved"),
                    monotonicNowMs() - m_probeStartedMs);
        if (ok) {
          // Evita una segunda resolución potencialmente distinta al conectar.
          m_targetHost = loopbackAddress.toString();
          startTCPProbe();
        } else {
          finishCompleted();
        }
      });
}

void ActiveDiagnosticsRunner::startTCPProbe() {
  m_probeStartedMs = monotonicNowMs();
  m_tcpSocket = new QTcpSocket(this);
  QTcpSocket *const socket = m_tcpSocket;
  disconnect(m_timeout, nullptr, this, nullptr);
  connect(m_timeout, &QTimer::timeout, this, [this, socket]() {
    if (m_tcpSocket != socket)
      return;
    m_tcpSocket = nullptr;
    disconnect(socket, nullptr, this, nullptr);
    socket->abort();
    socket->deleteLater();
    appendProbe(QStringLiteral("tcp_reachability"),
                QStringLiteral("failed"), QStringLiteral("timeout"),
                monotonicNowMs() - m_probeStartedMs);
    finishCompleted();
  });
  connect(socket, &QTcpSocket::connected, this, [this, socket]() {
    if (m_tcpSocket != socket)
      return;
    m_timeout->stop();
    socket->disconnectFromHost();
    socket->deleteLater();
    m_tcpSocket = nullptr;
    appendProbe(QStringLiteral("tcp_reachability"), QStringLiteral("ok"),
                QStringLiteral("connected"),
                monotonicNowMs() - m_probeStartedMs);
    if (m_targetTLS)
      startTLSProbe();
    else
      finishCompleted();
  });
  connect(socket, &QTcpSocket::errorOccurred, this,
          [this, socket](QAbstractSocket::SocketError) {
            if (m_tcpSocket != socket)
              return;
            m_timeout->stop();
            socket->deleteLater();
            m_tcpSocket = nullptr;
            appendProbe(QStringLiteral("tcp_reachability"),
                        QStringLiteral("failed"),
                        QStringLiteral("connection_failed"),
                        monotonicNowMs() - m_probeStartedMs);
            finishCompleted();
          });
  m_timeout->start(kTCPProbeTimeoutMs);
  socket->connectToHost(m_targetHost, m_targetPort);
}

void ActiveDiagnosticsRunner::startTLSProbe() {
  m_probeStartedMs = monotonicNowMs();
  m_tlsPinned = false;
  m_tlsSocket = new QSslSocket(this);
  QSslSocket *const socket = m_tlsSocket;
  socket->setPeerVerifyMode(QSslSocket::VerifyPeer);
  disconnect(m_timeout, nullptr, this, nullptr);
  connect(m_timeout, &QTimer::timeout, this, [this, socket]() {
    if (m_tlsSocket != socket)
      return;
    m_tlsSocket = nullptr;
    disconnect(socket, nullptr, this, nullptr);
    socket->abort();
    socket->deleteLater();
    appendProbe(QStringLiteral("tls_handshake"), QStringLiteral("failed"),
                QStringLiteral("timeout"),
                monotonicNowMs() - m_probeStartedMs);
    finishCompleted();
  });
  connect(socket, &QSslSocket::encrypted, this, [this, socket]() {
    if (m_tlsSocket != socket)
      return;
    m_timeout->stop();
    socket->disconnectFromHost();
    socket->deleteLater();
    m_tlsSocket = nullptr;
    appendProbe(QStringLiteral("tls_handshake"), QStringLiteral("ok"),
                m_tlsPinned ? QStringLiteral("pinned_verified")
                            : QStringLiteral("verified"),
                monotonicNowMs() - m_probeStartedMs);
    finishCompleted();
  });
  connect(socket, &QSslSocket::sslErrors, this,
          [this, socket](const QList<QSslError> &errors) {
            if (m_tlsSocket != socket || errors.isEmpty() ||
                m_expectedLocalTLSPins.isEmpty())
              return;
            const QSslCertificate peer = socket->peerCertificate();
            if (peer.isNull())
              return;
            const QString peerPin =
                QString::fromLatin1(
                    peer.digest(QCryptographicHash::Sha256).toHex())
                    .toLower();
            if (!m_expectedLocalTLSPins.contains(peerPin))
              return;
            m_tlsPinned = true;
            socket->ignoreSslErrors(errors);
          });
  connect(socket, &QSslSocket::errorOccurred, this,
          [this, socket](QAbstractSocket::SocketError) {
            if (m_tlsSocket != socket)
              return;
            m_timeout->stop();
            socket->deleteLater();
            m_tlsSocket = nullptr;
            appendProbe(QStringLiteral("tls_handshake"),
                        QStringLiteral("failed"),
                        QStringLiteral("handshake_failed"),
                        monotonicNowMs() - m_probeStartedMs);
            finishCompleted();
          });
  m_timeout->start(kTLSProbeTimeoutMs);
  socket->connectToHostEncrypted(m_targetHost, m_targetPort);
}

void ActiveDiagnosticsRunner::finishCompleted() {
  if (!m_running)
    return;
  resetNetworkObjects();

  const QVariantList probes =
      m_result.value(QStringLiteral("probes")).toList();
  auto probeState = [&probes](const QString &kind) {
    for (const QVariant &value : probes) {
      const QVariantMap probe = value.toMap();
      if (probe.value(QStringLiteral("kind")).toString() == kind)
        return probe.value(QStringLiteral("state")).toString();
    }
    return QString();
  };

  QString probableCause = QStringLiteral("insufficient_evidence");
  QString likelyOwner = QStringLiteral("unknown");
  QString suggestedAction = QStringLiteral("retry_and_export_incident");
  QString confidence = QStringLiteral("limited");
  if (probeState(QStringLiteral("dns_resolution")) ==
      QStringLiteral("failed")) {
    probableCause = QStringLiteral("dns_unavailable");
    likelyOwner = QStringLiteral("network_proxy");
    suggestedAction = QStringLiteral("review_network_proxy");
    confidence = QStringLiteral("moderate");
  } else if (probeState(QStringLiteral("tcp_reachability")) ==
             QStringLiteral("failed")) {
    probableCause = QStringLiteral("local_backend_unavailable");
    likelyOwner = QStringLiteral("app_local");
    suggestedAction = QStringLiteral("restart_local_backend");
    confidence = QStringLiteral("moderate");
  } else if (probeState(QStringLiteral("tls_handshake")) ==
             QStringLiteral("failed")) {
    probableCause = QStringLiteral("tls_validation_failed");
    likelyOwner = QStringLiteral("app_local");
    suggestedAction = QStringLiteral("review_tls_proxy_or_service");
    confidence = QStringLiteral("moderate");
  } else if (probeState(QStringLiteral("local_channel")) ==
             QStringLiteral("failed")) {
    probableCause = QStringLiteral("local_backend_unavailable");
    likelyOwner = QStringLiteral("app_local");
    suggestedAction = QStringLiteral("restart_local_backend");
    confidence = QStringLiteral("moderate");
  } else if (m_passiveCategory == QStringLiteral("certificate_store") ||
             m_passiveCategory ==
                 QStringLiteral("certificate_or_device")) {
    probableCause = QStringLiteral("certificate_context");
    likelyOwner = QStringLiteral("certificate_or_device");
    suggestedAction = QStringLiteral("review_certificate_or_device");
  }

  m_result.insert(QStringLiteral("probableCause"), probableCause);
  m_result.insert(QStringLiteral("likelyOwner"), likelyOwner);
  m_result.insert(QStringLiteral("suggestedAction"), suggestedAction);
  m_result.insert(QStringLiteral("confidence"), confidence);
  m_result.insert(QStringLiteral("completed"), true);
  m_running = false;
  emit runningChanged(false);
  emit finished(true, m_result);
  m_result.clear();
}

void ActiveDiagnosticsRunner::finishRejected(const QString &errorCode) {
  QVariantMap rejected{
      {QStringLiteral("schema"),
       QStringLiteral("grxfirma-active-diagnostic-v1")},
      {QStringLiteral("trigger"), QStringLiteral("post_failure")},
      {QStringLiteral("completed"), false},
      {QStringLiteral("errorCode"), errorCode}};
  emit finished(false, rejected);
}

void ActiveDiagnosticsRunner::resetNetworkObjects() {
  m_timeout->stop();
  disconnect(m_timeout, nullptr, this, nullptr);
  const int dnsLookupId = m_dnsLookupId;
  m_dnsLookupId = -1;
  if (dnsLookupId >= 0) {
    QHostInfo::abortHostLookup(dnsLookupId);
  }
  QTcpSocket *const tcpSocket = m_tcpSocket;
  m_tcpSocket = nullptr;
  if (tcpSocket) {
    disconnect(tcpSocket, nullptr, this, nullptr);
    tcpSocket->abort();
    tcpSocket->deleteLater();
  }
  QSslSocket *const tlsSocket = m_tlsSocket;
  m_tlsSocket = nullptr;
  if (tlsSocket) {
    disconnect(tlsSocket, nullptr, this, nullptr);
    tlsSocket->abort();
    tlsSocket->deleteLater();
  }
  m_targetHost.clear();
  m_targetPort = 0;
  m_expectedLocalTLSPins.clear();
  m_tlsPinned = false;
}
