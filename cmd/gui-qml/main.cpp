// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "backendbridge.h"
#include "incidentprivacy.h"
#include "ipcbridge.h"
#include "officialupdatechecker.h"
#include "processarguments.h"
#include "processenvironment.h"
#include "portalsealbridge.h"
#include "residentagent.h"
#include "savedialognames.h"
#include "releasenotes.h"
#include "qttranslations.h"
#include "translatorbridge.h"
#include <QApplication>
#include <QCoreApplication>
#include <QDateTime>
#include <QDebug>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QFileSystemWatcher>
#include <QIcon>
#include <QLoggingCategory>
#include <QLocalSocket>
#include <QMutex>
#include <QMutexLocker>
#include <QMessageBox>
#include <QObject>
#include <QProcess>
#include <QRandomGenerator>
#include <QRegularExpression>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QStandardPaths>
#include <QString>
#include <QTcpSocket>
#include <QTextStream>
#include <QTimer>
#include <QUrl>
#include <QVariantList>
#include <QVariantMap>
#include <QWindow>
#include <cstdio>

static void ensureSessionBusAddress() {
  if (!qEnvironmentVariableIsEmpty("DBUS_SESSION_BUS_ADDRESS"))
    return;
  QString runtimeDir = qEnvironmentVariable("XDG_RUNTIME_DIR");
  if (runtimeDir.isEmpty())
    runtimeDir = QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
  if (runtimeDir.isEmpty())
    return;
  const QString busPath = QDir(runtimeDir).absoluteFilePath("bus");
  if (!QFileInfo::exists(busPath))
    return;
  qputenv("DBUS_SESSION_BUS_ADDRESS",
          QByteArray("unix:path=") + busPath.toUtf8());
}

static QString resolveGuiAssetsDir(const QString &binDir) {
  const QStringList candidates = {
      binDir + "/../Resources/assets",
      binDir + "/../lib/grxfirma/gui-qml/assets",
      binDir + "/../lib64/grxfirma/gui-qml/assets",
      binDir + "/assets",
      QDir::currentPath() + "/cmd/gui-qml/assets",
      binDir + "/cmd/gui-qml/assets",
  };
  for (const auto &dir : candidates) {
    QFileInfo info(dir);
    if (info.exists() && info.isDir())
      return QDir(dir).absolutePath();
  }
  return QString();
}

static QMutex s_guiLogMutex;
static QString s_guiLogPath;

static QString guiDebugLogPath() {
  QString stateDir =
      QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation);
  if (stateDir.isEmpty()) {
    QString home = QDir::homePath();
    stateDir = home.isEmpty() ? QDir::tempPath() : home + "/.local/state/grxfirma";
  }
  return QDir(stateDir).filePath("logs/gui-qml.log");
}

static QString resolveGuiVersion(const QString &binDir) {
  const QStringList candidates = {
      QDir(binDir).filePath("../lib/grxfirma/gui-qml/VERSION.txt"),
      QDir(binDir).filePath("VERSION.txt"),
      QDir(binDir).filePath("../Resources/VERSION.txt"),
      QDir(binDir).filePath("../VERSION.txt"),
      QDir(binDir).filePath("../lib/grxfirma/gui-qml/VERSION.txt"),
      QDir::current().filePath("VERSION.txt"),
  };
  for (const QString &candidate : candidates) {
    QFile file(candidate);
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
      continue;
    const QString version = QString::fromUtf8(file.readAll()).trimmed();
    if (!version.isEmpty())
      return version;
  }
  return QStringLiteral("dev");
}

#ifdef Q_OS_LINUX
static QString installedGuiVersion(const QString &versionPath) {
  const QFileInfo info(versionPath);
  if (!info.isFile() || info.isSymLink() || info.size() < 1 || info.size() > 64)
    return QString();
  QFile file(versionPath);
  if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
    return QString();
  const QString version = QString::fromUtf8(file.readAll()).trimmed();
  static const QRegularExpression allowed(QStringLiteral("^[0-9][0-9A-Za-z.+~-]{0,63}$"));
  return allowed.match(version).hasMatch() ? version : QString();
}
#endif

