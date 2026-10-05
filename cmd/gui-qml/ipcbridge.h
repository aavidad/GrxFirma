// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef IPCBRIDGE_H
#define IPCBRIDGE_H

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QHash>
#include <QLocalSocket>
#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QObject>
#include <QProcess>
#include <QString>
#include <QUrl>
#include <QVariantList>
#include <QVariantMap>

class ActiveDiagnosticsRunner;

class IpcBridge : public QObject {
  Q_OBJECT
  Q_PROPERTY(bool expertMode READ expertMode WRITE setExpertMode NOTIFY
                 expertModeChanged)
  Q_PROPERTY(QString status READ status NOTIFY statusChanged)
  Q_PROPERTY(QVariantMap localTLSStartupStatus READ localTLSStartupStatus NOTIFY
                 localTLSStartupStatusChanged)
  Q_PROPERTY(QVariantMap lastDiagnostic READ lastDiagnostic NOTIFY
                 lastDiagnosticChanged)
  Q_PROPERTY(QString lastRequestId READ lastRequestId NOTIFY lastRequestIdChanged)
  Q_PROPERTY(QString lastTraceId READ lastTraceId NOTIFY lastTraceIdChanged)
  Q_PROPERTY(bool webCompatibilityActive READ webCompatibilityActive NOTIFY
                 webCompatibilityActiveChanged)
  Q_PROPERTY(bool activeDiagnosticsRunning READ activeDiagnosticsRunning NOTIFY
                 activeDiagnosticsRunningChanged)

public:
  explicit IpcBridge(QObject *parent = nullptr);
  ~IpcBridge();

  QString status() const { return m_status; }
  QVariantMap localTLSStartupStatus() const { return m_localTLSStartupStatus; }
  QVariantMap lastDiagnostic() const { return m_lastDiagnostic; }
  QString lastRequestId() const { return m_lastRequestId; }
  QString lastTraceId() const { return m_lastTraceId; }
  bool webCompatibilityActive() const;
  bool activeDiagnosticsRunning() const { return m_activeDiagnosticsRunning; }

