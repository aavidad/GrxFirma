// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "incidentprivacy.h"

#include <QJsonDocument>
#include <QJsonObject>
#include <QtTest>

namespace {

QVariantMap eligibleSavedFailure(const QString &kind =
                                     QStringLiteral("sign-failure")) {
  return {
      {QStringLiteral("schema"), QStringLiteral("grxfirma-incident-v1")},
      {QStringLiteral("generatedAt"),
       QStringLiteral("2026-07-26T10:15:30.000Z")},
      {QStringLiteral("kind"), kind},
      {QStringLiteral("userMessage"), QStringLiteral("Fallo controlado")},
      {QStringLiteral("supportPreview"),
       QVariantMap{
           {QStringLiteral("title"), QStringLiteral("Incidencia")},
           {QStringLiteral("consent"),
            QVariantMap{{QStringLiteral("required"), true},
                        {QStringLiteral("previewShown"), true},
                        {QStringLiteral("remoteSend"), false}}}}}};
}

bool writeSavedFailure(const QString &path, const QString &kind,
                       QString *error = nullptr) {
  const QByteArray data =
      QJsonDocument(QJsonObject::fromVariantMap(eligibleSavedFailure(kind)))
          .toJson(QJsonDocument::Indented);
  return IncidentWritePrivateFile(path, data, error);
}

bool setModificationTime(const QString &path, const QDateTime &modified) {
  QFile file(path);
  if (!file.open(QIODevice::ReadWrite))
    return false;
  const bool updated =
      file.setFileTime(modified, QFileDevice::FileModificationTime);
  file.close();
  return updated;
}

} // namespace

class IncidentPrivacyTest final : public QObject {
  Q_OBJECT

private slots:
  void removesSecretsPathsAndPersonalIdentifiers();
  void requiresExplicitRemoteConsent();
  void boundsNestedCollectionsAndStrings();
  void redactsFreeTextCryptoAndPersonalData();
  void createsPrivateStorage();
  void readsBoundedSanitizedLogTail();
  void keepsOnlyClosedActiveDiagnosticFields();
  void acceptsOnlyEligibleSavedFailures();
  void recordsMinimalRemoteAttemptAtomically();
  void formatsOnlyAllowlistedIpcLogMetadata();
  void sanitizesPersistentBridgeMessages();
  void opensPersistentLogWithPrivatePermissions();
  void prunesOnlyOwnedIncidentArtifacts();
  void prunesExpiredPrivateLogs();
};

void IncidentPrivacyTest::removesSecretsPathsAndPersonalIdentifiers() {
  QVariantMap payload{
      {QStringLiteral("schema"), QStringLiteral("grxfirma-incident-v1")},
      {QStringLiteral("userMessage"),
       QStringLiteral("fallo en /home/alice/expedientes/secreto.pdf "
                      "token=super-secret")},
      {QStringLiteral("operation"),
       QVariantMap{{QStringLiteral("inputPath"),
                    QStringLiteral("/mnt/privado/contrato.pdf")},
                   {QStringLiteral("outputPath"),
                    QStringLiteral("C:\\Users\\Alice\\firma.pdf")},
                   {QStringLiteral("selectedCertificateLabel"),
                    QStringLiteral("Alice Example")},
                   {QStringLiteral("batchCount"), 2}}},
      {QStringLiteral("extra"),
       QVariantMap{{QStringLiteral("rawURI"),
                    QStringLiteral("afirma://sign?dat=SECRETO")},
                   {QStringLiteral("nested"),
                    QVariantList{QVariantMap{
                        {QStringLiteral("p12B64"), QStringLiteral("BASE64KEY")},
                        {QStringLiteral("ok"), false}}}}}}};

  const QVariantMap sanitized = IncidentSanitizePayload(payload, false);
  const QByteArray json =
      QJsonDocument(QJsonObject::fromVariantMap(sanitized))
          .toJson(QJsonDocument::Compact);
  QVERIFY(!json.contains("super-secret"));
  QVERIFY(!json.contains("SECRETO"));
  QVERIFY(!json.contains("BASE64KEY"));
  QVERIFY(!json.contains("/home/alice"));
  QVERIFY(!json.contains("/mnt/privado"));
  QVERIFY(!json.contains("C:\\\\Users"));
  QVERIFY(!json.contains("Alice Example"));
  QVERIFY(json.contains("contrato.pdf"));
  QVERIFY(json.contains("firma.pdf"));
  QVERIFY(json.contains("[REDACTED]"));
}

