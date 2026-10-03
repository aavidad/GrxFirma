// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "residentagent.h"

#include <QAction>
#include <QApplication>
#include <QDateTime>
#include <QFileInfo>
#include <QIcon>
#include <QMenu>
#include <QSet>
#include <QSystemTrayIcon>
#include <QWindow>

ResidentAgent::ResidentAgent(QObject *parent) : QObject(parent) {}

ResidentAgent::~ResidentAgent() { shutdown(); }

void ResidentAgent::initialize(QWindow *window, const QString &iconPath) {
  if (m_shuttingDown || m_tray)
    return;

  m_window = window;
  m_available = QSystemTrayIcon::isSystemTrayAvailable();
  emit availableChanged();
  if (!m_available)
    return;

  m_menu = new QMenu();
  m_openAction = m_menu->addAction(m_openLabel);
  m_statusAction = m_menu->addAction(m_statusLabel);
  m_statusAction->setEnabled(false);
  m_settingsAction = m_menu->addAction(m_settingsLabel);
  m_menu->addSeparator();
  m_helpMenu = m_menu->addMenu(m_helpLabel);
  m_manualAction = m_helpMenu->addAction(m_manualLabel);
  m_releaseNotesAction = m_helpMenu->addAction(m_releaseNotesLabel);
  m_aboutAction = m_helpMenu->addAction(m_aboutLabel);
  m_menu->addSeparator();
  m_quitAction = m_menu->addAction(m_quitLabel);
  m_tray = new QSystemTrayIcon(this);
  m_tray->setContextMenu(m_menu);

  QIcon icon;
  if (!iconPath.isEmpty() && QFileInfo::exists(iconPath))
    icon = QIcon(iconPath);
  if (icon.isNull() && m_window)
    icon = m_window->icon();
  if (icon.isNull())
    icon = QApplication::windowIcon();
  m_tray->setIcon(icon);
  m_tray->setToolTip(QStringLiteral("GrxFirma"));

  connect(m_openAction, &QAction::triggered, this,
          &ResidentAgent::showMainWindow);
  connect(m_settingsAction, &QAction::triggered, this,
          &ResidentAgent::settingsRequested);
  connect(m_manualAction, &QAction::triggered, this,
          &ResidentAgent::helpRequested);
  connect(m_releaseNotesAction, &QAction::triggered, this, [this]() {
    m_releaseNotesNotificationPending = false;
    emit releaseNotesRequested();
  });
  connect(m_aboutAction, &QAction::triggered, this,
          &ResidentAgent::aboutRequested);
  connect(m_quitAction, &QAction::triggered, this,
          &ResidentAgent::exitRequested);
  connect(m_tray, &QSystemTrayIcon::activated, this,
          [this](QSystemTrayIcon::ActivationReason reason) {
            if (reason == QSystemTrayIcon::Trigger ||
                reason == QSystemTrayIcon::DoubleClick)
              showMainWindow();
          });
  connect(m_tray, &QSystemTrayIcon::messageClicked, this, [this]() {
    if (m_updateNotificationPending) {
      m_updateNotificationPending = false;
      showMainWindow();
    } else if (m_releaseNotesNotificationPending) {
      m_releaseNotesNotificationPending = false;
      emit pendingReleaseNotesRequested();
    }
  });
  updateTrayVisibility();
}

QString ResidentAgent::normalizedOperation(const QString &operation) {
  const QString normalized = operation.trimmed().toLower();
  static const QSet<QString> allowed = {
      QStringLiteral("sign"),      QStringLiteral("batch"),
      QStringLiteral("verify"),    QStringLiteral("protect"),
      QStringLiteral("unprotect"), QStringLiteral("hash"),
      QStringLiteral("security"),  QStringLiteral("service"),
  };
  return allowed.contains(normalized) ? normalized : QStringLiteral("other");
}

QString ResidentAgent::boundedLabel(const QString &value,
                                    const QString &fallback) {
  QString label = value.simplified();
  label.remove(QChar('\0'));
  if (label.isEmpty())
    label = fallback;
  return label.left(kMaxLabelLength);
}