  Q_INVOKABLE void startBackend(const QString &addr, const QString &token = "",
                                const QString &mode = "ipc",
                                const QString &fingerprints = "",
                                bool useTLS = false);
  Q_INVOKABLE void stopBackend();
  Q_INVOKABLE bool canStopOwnedBackend() const;
  Q_INVOKABLE bool startTemporaryWebCompatibility(
      const QString &addr, const QString &token,
      const QString &fingerprints, bool useTLS, int durationMinutes);
  Q_INVOKABLE void stopWebCompatibility();
  Q_INVOKABLE void refreshCertificates();
  Q_INVOKABLE void exportPublicCertificate(const QString &certificateId,
                                           const QString &outputPath,
                                           const QString &format);
  Q_INVOKABLE void signFile(const QString &inputPath, const QString &outputPath,
                            int certIndex, const QString &format);
  Q_INVOKABLE void signFileAdvanced(const QString &inputPath,
                                    const QString &outputPath, int certIndex,
                                    const QVariantMap &options);
  Q_INVOKABLE void signFileMultiAdvanced(const QString &inputPath,
                                         const QString &outputPath,
                                         int certIndex,
                                         const QVariantList &additionalCertificateIds,
                                         const QVariantMap &options);
  Q_INVOKABLE void signBatchAdvanced(const QVariantList &inputPaths,
                                     const QString &directoryPath,
                                     const QString &outputDir, int certIndex,
                                     const QVariantMap &options);
  Q_INVOKABLE void verifyFile(const QString &inputPath);
  Q_INVOKABLE void verifyFileWithOriginal(const QString &inputPath,
                                          const QString &originalPath);
  // Igual, pero pide además al motor el informe imprimible (HTML) en el
  // idioma de la aplicación; llega en el campo reportHtml del resultado.
  Q_INVOKABLE void verifyFileWithReport(const QString &inputPath,
                                        const QString &originalPath);
  Q_INVOKABLE void loadProtectionRecipients();
  Q_INVOKABLE void importProtectionRecipient(const QString &path);
  Q_INVOKABLE void removeProtectionRecipient(const QString &id);
  Q_INVOKABLE void requestSmartcardStatus();
  // Firma remota CSC. El motor guarda la sesión; aquí solo llegan el estado,
  // los hosts y la descripción de los certificados remotos.
  Q_INVOKABLE void cscStatus();
  Q_INVOKABLE void cscConfigure(const QString &serviceUrl,
                                const QString &clientId);
  Q_INVOKABLE void cscConnect();
  Q_INVOKABLE void cscDisconnect();
  Q_INVOKABLE void cscSendOtp(const QString &certificateId);
  // overwriteConfirmed: la ruta viene de un diálogo de guardar del sistema que
  // ya preguntó antes de reemplazar. Sin él, el motor aplica la preferencia.
  Q_INVOKABLE void createFacturae(const QVariantMap &draft, const QString &outputPath,
                                  bool overwriteConfirmed = false);
  Q_INVOKABLE void validateInvoice(const QString &inputPath);
  Q_INVOKABLE void validateVeriFactu(const QString &inputPath);
  Q_INVOKABLE void readVeriFactuQR(const QString &url);
  Q_INVOKABLE void readVeriFactuQRFile(const QString &inputPath);
  Q_INVOKABLE void queryVeriFactuQR(const QString &url);
  Q_INVOKABLE void detectVeriFactu(const QString &inputPath);
  Q_INVOKABLE void generateENIDocument(const QVariantMap &params);
  Q_INVOKABLE void generateENIFile(const QVariantMap &params);
  Q_INVOKABLE bool startupEnabled() const;
  Q_INVOKABLE bool setStartupEnabled(bool enabled);
  Q_INVOKABLE void getSealPreview(const QVariantMap &options,
                                 const QString &requestId);
  Q_INVOKABLE void protectFileAdvanced(const QString &inputPath,
                                       const QString &outputPath,
                                       const QVariantList &recipientIds,
                                       int certIndex,
                                       const QVariantMap &options);
  Q_INVOKABLE void protectEncryptedDataFile(const QString &inputPath,
                                            const QString &outputPath,
                                            QString secretB64);
  Q_INVOKABLE void unprotectFileAdvanced(const QString &inputPath,
                                         const QString &outputPath,
                                         const QVariantMap &options);
  Q_INVOKABLE void unprotectEncryptedDataFile(const QString &inputPath,
                                              const QString &outputPath,
                                              QString secretB64);
  Q_INVOKABLE void exportVerificationReport(const QString &outputPath,
                                            const QVariantMap &details,
                                            const QString &inputPath = QString(),
                                            const QString &originalPath = QString());
  Q_INVOKABLE void saveTextReport(const QString &outputPath,
                                  const QString &content);
  Q_INVOKABLE QString incidentReportsDir() const;
  Q_INVOKABLE QString defaultIncidentReportPath(const QString &kind = QString()) const;
  Q_INVOKABLE QString saveIncidentReport(const QVariantMap &report,
                                        const QString &kind = QString());
  Q_INVOKABLE void sendIncidentReport(const QVariantMap &report,
                                      const QString &endpoint,
                                      const QString &savedIncidentPath);
  Q_INVOKABLE void runActiveDiagnostics(bool explicitConsent,
                                        const QVariantMap &context);
  Q_INVOKABLE void cancelActiveDiagnostics();
  Q_INVOKABLE void createHash(const QString &inputPath,
                              const QString &outputPath,
                              const QString &algorithm,
                              const QString &format,
                              bool recursive = false);
  Q_INVOKABLE void checkHash(const QString &inputPath,
                             const QString &hashPath,
                             const QString &outputPath = QString(),
                             const QString &algorithm = QString(),
                             bool recursive = false,
                             bool saveReportToDisk = false);
  Q_INVOKABLE void updateStatus(const QString &msg) { setStatus(msg); }
  Q_INVOKABLE void openExternal(const QString &path);
  Q_INVOKABLE void openSignedDocument(const QString &path);
  Q_INVOKABLE void openCertManager();
  Q_INVOKABLE void openLogFolder();
  Q_INVOKABLE void openIncidentFolder();
  Q_INVOKABLE void openHelpManual();
  Q_INVOKABLE void checkCertificates();
  Q_INVOKABLE void checkUpdates();
  Q_INVOKABLE bool updateEngineAvailable() const;
  Q_INVOKABLE void runTLSDiagnostics();
  Q_INVOKABLE void exportDiagnosticReport();
  Q_INVOKABLE void clearTLSTrustStore();
  Q_INVOKABLE void reinstallBrowserConnectors();
  // Service management (via IPC)
  Q_INVOKABLE void getServiceStatus();
  Q_INVOKABLE void installService();
  Q_INVOKABLE void uninstallService();
  Q_INVOKABLE void startService();
  Q_INVOKABLE void stopService();
  Q_INVOKABLE void getSettings();
  Q_INVOKABLE void getTokenSettings();
  Q_INVOKABLE void diagnoseTokenSettings();
  Q_INVOKABLE void saveTokenSettings(const QVariantMap &settings);
  Q_INVOKABLE QString tokenModuleLocalPath(const QUrl &url) const;
  Q_INVOKABLE void saveSettings(const QVariantMap &settings);
  Q_INVOKABLE void getPdfPreview(const QString &path, int page = 1,
                                 const QString &requestId = QString());
  Q_INVOKABLE void importCertificate(const QString &path,
                                     const QString &password);
  Q_INVOKABLE void requestCertificateAccessOptions();
  Q_INVOKABLE void openCertificateManager(const QString &managerId);
  Q_INVOKABLE void importCertificateToStore(const QString &path,
                                            const QString &password,
                                            const QString &targetId);
  Q_INVOKABLE void useTemporaryCertificate(const QString &path,
                                           const QString &password);
  Q_INVOKABLE void removeTemporaryCertificate(const QString &certificateId);
  Q_INVOKABLE void clearTemporaryCertificates();
  Q_INVOKABLE void installPublicRoots();
  Q_INVOKABLE void checkRestHealth(const QString &addr, bool useTLS = true);
  Q_INVOKABLE void getProxySecretStoreStatus();
  Q_INVOKABLE void storeProxyCredentials(const QString &realm,
                                         const QString &username,
                                         QString password);
  Q_INVOKABLE void deleteProxyCredentials();
  Q_INVOKABLE void checkCertificateOnline(const QString &certificateId);
  Q_INVOKABLE void shutdownForExit();