static QString resolveReleaseNotes(const QString &binDir) {
  const QStringList candidates = {
      QDir(binDir).filePath("help/NOVEDADES.md"),
      QDir(binDir).filePath("../lib/grxfirma/gui-qml/help/NOVEDADES.md"),
      QDir(binDir).filePath("../../docs/NOVEDADES.md"),
  };
  for (const QString &candidate : candidates) {
    const QFileInfo info(candidate);
    if (!info.isFile() || info.isSymLink() || info.size() <= 0 || info.size() > 64 * 1024)
      continue;
    QFile file(candidate);
    if (file.open(QIODevice::ReadOnly | QIODevice::Text))
      return QString::fromUtf8(file.readAll());
  }
  return QString();
}

static bool rotateGuiDebugLogIfNeeded(const QString &path) {
  QFileInfo info(path);
  constexpr qint64 maxSize = 2 * 1024 * 1024;
  if (!info.exists() || info.size() < maxSize)
    return !info.isSymLink() && (!info.exists() || info.isFile());
  if (info.isSymLink() || !info.isFile())
    return false;
  const QString backup = path + ".1";
  const QFileInfo backupInfo(backup);
  if (backupInfo.isSymLink() ||
      (backupInfo.exists() && !backupInfo.isFile())) {
    return false;
  }
  if (backupInfo.exists() && !QFile::remove(backup))
    return false;
  if (!QFile::rename(path, backup))
    return false;
#ifdef Q_OS_UNIX
  if (!QFile::setPermissions(
          backup, QFileDevice::ReadOwner | QFileDevice::WriteOwner)) {
    return false;
  }
#endif
  return true;
}

static QString logLevelName(QtMsgType type) {
  switch (type) {
  case QtDebugMsg:
    return QStringLiteral("DEBUG");
  case QtInfoMsg:
    return QStringLiteral("INFO");
  case QtWarningMsg:
    return QStringLiteral("WARN");
  case QtCriticalMsg:
    return QStringLiteral("ERROR");
  case QtFatalMsg:
    return QStringLiteral("FATAL");
  }
  return QStringLiteral("LOG");
}

static void guiDebugMessageHandler(QtMsgType type, const QMessageLogContext &ctx,
                                   const QString &msg) {
  QMutexLocker locker(&s_guiLogMutex);
  if (!s_guiLogPath.isEmpty() &&
      rotateGuiDebugLogIfNeeded(s_guiLogPath)) {
    QFile file(s_guiLogPath);
    QString openError;
    if (IncidentOpenPrivateAppendFile(&file, &openError)) {
      const QString safeMessage =
          IncidentSanitizePersistentLogMessage(msg);
      QTextStream ts(&file);
      ts.setEncoding(QStringConverter::Utf8);
      ts << QDateTime::currentDateTime().toString(Qt::ISODateWithMs) << " ["
         << logLevelName(type) << "]";
      if (ctx.category && *ctx.category) {
        ts << " ["
           << IncidentSanitizePersistentLogMessage(
                  QString::fromUtf8(ctx.category))
                  .left(128)
           << "]";
      }
      if (ctx.file && *ctx.file) {
        ts << " ["
           << IncidentSanitizePersistentLogMessage(
                  QFileInfo(QString::fromUtf8(ctx.file)).fileName())
                  .left(128)
           << ":" << ctx.line << "]";
      }
      ts << " " << safeMessage << "\n";
      ts.flush();
    }
  }
  if (type == QtFatalMsg)
    abort();
}