void ResidentAgent::configureLabels(const QString &openLabel,
                                    const QString &statusLabel,
                                    const QString &settingsLabel,
                                    const QString &helpLabel,
                                    const QString &manualLabel,
                                    const QString &releaseNotesLabel,
                                    const QString &aboutLabel,
                                    const QString &quitLabel,
                                    const QString &residentMessage,
                                    const QString &successMessage,
                                    const QString &failureMessage) {
  m_openLabel = boundedLabel(openLabel, QStringLiteral("Abrir GrxFirma"));
  m_statusLabel = boundedLabel(statusLabel, QStringLiteral("Firmas desde portales: activo"));
  m_settingsLabel = boundedLabel(settingsLabel, QStringLiteral("Ajustes"));
  m_helpLabel = boundedLabel(helpLabel, QString());
  m_manualLabel = boundedLabel(manualLabel, QString());
  m_releaseNotesLabel = boundedLabel(releaseNotesLabel, QString());
  m_aboutLabel = boundedLabel(aboutLabel, QString());
  m_quitLabel = boundedLabel(quitLabel, QStringLiteral("Salir"));
  m_residentMessage =
      boundedLabel(residentMessage, QStringLiteral("GrxFirma"));
  m_successMessage = boundedLabel(successMessage, QStringLiteral("OK"));
  m_failureMessage = boundedLabel(failureMessage, QStringLiteral("Error"));
  if (m_openAction)
    m_openAction->setText(m_openLabel);
  if (m_statusAction)
    m_statusAction->setText(m_statusLabel);
  if (m_settingsAction)
    m_settingsAction->setText(m_settingsLabel);
  if (m_helpMenu)
    m_helpMenu->setTitle(m_helpLabel);
  if (m_manualAction)
    m_manualAction->setText(m_manualLabel);
  if (m_releaseNotesAction)
    m_releaseNotesAction->setText(m_releaseNotesLabel);
  if (m_aboutAction)
    m_aboutAction->setText(m_aboutLabel);
  if (m_quitAction)
    m_quitAction->setText(m_quitLabel);
}

void ResidentAgent::setEnabled(bool enabled) {
  if (m_shuttingDown || m_enabled == enabled)
    return;
  m_enabled = enabled;
  if (!m_enabled) {
    if (m_hidden)
      showMainWindow();
    clearEvents();
  }
  updateTrayVisibility();
  emit enabledChanged();
}

bool ResidentAgent::hideMainWindow() {
  if (m_shuttingDown || !m_enabled || !m_available || !m_tray || !m_window)
    return false;
  m_window->hide();
  setHidden(true);
  if (!m_firstHideExplained) {
    m_firstHideExplained = true;
    m_releaseNotesNotificationPending = false;
    m_tray->showMessage(QStringLiteral("GrxFirma"), m_residentMessage,
                        QSystemTrayIcon::Information, 4000);
  }
  return true;
}

void ResidentAgent::showMainWindow() {
  if (m_shuttingDown || !m_window)
    return;
  m_window->showNormal();
  m_window->raise();
  m_window->requestActivate();
  setHidden(false);
}

void ResidentAgent::recordOperation(const QString &operation, bool success) {
  if (m_shuttingDown || !m_enabled)
    return;

  // El evento se construye desde un vocabulario cerrado: no recibe mensajes,
  // rutas ni payloads procedentes del backend.
  QVariantMap event;
  event.insert(QStringLiteral("operation"), normalizedOperation(operation));
  event.insert(QStringLiteral("outcome"),
               success ? QStringLiteral("success")
                       : QStringLiteral("failure"));
  event.insert(QStringLiteral("occurredAt"),
               QDateTime::currentDateTimeUtc().toString(Qt::ISODate));
  m_recentEvents.append(event);
  while (m_recentEvents.size() > kMaxRecentEvents)
    m_recentEvents.removeFirst();
  emit recentEventsChanged();

  if (m_hidden && m_tray) {
    m_releaseNotesNotificationPending = false;
    m_tray->showMessage(
        QStringLiteral("GrxFirma"),
        success ? m_successMessage : m_failureMessage,
        success ? QSystemTrayIcon::Information : QSystemTrayIcon::Warning,
        5000);
  }
}

void ResidentAgent::notifyUpdate(const QString &message) {
  if (!m_shuttingDown && m_tray && m_hidden) {
    m_releaseNotesNotificationPending = false;
    m_updateNotificationPending = true;
    m_tray->showMessage(QStringLiteral("GrxFirma"),
                        boundedLabel(message, QString()),
                        QSystemTrayIcon::Information, 6000);
  }
}

void ResidentAgent::notifyReleaseNotes(const QString &message) {
  if (m_shuttingDown || !m_tray || !m_hidden)
    return;
  m_releaseNotesNotificationPending = true;
  m_tray->showMessage(QStringLiteral("GrxFirma"),
                      boundedLabel(message, QString()),
                      QSystemTrayIcon::Information, 10000);
}

void ResidentAgent::clearEvents() {
  if (m_recentEvents.isEmpty())
    return;
  m_recentEvents.clear();
  emit recentEventsChanged();
}

void ResidentAgent::setHidden(bool hidden) {
  if (m_hidden == hidden)
    return;
  m_hidden = hidden;
  emit hiddenChanged();
}

void ResidentAgent::updateTrayVisibility() {
  if (!m_tray)
    return;
  m_tray->show();
}

void ResidentAgent::shutdown() {
  if (m_shuttingDown)
    return;
  m_shuttingDown = true;
  m_enabled = false;
  m_hidden = false;
  m_recentEvents.clear();
  if (m_tray) {
    m_tray->hide();
    m_tray->setContextMenu(nullptr);
  }
  delete m_menu;
  m_menu = nullptr;
  m_openAction = nullptr;
  m_statusAction = nullptr;
  m_settingsAction = nullptr;
  m_helpMenu = nullptr;
  m_manualAction = nullptr;
  m_releaseNotesAction = nullptr;
  m_aboutAction = nullptr;
  m_quitAction = nullptr;
  m_window = nullptr;
}
