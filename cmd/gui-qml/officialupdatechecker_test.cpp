// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
#include "officialupdatechecker.h"
#include <QtTest>
#include <QTimer>
#include <cstring>

class FakeReply : public QNetworkReply {
public:
  FakeReply(const QNetworkRequest &request, QByteArray bytes, QObject *parent)
      : QNetworkReply(parent), m_bytes(std::move(bytes)) {
    setRequest(request);
    setUrl(request.url());
    setAttribute(QNetworkRequest::HttpStatusCodeAttribute, 200);
    open(QIODevice::ReadOnly | QIODevice::Unbuffered);
    QTimer::singleShot(0, this, [this]() { emit readyRead(); emit finished(); });
  }
  void abort() override { setError(QNetworkReply::OperationCanceledError, QString()); emit finished(); }
  qint64 bytesAvailable() const override { return m_bytes.size() - m_offset + QNetworkReply::bytesAvailable(); }
protected:
  qint64 readData(char *destination, qint64 maximum) override {
    const qint64 count = qMin(maximum, qint64(m_bytes.size()) - m_offset);
    if (count <= 0) return -1;
    memcpy(destination, m_bytes.constData() + m_offset, size_t(count));
    m_offset += count;
    return count;
  }
private:
  QByteArray m_bytes;
  qint64 m_offset = 0;
};

class FakeNetwork : public QNetworkAccessManager {
public:
  QUrl seen;
protected:
  QNetworkReply *createRequest(Operation operation, const QNetworkRequest &request,
                               QIODevice *outgoingData) override {
    Q_UNUSED(operation)
    Q_UNUSED(outgoingData)
    seen = request.url();
    return new FakeReply(request,
        QByteArray(R"({"tag_name":"v0.0.108","html_url":"https://github.com/aavidad/GrxFirma/releases/tag/v0.0.108"})"), this);
  }
};

class OfficialUpdateCheckerTest : public QObject {
  Q_OBJECT
private slots:
  void injectedNetwork() {
    FakeNetwork network;
    OfficialUpdateChecker checker(&network);
    QSignalSpy finished(&checker, &OfficialUpdateChecker::finished);
    checker.check("0.0.105");
    QVERIFY(finished.wait(1000));
    QCOMPARE(network.seen.toString(), QStringLiteral("https://api.github.com/repos/aavidad/GrxFirma/releases/latest"));
    QVERIFY(finished.takeFirst().at(0).toBool());
  }
  void versions() {
    QVERIFY(OfficialUpdateChecker::isNewer("0.0.105", "v0.0.108"));
    QVERIFY(!OfficialUpdateChecker::isNewer("0.0.108", "v0.0.108"));
    QVERIFY(!OfficialUpdateChecker::isNewer("dev", "v0.0.108"));
    QVERIFY(OfficialUpdateChecker::isNewer("1.0.0-rc1", "1.0.0"));
    QVERIFY(!OfficialUpdateChecker::isNewer("1.0.0", "2.0.0-rc1"));
  }
  void response() {
    const QByteArray valid = R"({"tag_name":"v0.0.108","html_url":"https://github.com/aavidad/GrxFirma/releases/tag/v0.0.108","draft":false,"prerelease":false})";
    const auto release = OfficialUpdateChecker::parse(valid, "0.0.105");
    QCOMPARE(release.value("version").toString(), QStringLiteral("v0.0.108"));
    QVERIFY(release.value("newer").toBool());
    const auto draft = valid;
    QByteArray draftRelease = draft;
    draftRelease.replace("\"draft\":false", "\"draft\":true");
    QVERIFY(OfficialUpdateChecker::parse(draftRelease, "0.0.105").value("ignored").toBool());
    QByteArray prereleaseRelease = valid;
    prereleaseRelease.replace("\"prerelease\":false", "\"prerelease\":true");
    QVERIFY(OfficialUpdateChecker::parse(prereleaseRelease, "0.0.105").value("ignored").toBool());
    const QByteArray oversized(1024 * 1024 + 1, 'x');
    QVERIFY(OfficialUpdateChecker::parse(oversized, "0.0.105").isEmpty());
    const QByteArray external = "{\"tag_name\":\"v0.0.108\",\"html_url\":\"https://github.com.evil/aavidad/GrxFirma/releases/tag/v0.0.108\"}";
    QVERIFY(OfficialUpdateChecker::parse(external, "0.0.105").isEmpty());
    const QByteArray download = "{\"tag_name\":\"v0.0.108\",\"html_url\":\"https://github.com/aavidad/GrxFirma/releases/download/v0.0.108/setup.exe\"}";
    QVERIFY(OfficialUpdateChecker::parse(download, "0.0.105").isEmpty());
    const QByteArray mismatch = "{\"tag_name\":\"v0.0.108\",\"html_url\":\"https://github.com/aavidad/GrxFirma/releases/tag/v0.0.109\"}";
    QVERIFY(OfficialUpdateChecker::parse(mismatch, "0.0.105").isEmpty());
  }
  void schedule() {
    const auto now = QDateTime::currentDateTimeUtc();
    QVERIFY(OfficialUpdateChecker::isDue(QDateTime(), now));
    QVERIFY(!OfficialUpdateChecker::isDue(now.addSecs(-5 * 3600 + 1), now));
    QVERIFY(OfficialUpdateChecker::isDue(now.addSecs(-5 * 3600), now));
    QVERIFY(!OfficialUpdateChecker::isDue(now.addSecs(1), now));
  }
};
QTEST_GUILESS_MAIN(OfficialUpdateCheckerTest)
#include "officialupdatechecker_test.moc"