void IncidentPrivacyTest::requiresExplicitRemoteConsent() {
  QVariantMap payload{
      {QStringLiteral("supportPreview"),
       QVariantMap{{QStringLiteral("consent"),
                    QVariantMap{{QStringLiteral("required"), true},
                                {QStringLiteral("previewShown"), true},
                                {QStringLiteral("remoteSend"), false}}}}}};
  QVERIFY(!IncidentHasExplicitRemoteConsent(payload));
  QVariantMap local = IncidentSanitizePayload(payload, false);
  QVERIFY(!local.value(QStringLiteral("supportPreview"))
               .toMap()
               .value(QStringLiteral("consent"))
               .toMap()
               .value(QStringLiteral("remoteSend"))
               .toBool());

  QVariantMap preview = payload.value(QStringLiteral("supportPreview")).toMap();
  QVariantMap consent = preview.value(QStringLiteral("consent")).toMap();
  consent.insert(QStringLiteral("remoteSend"), true);
  preview.insert(QStringLiteral("consent"), consent);
  payload.insert(QStringLiteral("supportPreview"), preview);
  QVERIFY(IncidentHasExplicitRemoteConsent(payload));
  QVariantMap remote = IncidentSanitizePayload(payload, true);
  QVERIFY(remote.value(QStringLiteral("supportPreview"))
              .toMap()
              .value(QStringLiteral("consent"))
              .toMap()
              .value(QStringLiteral("remoteSend"))
              .toBool());

  consent.insert(QStringLiteral("remoteSend"), QStringLiteral("true"));
  preview.insert(QStringLiteral("consent"), consent);
  payload.insert(QStringLiteral("supportPreview"), preview);
  QVERIFY(!IncidentHasExplicitRemoteConsent(payload));
}

void IncidentPrivacyTest::boundsNestedCollectionsAndStrings() {
  QVariantList oversized;
  for (int i = 0; i < 200; ++i)
    oversized.append(QString(5000, QLatin1Char('x')));
  QVariantMap payload{
      {QStringLiteral("supportPreview"),
       QVariantMap{{QStringLiteral("includedData"), oversized}}}};
  const QVariantMap sanitized = IncidentSanitizePayload(payload, false);
  const QVariantList items =
      sanitized.value(QStringLiteral("supportPreview"))
          .toMap()
          .value(QStringLiteral("includedData"))
          .toList();
  QVERIFY(items.size() <= 129);
  QVERIFY(items.first().toString().size() <= 4097);
  const QByteArray json =
      QJsonDocument(QJsonObject::fromVariantMap(sanitized))
          .toJson(QJsonDocument::Compact);
  QVERIFY(json.size() < 150 * 1024);
}

void IncidentPrivacyTest::redactsFreeTextCryptoAndPersonalData() {
  const QString base64(100, QLatin1Char('A'));
  QVariantMap payload{
      {QStringLiteral("userMessage"),
       QStringLiteral("afirma://sign?id=123 correo alice@example.test "
                      "DNI 12345678Z Authorization: Basic c2VjcmV0Cg==\n"
                      "{\"token\":\"short-secret\"} "
                      "https://example.test/?api_key=query-secret "
                      "-----BEGIN PRIVATE KEY-----\n%1\n"
                      "-----END PRIVATE KEY-----")
           .arg(base64)},
      {QStringLiteral("Bearer top-secret-as-key"),
       QStringLiteral("must not survive")}};
  const QByteArray json =
      QJsonDocument(
          QJsonObject::fromVariantMap(IncidentSanitizePayload(payload, false)))
          .toJson(QJsonDocument::Compact);
  QVERIFY(!json.contains("id=123"));
  QVERIFY(!json.contains("alice@example.test"));
  QVERIFY(!json.contains("12345678Z"));
  QVERIFY(!json.contains("PRIVATE KEY"));
  QVERIFY(!json.contains(base64.toUtf8()));
  QVERIFY(!json.contains("top-secret-as-key"));
  QVERIFY(!json.contains("must not survive"));
  QVERIFY(!json.contains("c2VjcmV0Cg"));
  QVERIFY(!json.contains("short-secret"));
  QVERIFY(!json.contains("query-secret"));
}