template <typename Bridge>
static void connectResidentEvents(Bridge *bridge, ResidentAgent *agent) {
  QObject::connect(
      bridge, &Bridge::signingFinished, agent,
      [agent](bool ok, const QString &, const QString &) {
        agent->recordOperation(QStringLiteral("sign"), ok);
      });
  QObject::connect(
      bridge, &Bridge::batchSigningFinished, agent,
      [agent](bool ok, const QString &, const QVariantList &) {
        agent->recordOperation(QStringLiteral("batch"), ok);
      });
  QObject::connect(
      bridge, &Bridge::verificationFinished, agent,
      [agent](bool ok, const QString &, const QVariantMap &) {
        agent->recordOperation(QStringLiteral("verify"), ok);
      });
  QObject::connect(
      bridge, &Bridge::protectionFinished, agent,
      [agent](bool ok, const QString &, const QVariantMap &) {
        agent->recordOperation(QStringLiteral("protect"), ok);
      });
  QObject::connect(
      bridge, &Bridge::unprotectionFinished, agent,
      [agent](bool ok, const QString &, const QVariantMap &) {
        agent->recordOperation(QStringLiteral("unprotect"), ok);
      });
  QObject::connect(
      bridge, &Bridge::hashCreateFinished, agent,
      [agent](bool ok, const QString &, const QVariantMap &) {
        agent->recordOperation(QStringLiteral("hash"), ok);
      });
  QObject::connect(
      bridge, &Bridge::hashCheckFinished, agent,
      [agent](bool ok, const QString &, const QVariantMap &) {
        agent->recordOperation(QStringLiteral("hash"), ok);
      });
}

