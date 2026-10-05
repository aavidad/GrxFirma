// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "savedialognames.h"

#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickWindow>
#include <QTemporaryDir>
#include <QtTest>

namespace {

QQuickItem *findVisibleItem(QQuickItem *item, const QString &name) {
  for (QQuickItem *child : item->childItems()) {
    if (child->objectName() == name && child->isVisible())
      return child;
    if (QQuickItem *found = findVisibleItem(child, name))
      return found;
  }
  return nullptr;
}

QString fileNameFieldText() {
  for (QWindow *window : QGuiApplication::topLevelWindows()) {
    auto *quick = qobject_cast<QQuickWindow *>(window);
    if (!quick)
      continue;
    if (QQuickItem *field = findVisibleItem(quick->contentItem(), QStringLiteral("fileNameTextField")))
      return field->property("text").toString();
  }
  return QStringLiteral("<sin campo>");
}

} // namespace

class SaveDialogNamesTest final : public QObject {
  Q_OBJECT

private slots:
  void proposedNameOfNewFile();
  void ignoresFoldersAndRemoteUrls();
  void showsTheProposedNameInQtQuickDialog();
};

void SaveDialogNamesTest::proposedNameOfNewFile() {
  QCOMPARE(SaveDialogNames::proposedName(QUrl::fromLocalFile("/tmp/acta 3_firmado.pdf")),
           QStringLiteral("acta 3_firmado.pdf"));
}

void SaveDialogNamesTest::ignoresFoldersAndRemoteUrls() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  QCOMPARE(SaveDialogNames::proposedName(QUrl::fromLocalFile(dir.path())), QString());
  QCOMPARE(SaveDialogNames::proposedName(QUrl("https://ejemplo.es/a.pdf")), QString());
  QCOMPARE(SaveDialogNames::proposedName(QUrl()), QString());
}

// El diálogo propio de Qt Quick deja vacío el campo con un nombre que aún no
// existe; tras abrirlo, la ayuda lo rellena y, al reabrir con otro nombre,
// lo cambia.
void SaveDialogNamesTest::showsTheProposedNameInQtQuickDialog() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  SaveDialogNames helper;
  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty("saveDialogNames", &helper);
  engine.rootContext()->setContextProperty("carpeta", QUrl::fromLocalFile(dir.path()));
  engine.loadData(R"(
import QtQuick
import QtQuick.Controls
import QtQuick.Dialogs
ApplicationWindow {
    width: 800; height: 600; visible: true
    function abrir(nombre) {
        dialogo.close()
        dialogo.currentFolder = carpeta
        dialogo.selectedFile = carpeta + "/" + nombre
        dialogo.open()
    }
    FileDialog {
        id: dialogo
        objectName: "dialogo"
        fileMode: FileDialog.SaveFile
        onVisibleChanged: if (visible) Qt.callLater(saveDialogNames.showProposedNames)
    }
}
)");
  QCOMPARE(engine.rootObjects().size(), 1);
  QObject *root = engine.rootObjects().first();

  QMetaObject::invokeMethod(root, "abrir", Q_ARG(QVariant, QStringLiteral("contrato_firmado.pdf")));
  // Versiones de Qt sin campo de nombre propio (o con diálogo nativo) no
  // tienen el fallo que se corrige aquí.
  if (!QTest::qWaitFor([] { return fileNameFieldText() != QStringLiteral("<sin campo>"); }, 3000))
    QSKIP("Este Qt no usa el diálogo propio con campo de nombre");
  QTRY_COMPARE_WITH_TIMEOUT(fileNameFieldText(), QStringLiteral("contrato_firmado.pdf"), 3000);

  QMetaObject::invokeMethod(root, "abrir", Q_ARG(QVariant, QStringLiteral("contrato_firmado_001.pdf")));
  QTRY_COMPARE_WITH_TIMEOUT(fileNameFieldText(), QStringLiteral("contrato_firmado_001.pdf"), 3000);
}

QTEST_MAIN(SaveDialogNamesTest)
#include "savedialognames_test.moc"