void IncidentPrivacyTest::createsPrivateStorage() {
  QTemporaryDir temp;
  QVERIFY(temp.isValid());
  const QString incidentDir =
      QDir(temp.path()).filePath(QStringLiteral("incidents"));
  QString error;
  QVERIFY2(IncidentEnsurePrivateDirectory(incidentDir, &error),
           qPrintable(error));

  QFile file(QDir(incidentDir).filePath(QStringLiteral("report.json")));
  QVERIFY2(IncidentWritePrivateFile(file.fileName(), QByteArrayLiteral("{}"),
                                    &error),
           qPrintable(error));
#ifdef Q_OS_UNIX
  QCOMPARE(QFileInfo(incidentDir).permissions() &
               (QFileDevice::ReadGroup | QFileDevice::WriteGroup |
                QFileDevice::ExeGroup | QFileDevice::ReadOther |
                QFileDevice::WriteOther | QFileDevice::ExeOther),
           QFileDevice::Permissions{});
  QCOMPARE(QFileInfo(file).permissions() &
               (QFileDevice::ReadGroup | QFileDevice::WriteGroup |
                QFileDevice::ExeGroup | QFileDevice::ReadOther |
                QFileDevice::WriteOther | QFileDevice::ExeOther),
           QFileDevice::Permissions{});
#endif

  const QString realDir =
      QDir(temp.path()).filePath(QStringLiteral("real-incidents"));
  QVERIFY(QDir().mkpath(realDir));
  const QString linkedDir =
      QDir(temp.path()).filePath(QStringLiteral("linked-incidents"));
  if (QFile::link(realDir, linkedDir))
    QVERIFY(!IncidentEnsurePrivateDirectory(linkedDir, &error));
}

void IncidentPrivacyTest::readsBoundedSanitizedLogTail() {
  QTemporaryDir temp;
  QVERIFY(temp.isValid());
  const QString logPath =
      QDir(temp.path()).filePath(QStringLiteral("gui.log"));
  QFile log(logPath);
  QVERIFY(log.open(QIODevice::WriteOnly));
  log.write(QByteArray(200000, 'x'));
  log.write("\nAuthorization: Bearer tail-secret\n");
  log.close();
  const QString tail = IncidentReadSanitizedTextTail(logPath, 1024);
  QVERIFY(tail.size() < 1024);
  QVERIFY(!tail.contains(QStringLiteral("tail-secret")));
  QVERIFY(tail.contains(QStringLiteral("[AUTH_HEADER]")));
}

void IncidentPrivacyTest::keepsOnlyClosedActiveDiagnosticFields() {
  const QVariantMap payload{
      {QStringLiteral("activeDiagnostic"),
       QVariantMap{
           {QStringLiteral("schema"),
            QStringLiteral("grxfirma-active-diagnostic-v1")},
           {QStringLiteral("probableCause"),
            QStringLiteral("dns_unavailable")},
           {QStringLiteral("originIncidentFileName"),
            QStringLiteral("/home/alice/incidents/fallo.json")},
           {QStringLiteral("endpoint"),
            QStringLiteral("https://user:secret@example.test/private")},
           {QStringLiteral("probes"),
            QVariantList{QVariantMap{
                {QStringLiteral("kind"), QStringLiteral("dns_resolution")},
                {QStringLiteral("state"), QStringLiteral("failed")},
                {QStringLiteral("detailCode"),
                 QStringLiteral("not_resolved")},
                {QStringLiteral("durationMs"), 42},
                {QStringLiteral("rawError"),
                 QStringLiteral("secret host example.test")}}}}}}};

  const QVariantMap sanitized = IncidentSanitizePayload(payload, false);
  const QByteArray json =
      QJsonDocument(QJsonObject::fromVariantMap(sanitized))
          .toJson(QJsonDocument::Compact);
  QVERIFY(json.contains("activeDiagnostic"));
  QVERIFY(json.contains("dns_resolution"));
  QVERIFY(json.contains("fallo.json"));
  QVERIFY(!json.contains("/home/alice"));
  QVERIFY(!json.contains("secret"));
  QVERIFY(!json.contains("example.test"));
  QVERIFY(!json.contains("rawError"));
}