  bool expertMode() const { return m_expertMode; }
  void setExpertMode(bool v);

signals:
  void tokenSettingsFinished(QString action, bool ok, QVariantMap snapshot, QString message);
  void certificatesLoaded(QVariantList certs);
  void certificatePublicExportFinished(bool ok, QString message);
  void signingFinished(bool success, QString message, QString outputPath);
  void batchSigningFinished(bool success, QString message, QVariantList results);
  void verificationFinished(bool success, QString message, QVariantMap details);
  void protectionRecipientsLoaded(QVariantList recipients);
  void protectionRecipientChanged(bool ok, QString message);
  void smartcardStatusReceived(bool ok, QVariantList readers, QString message);
  void cscFinished(QString action, bool ok, QVariantMap data, QString message);
  void facturaeCreated(bool ok, QVariantMap result, QString message);
  void invoiceValidated(bool ok, QVariantMap result, QString message);
  void verifactuValidated(bool ok, QVariantMap result, QString message);
  void verifactuQRFinished(QString action, bool ok, QVariantMap result, QString message);
  void verifactuDetected(bool ok, QVariantMap result);
  void eniGenerated(QString action, bool ok, QVariantMap result, QString message);
  void sealPreviewReceived(QString requestId, bool ok, QString image,
                           QString message);
  void protectionFinished(bool success, QString message, QVariantMap result);
  void unprotectionFinished(bool success, QString message, QVariantMap result);
  void hashCreateFinished(bool success, QString message, QVariantMap result);
  void hashCheckFinished(bool success, QString message, QVariantMap result);
  void backendLogReceived(QString log);
  void expertModeChanged();
  void statusChanged();
  void localTLSStartupStatusChanged();
  void lastDiagnosticChanged();
  void lastRequestIdChanged();
  void lastTraceIdChanged();
  void webCompatibilityActiveChanged();
  void webCompatibilityStateChanged(bool active, int durationMinutes,
                                    QString message);
  void serviceStatusReceived(bool installed, bool running, QString platform,
                             QString method);
  void serviceActionFinished(bool ok, QString message);
  void settingsLoaded(QVariantMap settings);
  void settingsSaved(bool ok, QString message);
  void pdfPreviewReceived(QString requestId, QString requestedPath,
                          int requestedPage, bool ok, QString data,
                          double width, double height, int currentPage,
                          int totalPages);
  void certificateImportFinished(bool ok, QString message);
  void certificateAccessOptionsLoaded(bool ok, QVariantMap options,
                                      QString message);
  void temporaryCertificateFinished(bool ok, QString message,
                                    QVariantMap certificate);
  void temporaryCertificatesCleared(bool ok, QString message);
  void publicRootsInstallationFinished(bool ok, QString message);
  void restHealthChecked(bool running, QString message);
  void proxySecretStoreStatusReceived(bool available, QString platform,
                                      QString backend, QString reason,
                                      QString runtimeMode = QString());
  void proxyCredentialsFinished(bool ok, QString message, bool configured,
                                QString realm, QString username);
  void certificateOnlineCheckFinished(bool ok, QString message,
                                      QVariantMap result);
  void updateCheckFinished(bool ok, QString message, QVariantMap result);
  void incidentReportSent(bool ok, QString message, QString localPath);
  void activeDiagnosticsRunningChanged();
  void activeDiagnosticsFinished(bool completed, QVariantMap result);

private slots:
  void onReadyRead();
  void onConnected();
  void onError(QLocalSocket::LocalSocketError error);

private:
  void sendTokenSettingsRequest(const QString &action, const QVariantMap &params = QVariantMap());
  QString m_tokenSettingsRequestId;
  QString m_tokenSettingsAction;
  friend class IpcBridgeTransientSecretTest;

