// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef INCIDENTPRIVACY_H
#define INCIDENTPRIVACY_H

#include <QDateTime>
#include <QFile>
#include <QFileDevice>
#include <QString>
#include <QVariantMap>

// Sanea de forma defensiva un informe antes de persistirlo o enviarlo. Los
// nombres de fichero pueden conservarse, pero nunca las rutas completas,
// material criptográfico, credenciales ni identificadores personales.
QVariantMap IncidentSanitizePayload(const QVariantMap &payload,
                                    bool remoteSend);
QString IncidentSanitizeText(const QString &text);
QString IncidentReadSanitizedTextTail(const QString &path,
                                      qint64 maxBytes = 65536);

// Frontera final antes de escribir una línea en gui-qml.log. Redacta datos
// estructurados, rutas, red, identidad, material criptográfico e
// identificadores de correlación brutos.
QString IncidentSanitizePersistentLogMessage(const QString &message);

// Proyección cerrada para peticiones/respuestas IPC. Nunca serializa params,
// data ni errores: conserva solo acción, categoría, resultado, tamaños y una
// referencia opaca efímera cuando el mensaje ya traía correlación.
QString IncidentFormatIpcLogEvent(const QVariantMap &message,
                                  const QString &category,
                                  const QString &result,
                                  qint64 wireBytes = -1);

// El borde C++ vuelve a exigir el consentimiento aunque la QML solo habilite
// visualmente el botón tras marcar la casilla.
bool IncidentHasExplicitRemoteConsent(const QVariantMap &payload);

// El envío remoto solo puede partir de una incidencia de fallo ya persistida
// por la aplicación. La ruta debe señalar a un JSON regular, no simbólico y
// contenido directamente en el directorio privado indicado.
bool IncidentLoadEligibleSavedReport(const QString &path,
                                     const QString &reportsDirectory,
                                     QVariantMap *payload,
                                     QString *errorMessage = nullptr);

// Registra el último intento con el mínimo dato de auditoría necesario. La
// actualización vuelve a sanear el informe y usa la escritura atómica común;
// no persiste endpoint, IP, respuesta remota ni texto de error.
bool IncidentRecordRemoteAttempt(const QString &path,
                                 const QString &reportsDirectory,
                                 bool succeeded,
                                 const QDateTime &attemptedAtUtc,
                                 QString *errorMessage = nullptr);

// Las incidencias locales se guardan con permisos exclusivos del usuario en
// POSIX. También se rechazan directorios simbólicos para evitar redirecciones
// del destino de escritura.
bool IncidentEnsurePrivateDirectory(const QString &path,
                                    QString *errorMessage = nullptr);
bool IncidentSetPrivateFilePermissions(QFileDevice *file,
                                       QString *errorMessage = nullptr);
bool IncidentWritePrivateFile(const QString &path, const QByteArray &data,
                              QString *errorMessage = nullptr);

// Aplica la retención local de incidencias. Solo reconoce y elimina los
// nombres generados por GrxFirma, nunca sigue enlaces simbólicos y deja
// intactos los ficheros ajenos. La política de producto usa 30 días y un
// máximo de 50 grupos de incidencia por superficie.
bool IncidentApplyReportRetention(const QString &reportsDirectory,
                                  const QDateTime &nowUtc,
                                  int maxAgeDays = 30,
                                  int maxIncidentGroups = 50,
                                  int *removedFiles = nullptr,
                                  QString *errorMessage = nullptr);

// El log Qt conserva como máximo el fichero activo y una rotación. Esta
// función retira ambos cuando superan la edad configurada y rechaza destinos
// simbólicos o no regulares antes de que el logger los abra.
bool IncidentApplyPrivateLogRetention(const QString &path,
                                      const QDateTime &nowUtc,
                                      int maxAgeDays = 30,
                                      int *removedFiles = nullptr,
                                      QString *errorMessage = nullptr);

// Apertura segura en append para el log persistente. Crea/restringe el
// directorio a 0700 y el fichero a 0600 en POSIX, y rechaza enlaces
// simbólicos o destinos no regulares.
bool IncidentOpenPrivateAppendFile(QFile *file,
                                   QString *errorMessage = nullptr);

#endif // INCIDENTPRIVACY_H