void IncidentPrivacyTest::acceptsOnlyEligibleSavedFailures() {
  QTemporaryDir temp;
  QVERIFY(temp.isValid());
  const QString incidentDir =
      QDir(temp.path()).filePath(QStringLiteral("incidents"));
  QString error;
  QVERIFY2(IncidentEnsurePrivateDirectory(incidentDir, &error),
           qPrintable(error));

  const QString eligiblePath = QDir(incidentDir).filePath(
      QStringLiteral(
          "incident-sign-failure-20260726-101530-000-deadbeef.json"));
  QVERIFY2(writeSavedFailure(eligiblePath, QStringLiteral("sign-failure"),
                             &error),
           qPrintable(error));
  QVariantMap loaded;
  QVERIFY2(IncidentLoadEligibleSavedReport(eligiblePath, incidentDir, &loaded,
                                          &error),
           qPrintable(error));
  QCOMPARE(loaded.value(QStringLiteral("kind")).toString(),
           QStringLiteral("sign-failure"));

  const QString genericPath = QDir(incidentDir).filePath(
      QStringLiteral("incident-support-20260726-101531-000-cafebabe.json"));
  QVERIFY2(writeSavedFailure(genericPath, QStringLiteral("support"), &error),
           qPrintable(error));
  QVERIFY(!IncidentLoadEligibleSavedReport(genericPath, incidentDir, &loaded,
                                           &error));

  const QString otherDir =
      QDir(temp.path()).filePath(QStringLiteral("other-incidents"));
  const QString outsidePath = QDir(otherDir).filePath(
      QStringLiteral(
          "incident-sign-failure-20260726-101532-000-acde1234.json"));
  QVERIFY2(writeSavedFailure(outsidePath, QStringLiteral("sign-failure"),
                             &error),
           qPrintable(error));
  QVERIFY(!IncidentLoadEligibleSavedReport(outsidePath, incidentDir, &loaded,
                                           &error));

  const QString linkPath = QDir(incidentDir).filePath(
      QStringLiteral(
          "incident-sign-failure-20260726-101533-000-acde5678.json"));
  if (QFile::link(eligiblePath, linkPath) && QFileInfo(linkPath).isSymLink()) {
    QVERIFY(!IncidentLoadEligibleSavedReport(linkPath, incidentDir, &loaded,
                                             &error));
  }
}

void IncidentPrivacyTest::recordsMinimalRemoteAttemptAtomically() {
  QTemporaryDir temp;
  QVERIFY(temp.isValid());
  const QString incidentDir =
      QDir(temp.path()).filePath(QStringLiteral("incidents"));
  const QString reportPath = QDir(incidentDir).filePath(
      QStringLiteral(
          "incident-verify-failure-20260726-101530-000-deadbeef.json"));
  QVariantMap report = eligibleSavedFailure(QStringLiteral("verify-failure"));
  report.insert(QStringLiteral("endpoint"),
                QStringLiteral("https://secret.example.test/upload"));
  report.insert(QStringLiteral("rawResponse"),
                QStringLiteral("server-secret-response"));
  QString error;
  QVERIFY2(
      IncidentWritePrivateFile(
          reportPath,
          QJsonDocument(QJsonObject::fromVariantMap(report))
              .toJson(QJsonDocument::Indented),
          &error),
      qPrintable(error));

  const QDateTime attemptedAt = QDateTime::fromString(
      QStringLiteral("2026-07-26T10:20:30.456Z"), Qt::ISODateWithMs);
  QVERIFY2(IncidentRecordRemoteAttempt(reportPath, incidentDir, true,
                                      attemptedAt, &error),
           qPrintable(error));

  QFile stored(reportPath);
  QVERIFY(stored.open(QIODevice::ReadOnly));
  const QByteArray bytes = stored.readAll();
  const QJsonObject object = QJsonDocument::fromJson(bytes).object();
  const QJsonObject attempt =
      object.value(QStringLiteral("remoteAttempt")).toObject();
  QCOMPARE(attempt.size(), 2);
  QCOMPARE(attempt.value(QStringLiteral("attemptedAtUtc")).toString(),
           QStringLiteral("2026-07-26T10:20:30.456Z"));
  QCOMPARE(attempt.value(QStringLiteral("result")).toString(),
           QStringLiteral("sent"));
  QVERIFY(!bytes.contains("secret.example.test"));
  QVERIFY(!bytes.contains("server-secret-response"));
  QVERIFY(!bytes.contains("endpoint"));
  QVERIFY(!bytes.contains("rawResponse"));
}