int main(int argc, char *argv[]) {
  if (!ChildProcessEnvironment::scrubCurrentProcess()) {
    std::fputs("Error: no se pudo sanear el entorno del proceso Qt.\n",
               stderr);
    return 1;
  }
  QStringList startupArguments;
  startupArguments.reserve(argc > 1 ? argc - 1 : 0);
  for (int i = 1; i < argc; ++i)
    startupArguments.append(QString::fromLocal8Bit(argv[i]));
  if (ProcessArguments::containsSensitiveOptionName(startupArguments)) {
    std::fputs("Error: opcion sensible no permitida en el proceso Qt.\n",
               stderr);
    return 1;
  }

  // En V2 el frontend Qt/QML debe funcionar por IPC local por defecto.
  // El modo REST queda solo para uso explícito.
  bool useIpc = true;
  QString forcedIpcPath;
  QString portalSealRequest;
  for (int i = 1; i < argc; ++i) {
    QString arg = QString::fromLocal8Bit(argv[i]);
    if (arg == "--rest" || arg == "-rest") {
      useIpc = false;
    } else if ((arg == "--ipc-socket" || arg == "-ipc-socket") &&
               i + 1 < argc) {
      forcedIpcPath = QString::fromLocal8Bit(argv[++i]);
      useIpc = true;
    } else if (arg == "--ipc" || arg == "-ipc") {
      useIpc = true;
    } else if (arg == "--portal-seal-request" && i + 1 < argc) {
      portalSealRequest = QString::fromLocal8Bit(argv[++i]);
    }
  }
  ensureSessionBusAddress();
  QApplication app(argc, argv);
  PortalSealBridge portalSeal;
  if (!portalSealRequest.isEmpty() && !portalSeal.load(portalSealRequest))
    return 1;
  if (portalSeal.active())
    forcedIpcPath = QFileInfo(portalSealRequest).absolutePath() +
                    QStringLiteral("/preview.sock");
  // Qt.labs.settings uses this identifier to find existing user preferences.
  app.setApplicationName("GrxFirma");
  app.setOrganizationName("Diputacion de Granada");
  // Wayland asocia la ventana al lanzador por este identificador; sin él, el
  // panel muestra un icono genérico en lugar del de GrxFirma.
  QGuiApplication::setDesktopFileName(QStringLiteral("grxfirma-manual"));
#ifdef Q_OS_WIN
  const QString applicationIconPath =
      QStringLiteral(":/assets/grxfirma.ico");
#else
  const QString applicationIconPath =
      QStringLiteral(":/assets/grxfirma-icono-256.png");
#endif
  const QIcon applicationIcon(applicationIconPath);
  if (!applicationIcon.isNull())
    app.setWindowIcon(applicationIcon);
  const QString binDir = QCoreApplication::applicationDirPath();
  app.setApplicationVersion(resolveGuiVersion(binDir));
  s_guiLogPath = guiDebugLogPath();
  QFile initialLog(s_guiLogPath);
  QString logOpenError;
  bool logStorageReady = IncidentApplyPrivateLogRetention(
      s_guiLogPath, QDateTime::currentDateTimeUtc(), 30, nullptr,
      &logOpenError);
  if (logStorageReady)
    logStorageReady =
        IncidentOpenPrivateAppendFile(&initialLog, &logOpenError);
  if (logStorageReady) {
    initialLog.close();
    const QFileInfo backup(s_guiLogPath + QStringLiteral(".1"));
    if (backup.isSymLink() ||
        (backup.exists() && !backup.isFile())) {
      logStorageReady = false;
    }
#ifdef Q_OS_UNIX
    if (logStorageReady && backup.exists()) {
      logStorageReady = QFile::setPermissions(
          backup.absoluteFilePath(),
          QFileDevice::ReadOwner | QFileDevice::WriteOwner);
    }
#endif
  }
  if (!logStorageReady) {
    initialLog.close();
    s_guiLogPath.clear();
    std::fputs(
        "Aviso: no se pudo preparar el log privado de Qt/QML.\n", stderr);
  }
  qInstallMessageHandler(guiDebugMessageHandler);
  QLoggingCategory::setFilterRules(QStringLiteral(
      "qt.scenegraph.*.debug=false\n"
      "qt.quick.*.debug=false\n"
      "qt.pointer.*.debug=false\n"
      "qt.qml.binding.removal.info=false\n"));
  if (!s_guiLogPath.isEmpty())
    qInfo().noquote() << "[debug] Log persistente privado activado.";

  QString ipcPath;
#if defined(Q_OS_WIN)
  QString nonce;
  nonce.reserve(32);
  auto *secureRandom = QRandomGenerator::system();
  for (int i = 0; i < 4; ++i) {
    nonce.append(QStringLiteral("%1").arg(secureRandom->generate(), 8, 16,
                                          QLatin1Char('0')));
  }
  ipcPath = QStringLiteral("\\\\.\\pipe\\grxfirma_ipc_%1_%2")
                .arg(QCoreApplication::applicationPid())
                .arg(nonce);
#else
  QString userName = QDir::home().dirName();
  if (userName.isEmpty())
    userName = "default";
  // Usar /run/user/UID (XDG_RUNTIME_DIR) para mayor seguridad si está
  // disponible
  QString runtimeDir =
      QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
  if (runtimeDir.isEmpty())
    runtimeDir = QDir::tempPath();
  ipcPath =
      QDir(runtimeDir).absoluteFilePath("grxfirma_ipc_" + userName + ".sock");
#endif
  if (!forcedIpcPath.isEmpty())
    ipcPath = forcedIpcPath;
  qDebug() << "[Main] Ruta IPC privada preparada.";

  BackendBridge restBridge;
  IpcBridge ipcBridge;
  ResidentAgent residentAgent;
  connectResidentEvents(&restBridge, &residentAgent);
  connectResidentEvents(&ipcBridge, &residentAgent);
  QObject::connect(&restBridge, &BackendBridge::backendLogReceived, &app,
                   [](const QString &msg) {
                     qInfo().noquote()
                         << "[rest]"
                         << IncidentSanitizePersistentLogMessage(msg);
                   });
  QObject::connect(&ipcBridge, &IpcBridge::backendLogReceived, &app,
                   [](const QString &msg) {
                     qInfo().noquote()
                         << "[ipc]"
                         << IncidentSanitizePersistentLogMessage(msg);
                   });
  QObject *activeBridge = useIpc ? static_cast<QObject *>(&ipcBridge)
                                 : static_cast<QObject *>(&restBridge);

  // Check expert mode arg
  for (int i = 1; i < argc; ++i) {
    QString arg = QString::fromLocal8Bit(argv[i]).toLower();
    if (arg == "--experto" || arg == "-experto") {
      restBridge.setExpertMode(true);
      ipcBridge.setExpertMode(true);
    }
  }

  QQmlApplicationEngine engine;
  TranslatorBridge translator;
  // Botones estándar y selector de ficheros de Qt en el idioma de la
  // aplicación; se cambian con él.
  QtTranslations qtTranslations(
      binDir, [&translator](const QString &key) { return translator.t(key); });
  qtTranslations.apply(translator.locale());
  QObject::connect(&translator, &TranslatorBridge::localeChanged, &translator,
                   [&qtTranslations, &translator, &engine]() {
                     qtTranslations.apply(translator.locale());
                     engine.retranslate();
                   });
  OfficialUpdateChecker officialUpdateChecker;
  ReleaseNotesBridge releaseNotes(resolveReleaseNotes(binDir));
  const QString guiAssetsDir = resolveGuiAssetsDir(binDir);
  const QString bundledSealLogoPath =
      guiAssetsDir.isEmpty()
          ? QString()
          : QUrl::fromLocalFile(
                QDir(guiAssetsDir).absoluteFilePath("logo_firma_grxfirma_final.png"))
                .toString();
  engine.rootContext()->setContextProperty("backend", activeBridge);
  engine.rootContext()->setContextProperty("portalSeal", &portalSeal);
  engine.rootContext()->setContextProperty("isIpcMode", useIpc);
  engine.rootContext()->setContextProperty("ipcSocketPath", ipcPath);
  engine.rootContext()->setContextProperty("i18n", &translator);
  engine.rootContext()->setContextProperty("officialUpdateChecker", &officialUpdateChecker);
  engine.rootContext()->setContextProperty("residentAgent", &residentAgent);
  engine.rootContext()->setContextProperty("appVersion",
                                           QCoreApplication::applicationVersion());
  engine.rootContext()->setContextProperty("guiAssetsDir", guiAssetsDir);
  SaveDialogNames saveDialogNames;
  engine.rootContext()->setContextProperty("saveDialogNames", &saveDialogNames);
  // Carpeta que se propone al guardar cuando no hay un documento de referencia.
  QString documentsFolder =
      QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation);
  if (documentsFolder.isEmpty() || !QDir(documentsFolder).exists())
    documentsFolder = QDir::homePath();
  engine.rootContext()->setContextProperty("documentsFolderPath",
                                           QDir::fromNativeSeparators(documentsFolder));
  const QString releaseNotesSource = releaseNotes.since(app.applicationVersion(), QString());
  engine.rootContext()->setContextProperty(
      "releaseNotesText", releaseNotesSource);
  engine.rootContext()->setContextProperty("releaseNotes", &releaseNotes);
  engine.rootContext()->setContextProperty("bundledSealLogoResolvedPath",
                                           bundledSealLogoPath);

  // Resolver ruta QML
  QStringList candidates = {
      binDir + "/../Resources/qml/main.qml",
      binDir + "/qml/main.qml",
      binDir + "/../lib/grxfirma/gui-qml/qml/main.qml",
      binDir + "/../lib64/grxfirma/gui-qml/qml/main.qml",
      binDir + "/../qml/main.qml",
      QDir::currentPath() + "/cmd/gui-qml/qml/main.qml",
      binDir + "/cmd/gui-qml/qml/main.qml",
  };
  QString qmlPath;
  for (const auto &c : candidates) {
    if (QFileInfo::exists(c)) {
      qmlPath = c;
      break;
    }
  }
  if (qmlPath.isEmpty() && QFile::exists(":/qml/main.qml")) {
    qmlPath = QStringLiteral("qrc:/qml/main.qml");
  }
  if (qmlPath.isEmpty()) {
    qWarning("No se encontro main.qml en ninguna ubicacion conocida");
    return -1;
  }
  qDebug() << (useIpc ? "[modo IPC]" : "[modo REST]") << "QML:" << qmlPath;

  if (qmlPath.startsWith("qrc:/"))
    engine.load(QUrl(qmlPath));
  else
    engine.load(QUrl::fromLocalFile(qmlPath));
  if (engine.rootObjects().isEmpty())
    return -1;

  if (!portalSeal.active())
    residentAgent.initialize(
        qobject_cast<QWindow *>(engine.rootObjects().constFirst()),
        applicationIconPath);
  if (startupArguments.contains(QStringLiteral("--start-hidden"))) {
    residentAgent.setEnabled(true);
    residentAgent.hideMainWindow();
  }

  QObject::connect(&app, &QCoreApplication::aboutToQuit, &residentAgent,
                   &ResidentAgent::shutdown);
  QObject::connect(&app, &QCoreApplication::aboutToQuit, [&]() {
    if (useIpc)
      ipcBridge.shutdownForExit();
    else
      restBridge.shutdownForExit();
  });

  // Arrancar backend automaticamente
  if (useIpc) {
    ipcBridge.startBackend(ipcPath);
  } else {
    restBridge.startBackend("127.0.0.1:63118", "", "rest", "", true);
  }

