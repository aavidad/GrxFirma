// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef RESIDENTAGENT_H
#define RESIDENTAGENT_H

#include <QObject>
#include <QString>
#include <QVariantList>

class QAction;
class QMenu;
class QSystemTrayIcon;
class QWindow;

// ResidentAgent mantiene exclusivamente estado operativo no sensible para el
// modo residente. Los documentos, rutas, credenciales y payloads de las
// operaciones pertenecen a workers efimeros y nunca se copian aqui.
class ResidentAgent final : public QObject {
  Q_OBJECT
  Q_PROPERTY(bool available READ available NOTIFY availableChanged)
  Q_PROPERTY(bool enabled READ enabled NOTIFY enabledChanged)
  Q_PROPERTY(bool hidden READ hidden NOTIFY hiddenChanged)
  Q_PROPERTY(QVariantList recentEvents READ recentEvents NOTIFY
                 recentEventsChanged)

public:
  explicit ResidentAgent(QObject *parent = nullptr);
  ~ResidentAgent() override;

  bool available() const { return m_available; }
  bool enabled() const { return m_enabled; }
  bool hidden() const { return m_hidden; }
  QVariantList recentEvents() const { return m_recentEvents; }

  void initialize(QWindow *window, const QString &iconPath = QString());
  void recordOperation(const QString &operation, bool success);
  void notifyUpdate(const QString &message);
  Q_INVOKABLE void notifyReleaseNotes(const QString &message);
  void shutdown();

  Q_INVOKABLE void setEnabled(bool enabled);
  Q_INVOKABLE bool hideMainWindow();
  Q_INVOKABLE void showMainWindow();
  Q_INVOKABLE void clearEvents();
  Q_INVOKABLE void configureLabels(const QString &openLabel,
                                   const QString &statusLabel,
                                   const QString &settingsLabel,
                                   const QString &helpLabel,
                                   const QString &manualLabel,
                                   const QString &releaseNotesLabel,
                                   const QString &aboutLabel,
                                   const QString &quitLabel,
                                   const QString &residentMessage,
                                   const QString &successMessage,
                                   const QString &failureMessage);

signals:
  void availableChanged();
  void enabledChanged();
  void hiddenChanged();
  void recentEventsChanged();
  void exitRequested();
  void settingsRequested();
  void helpRequested();
  void aboutRequested();
  void releaseNotesRequested();
  void pendingReleaseNotesRequested();

private:
  static QString normalizedOperation(const QString &operation);
  static QString boundedLabel(const QString &value,
                              const QString &fallback);
  void setHidden(bool hidden);
  void updateTrayVisibility();

  static constexpr int kMaxRecentEvents = 24;
  static constexpr int kMaxLabelLength = 96;

  QWindow *m_window = nullptr;
  QSystemTrayIcon *m_tray = nullptr;
  QMenu *m_menu = nullptr;
  QAction *m_openAction = nullptr;
  QAction *m_statusAction = nullptr;
  QAction *m_settingsAction = nullptr;
  QMenu *m_helpMenu = nullptr;
  QAction *m_manualAction = nullptr;
  QAction *m_releaseNotesAction = nullptr;
  QAction *m_aboutAction = nullptr;
  QAction *m_quitAction = nullptr;
  bool m_releaseNotesNotificationPending = false;
  bool m_available = false;
  bool m_enabled = false;
  bool m_hidden = false;
  bool m_shuttingDown = false;
  bool m_firstHideExplained = false;
  QVariantList m_recentEvents;
  QString m_openLabel = QStringLiteral("Abrir GrxFirma");
  QString m_statusLabel = QStringLiteral("Firmas desde portales: activo");
  QString m_settingsLabel = QStringLiteral("Ajustes");
  QString m_helpLabel;
  QString m_manualLabel;
  QString m_releaseNotesLabel;
  QString m_aboutLabel;
  QString m_quitLabel = QStringLiteral("Salir");
  QString m_residentMessage = QStringLiteral("GrxFirma");
  QString m_successMessage = QStringLiteral("OK");
  QString m_failureMessage = QStringLiteral("Error");
};

#endif // RESIDENTAGENT_H