void IncidentPrivacyTest::formatsOnlyAllowlistedIpcLogMetadata() {
  const QString rawRequestId =
      QStringLiteral("ipc-1722000000000-42");
  const QVariantMap message{
      {QStringLiteral("action"), QStringLiteral("sign")},
      {QStringLiteral("requestId"), rawRequestId},
      {QStringLiteral("traceId"),
       QStringLiteral("trace-1722000000000-42")},
      {QStringLiteral("params"),
       QVariantMap{
           {QStringLiteral("inputPath"),
            QStringLiteral("/home/alice/contrato.pdf")},
           {QStringLiteral("subject"), QStringLiteral("CN=Alice Example")},
           {QStringLiteral("issuer"), QStringLiteral("Test CA")},
           {QStringLiteral("nif"), QStringLiteral("12345678Z")},
           {QStringLiteral("fingerprint"),
            QStringLiteral(
                "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")},
           {QStringLiteral("serial"), QStringLiteral("112233445566")},
           {QStringLiteral("payload"),
            QStringLiteral("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=")}}},
      {QStringLiteral("data"),
       QVariantMap{{QStringLiteral("content_base64"),
                    QStringLiteral("c2VjcmV0LWRvY3VtZW50")}}}};

  const QString formatted = IncidentFormatIpcLogEvent(
      message, QStringLiteral("ipc-request"), QStringLiteral("pending"),
      4096);
  const QJsonObject event =
      QJsonDocument::fromJson(formatted.toUtf8()).object();
  QCOMPARE(event.size(), 7);
  QCOMPARE(event.value(QStringLiteral("action")).toString(),
           QStringLiteral("sign"));
  QCOMPARE(event.value(QStringLiteral("category")).toString(),
           QStringLiteral("ipc-request"));
  QCOMPARE(event.value(QStringLiteral("result")).toString(),
           QStringLiteral("pending"));
  QCOMPARE(event.value(QStringLiteral("wireBytes")).toInteger(), 4096);
  QVERIFY(event.value(QStringLiteral("paramsBytes")).toInteger() > 0);
  QVERIFY(event.value(QStringLiteral("dataBytes")).toInteger() > 0);
  QVERIFY(QRegularExpression(QStringLiteral("^ref-[0-9a-f]{16}$"))
              .match(event.value(QStringLiteral("reference")).toString())
              .hasMatch());
  const QJsonObject withoutReference =
      QJsonDocument::fromJson(
          IncidentFormatIpcLogEvent(
              QVariantMap{{QStringLiteral("action"),
                           QStringLiteral("refresh_certificates")}},
              QStringLiteral("ipc-request"), QStringLiteral("pending"))
              .toUtf8())
          .object();
  QVERIFY(!withoutReference.contains(QStringLiteral("reference")));
  for (const QString &forbidden :
       {QStringLiteral("params"), QStringLiteral("data"),
        QStringLiteral("requestId"), QStringLiteral("traceId")}) {
    QVERIFY2(!event.contains(forbidden), qPrintable(forbidden));
  }
  for (const QString &marker :
       {rawRequestId, QStringLiteral("/home/alice"),
        QStringLiteral("Alice Example"), QStringLiteral("Test CA"),
        QStringLiteral("12345678Z"),
        QStringLiteral("0123456789abcdef"),
        QStringLiteral("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo"),
        QStringLiteral("c2VjcmV0LWRvY3VtZW50")}) {
    QVERIFY2(!formatted.contains(marker), qPrintable(marker));
  }
}

