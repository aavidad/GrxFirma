// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef TRANSIENTSECRET_H
#define TRANSIENTSECRET_H

#include <QByteArray>
#include <QJsonObject>
#include <QList>
#include <QPair>
#include <QString>

namespace TransientSecret {

inline constexpr qsizetype AES256KeyBytes = 32;
inline constexpr qsizetype AES256Base64Length = 44;

// Valida el contrato público de EncryptedData sin conservar la clave
// decodificada. La entrada debe ser Base64 canónico de exactamente 32 bytes.
bool isCanonicalAES256Base64(const QString &value);

// Los ficheros creados con la clave transitoria deben conservar un sufijo
// inequívoco para que la UI vuelva a exigirla al abrirlos.
QString normalizedEncryptedDataOutputPath(const QString &value);

// Limpieza best-effort de las copias controladas por el bridge. Qt/QML puede
// haber creado otras copias implícitas; por eso la UI también elimina todas
// sus referencias inmediatamente después del envío. Ojo: QString y QByteArray
// comparten datos entre copias; si el búfer está compartido, fill() separa
// antes una copia propia y solo se borra esa, no la original.
void zeroize(QString &value);
void zeroize(QByteArray &value);

// Secreto que va en una trama IPC: nombre del campo y bytes en claro. La
// trama lo lleva en Base64 (p12B64, credentialB64, passwordB64).
using SecretField = QPair<QString, QByteArray *>;

// Construye la línea JSON de una petición IPC con secretos sin pasarlos por
// QString, QVariantMap ni QJsonObject. Los campos normales van en `header`
// (requestId, traceId, action) y `plainParams`; cada secreto se codifica en
// Base64 directamente en el QByteArray devuelto. Al terminar, los bytes de
// cada secreto de entrada quedan a cero (mismo tamaño) y los Base64
// intermedios se borran. Quien llama pone a cero la trama tras enviarla.
// No alcanza a copias que el puente no controla: la QString de la contraseña
// que llega desde QML y el búfer interno de escritura de QLocalSocket.
QByteArray buildSecretRequestFrame(const QJsonObject &header,
                                   const QJsonObject &plainParams,
                                   const QList<SecretField> &secrets);

} // namespace TransientSecret

#endif // TRANSIENTSECRET_H
