// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef BACKENDBRIDGE_H
#define BACKENDBRIDGE_H
#include <QCoreApplication>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QObject>
#include <QProcess>
#include <QString>
#include <QVariantList>
#include <QVariantMap>

class ActiveDiagnosticsRunner;

class BackendBridge : public QObject {
  Q_OBJECT
  Q_PROPERTY(QString status READ status NOTIFY statusChanged)
  Q_PROPERTY(bool expertMode READ expertMode WRITE setExpertMode NOTIFY
                 expertModeChanged)
  Q_PROPERTY(bool webCompatibilityActive READ webCompatibilityActive NOTIFY
                 webCompatibilityActiveChanged)
  Q_PROPERTY(bool activeDiagnosticsRunning READ activeDiagnosticsRunning NOTIFY
                 activeDiagnosticsRunningChanged)

public:
  Q_INVOKABLE void announceAccessible(QObject *target, const QString &text);
  explicit BackendBridge(QObject *parent = nullptr);
  ~BackendBridge();

  QString status() const { return m_status; }
  bool expertMode() const { return m_expertMode; }
  bool webCompatibilityActive() const;
  bool activeDiagnosticsRunning() const { return m_activeDiagnosticsRunning; }
  void setExpertMode(bool v);

  Q_INVOKABLE void startBackend(const QString &addr, const QString &token,
                                const QString &mode = "rest",
                                const QString &fingerprints = "",
                                bool useTLS = false);
  Q_INVOKABLE void stopBackend();
  Q_INVOKABLE bool canStopOwnedBackend() const;
  Q_INVOKABLE bool startTemporaryWebCompatibility(
      const QString &addr, const QString &token,
      const QString &fingerprints, bool useTLS, int durationMinutes);
  Q_INVOKABLE void stopWebCompatibility();
  Q_INVOKABLE void signFile(const QString &inputPath, const QString &outputPath,
                            int certIndex, const QString &format);
  Q_INVOKABLE void signFileAdvanced(const QString &inputPath,
                                    const QString &outputPath, int certIndex,
                                    const QVariantMap &options);
  Q_INVOKABLE void signFileMultiAdvanced(
      const QString &inputPath, const QString &outputPath, int certIndex,
      const QVariantList &additionalCertificateIds,
      const QVariantMap &options);
  Q_INVOKABLE void signBatchAdvanced(const QVariantList &inputPaths,
                                     const QString &directoryPath,
                                     const QString &outputDir, int certIndex,
                                     const QVariantMap &options);
  Q_INVOKABLE void refreshCertificates();
  Q_INVOKABLE void verifyFile(const QString &inputPath);
  Q_INVOKABLE void verifyFileWithOriginal(const QString &inputPath,
                                          const QString &originalPath);
  Q_INVOKABLE void loadProtectionRecipients();
  Q_INVOKABLE void importProtectionRecipient(const QString &path);
  Q_INVOKABLE void removeProtectionRecipient(const QString &id);
  Q_INVOKABLE void requestSmartcardStatus();
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
  Q_INVOKABLE void runTLSDiagnostics();
  Q_INVOKABLE void exportDiagnosticReport();
  Q_INVOKABLE void clearTLSTrustStore();
  Q_INVOKABLE void reinstallBrowserConnectors();
  // Programa que abre las firmas de los portales (afirma://), solo Windows.
  Q_INVOKABLE void getAfirmaHandlerStatus();
  Q_INVOKABLE void selectAfirmaHandler(const QString &handler);
  // Service management
  Q_INVOKABLE void getServiceStatus();
  Q_INVOKABLE void installService();
  Q_INVOKABLE void uninstallService();
  Q_INVOKABLE void startService();
  Q_INVOKABLE void stopService();
  Q_INVOKABLE void getSettings();
  Q_INVOKABLE void saveSettings(const QVariantMap &settings);
  Q_INVOKABLE void getPdfPreview(const QString &path, int page = 1,
                                 const QString &requestId = QString());
  Q_INVOKABLE void installCamerfirmaCerts();
  Q_INVOKABLE void importCertificate(const QString &path,
                                     const QString &password);
  Q_INVOKABLE void installPublicRoots();
  Q_INVOKABLE void checkRestHealth(const QString &addr, bool useTLS = true);
  Q_INVOKABLE void getProxySecretStoreStatus();
  Q_INVOKABLE void checkCertificateOnline(const QString &certificateId);
  Q_INVOKABLE void shutdownForExit();
  Q_INVOKABLE QString getAppDirPath() const {
#ifdef Q_OS_WIN
    return QCoreApplication::applicationDirPath();
#else
    // On Linux AppImage or direct execution, the docs are relative to the bin
    return QCoreApplication::applicationDirPath();
#endif
  }

signals:
  void statusChanged();
  void expertModeChanged();
  void webCompatibilityActiveChanged();
  void webCompatibilityStateChanged(bool active, int durationMinutes,
                                    QString message);
  void certificatesLoaded(QVariantList certs);
  void certificatePublicExportFinished(bool ok, QString message);
  void signingFinished(bool success, QString message, QString outputPath);
  void batchSigningFinished(bool success, QString message,
                            QVariantList results);
  void verificationFinished(bool success, QString message, QVariantMap details);
  void protectionRecipientsLoaded(QVariantList recipients);
  void protectionRecipientChanged(bool ok, QString message);
  void smartcardStatusReceived(bool ok, QVariantList readers, QString message);
  void sealPreviewReceived(QString requestId, bool ok, QString image,
                           QString message);
  void protectionFinished(bool success, QString message, QVariantMap result);
  void unprotectionFinished(bool success, QString message, QVariantMap result);
  void hashCreateFinished(bool success, QString message, QVariantMap result);
  void hashCheckFinished(bool success, QString message, QVariantMap result);
  void backendLogReceived(QString log);
  void afirmaHandlerFinished(QString action, bool ok, QVariantMap status,
                             QString errorCode);
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
  void publicRootsInstallationFinished(bool ok, QString message);
  void restHealthChecked(bool running, QString message);
  void proxySecretStoreStatusReceived(bool available, QString platform,
                                      QString backend, QString reason,
                                      QString runtimeMode = QString());
  // Mantiene paridad de señales con IpcBridge. La gestión de credenciales
  // solo se muestra y ejecuta en modo IPC.
  void proxyCredentialsFinished(bool ok, QString message, bool configured,
                                QString realm, QString username);
  void certificateOnlineCheckFinished(bool ok, QString message,
                                      QVariantMap result);
  void updateCheckFinished(bool ok, QString message, QVariantMap result);
  void incidentReportSent(bool ok, QString message, QString localPath);
  void activeDiagnosticsRunningChanged();
  void activeDiagnosticsFinished(bool completed, QVariantMap result);

private slots:
  void onBackendReadyRead();
  void onNetworkReplyFinished(QNetworkReply *reply);

private:
  QString m_status;
  bool m_expertMode = false;
  QProcess *m_process = nullptr;
  QNetworkAccessManager *m_nam = nullptr;
  QString m_addr = "127.0.0.1:63118";
  QString m_token;
  QString m_activePreviewRequestId;
  bool m_useTLS = false;
  bool m_shuttingDown = false;
  int m_nextRESTLifetimeMinutes = 0;
  class WebCompatibilityLease *m_webCompatibilityLease = nullptr;
  ActiveDiagnosticsRunner *m_activeDiagnostics = nullptr;
  bool m_activeDiagnosticsRunning = false;

  void setStatus(const QString &s);
};

#endif // BACKENDBRIDGE_H
