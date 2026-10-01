// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "webcompatibilitylease.h"

#include <utility>

namespace {
constexpr int kMaximumLeaseMilliseconds = 240 * 60 * 1000;
}

WebCompatibilityLease::WebCompatibilityLease(
    std::function<void()> stopOwnedServer, QObject *parent)
    : QObject(parent), m_stopOwnedServer(std::move(stopOwnedServer)) {
  m_timer.setSingleShot(true);
  m_timer.setTimerType(Qt::PreciseTimer);
  connect(&m_timer, &QTimer::timeout, this, [this]() {
    if (!m_active)
      return;
    m_active = false;
    emit activeChanged();
    if (m_stopOwnedServer)
      m_stopOwnedServer();
    emit expired();
  });
}

bool WebCompatibilityLease::startMilliseconds(int durationMilliseconds) {
  if (durationMilliseconds <= 0 ||
      durationMilliseconds > kMaximumLeaseMilliseconds) {
    cancel();
    return false;
  }
  m_timer.start(durationMilliseconds);
  if (!m_active) {
    m_active = true;
    emit activeChanged();
  }
  return true;
}

void WebCompatibilityLease::cancel() {
  m_timer.stop();
  if (!m_active)
    return;
  m_active = false;
  emit activeChanged();
}