void IncidentPrivacyTest::sanitizesPersistentBridgeMessages() {
  const QString fingerprint(64, QLatin1Char('a'));
  const QString raw =
      QStringLiteral(
          "requestId=ipc-1722000000000-42 traceId=trace-raw "
          "subject=Alice issuer=TestCA serial=998877 NIF=12345678Z "
          "fingerprint=%1 payload=QUJDREVGRw== "
          "https://192.0.2.163/private /home/alice/document.pdf")
          .arg(fingerprint);
  const QString sanitized = IncidentSanitizePersistentLogMessage(raw);
  for (const QString &marker :
       {QStringLiteral("ipc-1722000000000-42"),
        QStringLiteral("trace-raw"), QStringLiteral("Alice"),
        QStringLiteral("TestCA"), QStringLiteral("998877"),
        QStringLiteral("12345678Z"), fingerprint,
        QStringLiteral("QUJDREVGRw"), QStringLiteral("192.0.2.163"),
        QStringLiteral("/home/alice")}) {
    QVERIFY2(!sanitized.contains(marker), qPrintable(marker));
  }

  const QString structured = IncidentSanitizePersistentLogMessage(
      QStringLiteral(
          "respuesta={\"data\":{\"subject\":\"Alice\","
          "\"content_base64\":\"c2VjcmV0\"}}"));
  QVERIFY(structured.startsWith(
      QStringLiteral("[STRUCTURED_DATA_REDACTED] bytes=")));
  QVERIFY(!structured.contains(QStringLiteral("Alice")));
  QVERIFY(!structured.contains(QStringLiteral("c2VjcmV0")));
}

void IncidentPrivacyTest::opensPersistentLogWithPrivatePermissions() {
  QTemporaryDir temp;
  QVERIFY(temp.isValid());
  const QString logPath =
      QDir(temp.path()).filePath(QStringLiteral("logs/gui-qml.log"));
  QFile log(logPath);
  QString error;
  QVERIFY2(IncidentOpenPrivateAppendFile(&log, &error), qPrintable(error));
  QCOMPARE(log.write(QByteArrayLiteral("safe-event\n")), qint64{11});
  log.close();
#ifdef Q_OS_UNIX
  QCOMPARE(QFileInfo(QFileInfo(logPath).absolutePath()).permissions() &
               (QFileDevice::ReadGroup | QFileDevice::WriteGroup |
                QFileDevice::ExeGroup | QFileDevice::ReadOther |
                QFileDevice::WriteOther | QFileDevice::ExeOther),
           QFileDevice::Permissions{});
  QCOMPARE(QFileInfo(logPath).permissions() &
               (QFileDevice::ReadGroup | QFileDevice::WriteGroup |
                QFileDevice::ExeGroup | QFileDevice::ReadOther |
                QFileDevice::WriteOther | QFileDevice::ExeOther),
           QFileDevice::Permissions{});
  QVERIFY(QFile::setPermissions(
      logPath, QFileDevice::ReadOwner | QFileDevice::WriteOwner |
                   QFileDevice::ReadGroup | QFileDevice::WriteGroup));
  QFile reopened(logPath);
  QVERIFY2(IncidentOpenPrivateAppendFile(&reopened, &error),
           qPrintable(error));
  reopened.close();
  QCOMPARE(QFileInfo(logPath).permissions() &
               (QFileDevice::ReadGroup | QFileDevice::WriteGroup |
                QFileDevice::ExeGroup | QFileDevice::ReadOther |
                QFileDevice::WriteOther | QFileDevice::ExeOther),
           QFileDevice::Permissions{});
#endif

  const QString realPath =
      QDir(temp.path()).filePath(QStringLiteral("real.log"));
  QFile real(realPath);
  QVERIFY(real.open(QIODevice::WriteOnly));
  real.close();
  const QString linkedPath =
      QDir(temp.path()).filePath(QStringLiteral("logs/linked.log"));
  if (QFile::link(realPath, linkedPath) &&
      QFileInfo(linkedPath).isSymLink()) {
    QFile linked(linkedPath);
    QVERIFY(!IncidentOpenPrivateAppendFile(&linked, &error));
  }
}