#ifdef Q_OS_LINUX
  const QString installedFrontend = QStringLiteral("/usr/bin/grxfirma-gui-qml");
  const QString installedLauncher = QStringLiteral("/usr/bin/grxfirma-gui");
  const QString installedVersionPath =
      QStringLiteral("/usr/lib/grxfirma/gui-qml/VERSION.txt");
  const bool installedSession =
      QFileInfo(QCoreApplication::applicationFilePath()).canonicalFilePath() ==
          installedFrontend &&
      QFileInfo(installedLauncher).canonicalFilePath() == installedLauncher;
  if (installedSession) {
    QFileSystemWatcher versionWatcher;
    const QString versionDirectory = QFileInfo(installedVersionPath).absolutePath();
    if (QFileInfo(versionDirectory).isDir())
      versionWatcher.addPath(versionDirectory);
    QTimer versionTimer;
    versionTimer.setInterval(3000);
    QString dismissedVersion;
    bool promptOpen = false;
    bool restartPending = false;
    auto checkInstalledVersion = [&]() {
      if (!versionWatcher.directories().contains(versionDirectory) &&
          QFileInfo(versionDirectory).isDir())
        versionWatcher.addPath(versionDirectory);
      const QString version = installedGuiVersion(installedVersionPath);
      if (version.isEmpty() || version == app.applicationVersion() ||
          version == dismissedVersion || promptOpen || restartPending)
        return;
      QObject *root = engine.rootObjects().constFirst();
      if (root->property("restartBlocked").toBool())
        return;
      auto restart = [root, &app, &residentAgent, installedLauncher,
                      installedFrontend, &restartPending]() {
        restartPending = false;
        if (root->property("restartBlocked").toBool())
          return;
        QStringList args{QStringLiteral("--frontend=qt"),
                         QStringLiteral("--ui-binary=") + installedFrontend};
        if (residentAgent.hidden())
          args.append(QStringLiteral("--start-hidden"));
        if (QFileInfo(installedLauncher).canonicalFilePath() != installedLauncher ||
            QFileInfo(installedFrontend).canonicalFilePath() != installedFrontend)
          return;
        if (!QProcess::startDetached(installedLauncher, args))
          return;
        app.quit();
      };
      if (residentAgent.hidden()) {
        restartPending = true;
        residentAgent.notifyUpdate(
            translator.t(QStringLiteral("GrxFirma se ha actualizado a la versión %1 y se reinicia"))
                .arg(version));
        QTimer::singleShot(1200, &app, restart);
      } else {
        promptOpen = true;
        auto *prompt = new QMessageBox(QMessageBox::Information,
            QStringLiteral("GrxFirma"),
            translator.t(QStringLiteral("GrxFirma se ha actualizado a la versión %1. ¿Reiniciar ahora?"))
                .arg(version), QMessageBox::NoButton);
        prompt->setAttribute(Qt::WA_DeleteOnClose);
        prompt->addButton(
            translator.t(QStringLiteral("Reiniciar ahora")), QMessageBox::AcceptRole);
        prompt->addButton(translator.t(QStringLiteral("Más tarde")),
                          QMessageBox::RejectRole);
        QObject::connect(prompt, &QMessageBox::finished, &app,
                         [&, prompt, version, restart](int) {
                           promptOpen = false;
                           if (prompt->buttonRole(prompt->clickedButton()) == QMessageBox::AcceptRole)
                             restart();
                           else
                             dismissedVersion = version;
                         });
        prompt->open();
      }
    };
    QObject::connect(&versionWatcher, &QFileSystemWatcher::directoryChanged,
                     &app, checkInstalledVersion);
    QObject::connect(&versionTimer, &QTimer::timeout, &app,
                     checkInstalledVersion);
    versionTimer.start();
    return app.exec();
  }
#endif

  return app.exec();
}
