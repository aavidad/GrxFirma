// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef WEBCOMPATIBILITYLEASE_H
#define WEBCOMPATIBILITYLEASE_H

#include <QObject>
#include <QTimer>

#include <functional>

// Temporizador efimero para la compatibilidad web local. No persiste el estado
// activo ni permite una duracion ilimitada; el propietario debe detener todos
// los procesos/servidores que controle al recibir expired().
class WebCompatibilityLease final : public QObject {
  Q_OBJECT

public:
  explicit WebCompatibilityLease(std::function<void()> stopOwnedServer,
                                 QObject *parent = nullptr);

  bool active() const { return m_active; }
  bool startMilliseconds(int durationMilliseconds);
  void cancel();

signals:
  void activeChanged();
  void expired();

private:
  QTimer m_timer;
  bool m_active = false;
  std::function<void()> m_stopOwnedServer;
};

#endif // WEBCOMPATIBILITYLEASE_H