void IncidentPrivacyTest::prunesOnlyOwnedIncidentArtifacts() {
  QTemporaryDir temp;
  QVERIFY(temp.isValid());
  const QString incidentDir =
      QDir(temp.path()).filePath(QStringLiteral("incidents"));
  QString error;
  QVERIFY2(IncidentEnsurePrivateDirectory(incidentDir, &error),
           qPrintable(error));

  const QDateTime now =
      QDateTime::fromString(QStringLiteral("2026-07-29T12:00:00Z"),
                            Qt::ISODate);
  const QStringList bases = {
      QStringLiteral(
          "incident-sign-failure-20260501-120000-000-00000001"),
      QStringLiteral(
          "incident-sign-failure-20260728-120000-000-00000002"),
      QStringLiteral(
          "incident-sign-failure-20260729-110000-000-00000003")};
  for (int index = 0; index < bases.size(); ++index) {
    const QString basePath = QDir(incidentDir).filePath(bases.at(index));
    QVERIFY2(IncidentWritePrivateFile(basePath + QStringLiteral(".json"),
                                      QByteArrayLiteral("{}"), &error),
             qPrintable(error));
    QVERIFY2(IncidentWritePrivateFile(basePath + QStringLiteral(".txt"),
                                      QByteArrayLiteral("resumen"), &error),
             qPrintable(error));
    const QDateTime modified =
        index == 0 ? now.addDays(-60) : now.addSecs(index);
    QVERIFY(setModificationTime(basePath + QStringLiteral(".json"), modified));
    QVERIFY(setModificationTime(basePath + QStringLiteral(".txt"), modified));
  }

  const QString foreignPath =
      QDir(incidentDir).filePath(QStringLiteral("notas-del-usuario.txt"));
  QVERIFY2(IncidentWritePrivateFile(foreignPath, QByteArrayLiteral("no borrar"),
                                    &error),
           qPrintable(error));
  const QString externalPath =
      QDir(temp.path()).filePath(QStringLiteral("externo.json"));
  QVERIFY2(IncidentWritePrivateFile(externalPath, QByteArrayLiteral("externo"),
                                    &error),
           qPrintable(error));
  const QString linkedPath = QDir(incidentDir).filePath(
      QStringLiteral(
          "incident-sign-failure-20250101-000000-000-deadbeef.json"));
  const bool linked = QFile::link(externalPath, linkedPath) &&
                      QFileInfo(linkedPath).isSymLink();

  int removed = 0;
  QVERIFY2(IncidentApplyReportRetention(incidentDir, now, 30, 1, &removed,
                                        &error),
           qPrintable(error));
  QCOMPARE(removed, 4);
  QVERIFY(!QFileInfo::exists(
      QDir(incidentDir).filePath(bases.at(0) + QStringLiteral(".json"))));
  QVERIFY(!QFileInfo::exists(
      QDir(incidentDir).filePath(bases.at(1) + QStringLiteral(".json"))));
  QVERIFY(QFileInfo::exists(
      QDir(incidentDir).filePath(bases.at(2) + QStringLiteral(".json"))));
  QVERIFY(QFileInfo::exists(foreignPath));
  QVERIFY(QFileInfo::exists(externalPath));
  if (linked)
    QVERIFY(QFileInfo::exists(linkedPath));
}

void IncidentPrivacyTest::prunesExpiredPrivateLogs() {
  QTemporaryDir temp;
  QVERIFY(temp.isValid());
  const QString logPath =
      QDir(temp.path()).filePath(QStringLiteral("logs/gui-qml.log"));
  QString error;
  QVERIFY2(IncidentWritePrivateFile(logPath, QByteArrayLiteral("old"), &error),
           qPrintable(error));
  QVERIFY2(IncidentWritePrivateFile(logPath + QStringLiteral(".1"),
                                    QByteArrayLiteral("older"), &error),
           qPrintable(error));
  const QDateTime now =
      QDateTime::fromString(QStringLiteral("2026-07-29T12:00:00Z"),
                            Qt::ISODate);
  QVERIFY(setModificationTime(logPath, now.addDays(-31)));
  QVERIFY(setModificationTime(logPath + QStringLiteral(".1"),
                              now.addDays(-60)));

  int removed = 0;
  QVERIFY2(IncidentApplyPrivateLogRetention(logPath, now, 30, &removed,
                                            &error),
           qPrintable(error));
  QCOMPARE(removed, 2);
  QVERIFY(!QFileInfo::exists(logPath));
  QVERIFY(!QFileInfo::exists(logPath + QStringLiteral(".1")));

  const QString external =
      QDir(temp.path()).filePath(QStringLiteral("external.log"));
  QVERIFY2(IncidentWritePrivateFile(external, QByteArrayLiteral("external"),
                                    &error),
           qPrintable(error));
  if (QFile::link(external, logPath) && QFileInfo(logPath).isSymLink())
    QVERIFY(!IncidentApplyPrivateLogRetention(logPath, now, 30, nullptr,
                                              &error));
  QVERIFY(QFileInfo::exists(external));
}

QTEST_MAIN(IncidentPrivacyTest)

#include "incidentprivacy_test.moc"
