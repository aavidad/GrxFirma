// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef TRANSIENTSECRET_H
#define TRANSIENTSECRET_H

#include <QByteArray>
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
// sus referencias inmediatamente después del envío.
void zeroize(QString &value);
void zeroize(QByteArray &value);

} // namespace TransientSecret

#endif // TRANSIENTSECRET_H
