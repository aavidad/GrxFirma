// SPDX-License-Identifier: EUPL-1.2
#include "../translatorbridge.h"
#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickWindow>
#include <QJsonArray>
#include <QJsonDocument>
#include <QFile>
#include <QTimer>

int main(int argc, char **argv) {
    QGuiApplication app(argc, argv);
    if (argc != 6) return 2;
    TranslatorBridge translator;
    translator.setLocale("es");
    QFile data(argv[1]);
    if (!data.open(QIODevice::ReadOnly)) return 3;
    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("i18n", &translator);
    engine.rootContext()->setContextProperty("previewItems", QJsonDocument::fromJson(data.readAll()).array().toVariantList());
    engine.rootContext()->setContextProperty("captureScale", QString::fromLocal8Bit(argv[2]).toInt());
    engine.rootContext()->setContextProperty("captureScenario", QString::fromLocal8Bit(argv[3]));
    engine.load(QUrl::fromLocalFile(QString::fromLocal8Bit(argv[4])));
    if (engine.rootObjects().isEmpty()) return 4;
    auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
    if (!window) return 5;
    QTimer::singleShot(700, [&]() { app.exit(window->grabWindow().save(QString::fromLocal8Bit(argv[5])) ? 0 : 6); });
    return app.exec();
}
