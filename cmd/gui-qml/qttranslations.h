// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef QTTRANSLATIONS_H
#define QTTRANSLATIONS_H

#include <QCoreApplication>
#include <QDir>
#include <QLibraryInfo>
#include <QLocale>
#include <QString>
#include <QStringList>
#include <QRegularExpression>
#include <QTranslator>
#include <functional>
#include <memory>
#include <vector>

// Completa los textos de Qt que sus propias traducciones no traen (en
// castellano faltan, por ejemplo, «File name», «Filter» o «Downloads» del
// selector de ficheros) con el catálogo de la aplicación. La clave sale del
// contexto y del texto original de Qt: «qt.filedialog.file_name». Si el
// catálogo no la tiene, decide la traducción de Qt.
class CatalogQtTranslator : public QTranslator {
public:
  using Lookup = std::function<QString(const QString &)>;

  explicit CatalogQtTranslator(Lookup lookup) : m_lookup(std::move(lookup)) {}

  static QString catalogKeyFor(const QString &context, const QString &source) {
    static const QRegularExpression separators(QStringLiteral("[^a-z0-9]+"));
    QString text = source.toLower();
    text.replace(separators, QStringLiteral("_"));
    while (text.startsWith(QLatin1Char('_')))
      text.remove(0, 1);
    while (text.endsWith(QLatin1Char('_')))
      text.chop(1);
    if (context.isEmpty() || text.isEmpty())
      return QString();
    return QStringLiteral("qt.") + context.toLower() + QLatin1Char('.') + text;
  }

  QString translate(const char *context, const char *sourceText,
                    const char *disambiguation = nullptr,
                    int n = -1) const override {
    Q_UNUSED(disambiguation);
    Q_UNUSED(n);
    if (!m_lookup || !context || !sourceText)
      return QString();
    const QString key = catalogKeyFor(QString::fromUtf8(context),
                                      QString::fromUtf8(sourceText));
    if (key.isEmpty())
      return QString();
    const QString value = m_lookup(key);
    return value == key ? QString() : value;
  }

  bool isEmpty() const override { return false; }

private:
  Lookup m_lookup;
};

// Textos propios de Qt (botones estándar, selector de ficheros, nombres de
// las carpetas del sistema) en el idioma de la aplicación y no en el del
// sistema. Usa los catálogos «qt_<idioma>.qm» de Qt; si no están instalados,
// Qt sigue con sus textos originales.
class QtTranslations {
public:
  // Catálogos de Qt que se cargan para un idioma de la aplicación, del más
  // preferido al menos. Gallego, euskera y valenciano completan con el
  // castellano (o el catalán) lo que Qt no tenga traducido.
  static QStringList qtLocalesFor(const QString &appLocale) {
    const QString code = appLocale.trimmed().toLower();
    if (code == QStringLiteral("en"))
      return {QStringLiteral("en")};
    if (code == QStringLiteral("ca"))
      return {QStringLiteral("ca"), QStringLiteral("es")};
    if (code == QStringLiteral("va"))
      return {QStringLiteral("ca"), QStringLiteral("es")};
    if (code == QStringLiteral("gl"))
      return {QStringLiteral("gl"), QStringLiteral("es")};
    if (code == QStringLiteral("eu"))
      return {QStringLiteral("es")};
    if (code == QStringLiteral("fr"))
      return {QStringLiteral("fr")};
    if (code == QStringLiteral("de"))
      return {QStringLiteral("de")};
    if (code == QStringLiteral("it"))
      return {QStringLiteral("it")};
    if (code == QStringLiteral("pt"))
      return {QStringLiteral("pt_PT"), QStringLiteral("pt_BR")};
    if (code == QStringLiteral("zh"))
      return {QStringLiteral("zh_CN")};
    return {QStringLiteral("es")};
  }

  explicit QtTranslations(const QString &appDir,
                          CatalogQtTranslator::Lookup lookup = {})
      : m_appDir(appDir), m_catalog(std::move(lookup)) {}
  ~QtTranslations() { clear(); }
  QtTranslations(const QtTranslations &) = delete;
  QtTranslations &operator=(const QtTranslations &) = delete;

  // Sustituye los catálogos instalados por los del idioma indicado. Devuelve
  // cuántos se han podido cargar.
  int apply(const QString &appLocale) {
    clear();
    const QStringList dirs = {
        QDir(m_appDir).filePath(QStringLiteral("translations")),
        QLibraryInfo::path(QLibraryInfo::TranslationsPath)};
    const QStringList locales = qtLocalesFor(appLocale);
    // El último instalado es el primero que consulta Qt: el preferido al final.
    for (auto it = locales.crbegin(); it != locales.crend(); ++it) {
      for (const QString &dir : dirs) {
        auto translator = std::make_unique<QTranslator>();
        if (translator->load(QStringLiteral("qt_") + *it, dir)) {
          QCoreApplication::installTranslator(translator.get());
          m_installed.push_back(std::move(translator));
          break;
        }
      }
    }
    // El catálogo de la aplicación, el último: Qt lo consulta antes que a
    // los demás y, si no tiene la clave, sigue con ellos.
    QCoreApplication::installTranslator(&m_catalog);
    return static_cast<int>(m_installed.size());
  }

private:
  void clear() {
    QCoreApplication::removeTranslator(&m_catalog);
    for (const auto &translator : m_installed)
      QCoreApplication::removeTranslator(translator.get());
    m_installed.clear();
  }

  QString m_appDir;
  CatalogQtTranslator m_catalog;
  std::vector<std::unique_ptr<QTranslator>> m_installed;
};

#endif