  bool queueDeferredRequest(const QString &action, const QVariantMap &params);
  void flushDeferredRequest();
  void failPendingActionDueToConnection(const QString &message);
  void failActionDueToConnection(const QString &action, const QString &message);
  void sendRequest(const QString &action,
                   const QVariantMap &params = QVariantMap());
  void launchBackendProcess();
  void stopOwnedWebCompatibilityServer();
  void tryConnect();
  void setStatus(const QString &s);
  void setLastDiagnostic(const QVariantMap &diagnostic);
  void setLastRequestId(const QString &requestId);
  void setLastTraceId(const QString &traceId);

  QLocalSocket *m_socket;
  QNetworkAccessManager *m_nam = nullptr;
  QProcess *m_process = nullptr;
  QProcess *m_webCompatibilityProcess = nullptr;
  QString m_status;
  QVariantMap m_localTLSStartupStatus;
  bool m_requestLocalTLSStartupAfterDeferred = false;
  bool m_refreshCertificatesAfterLocalTLSStartup = false;
  bool m_expertMode = false;
  QString m_addr;
  QString m_token;
  QString m_socketPath; // For socket connection
  QString m_serverMode = "ipc";
  QString m_fingerprints;
  bool m_useTLS = false;
  int m_retryCount = 0;
  QString m_pendingAction; // ultima accion enviada, para distinguir respuestas
  QString m_pendingRequestId;
  QString m_pendingTraceId;
  struct PreviewRequestContext {
    QString path;
    int page = 1;
  };
  QHash<QString, PreviewRequestContext> m_previewRequests;
  QHash<QString, bool> m_sealPreviewRequests;
  QByteArray m_deferredRequest;
  QString m_deferredAction;
  quint64 m_requestSeq = 0;
  bool m_shuttingDown = false;
  class WebCompatibilityLease *m_webCompatibilityLease = nullptr;
  QVariantMap m_lastDiagnostic;
  QString m_lastRequestId;
  QString m_lastTraceId;
  ActiveDiagnosticsRunner *m_activeDiagnostics = nullptr;
  bool m_activeDiagnosticsRunning = false;
};

#endif // IPCBRIDGE_H
