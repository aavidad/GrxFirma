# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""FolderDialog llegó a QtQuick.Dialogs en Qt 6.3. Importar una versión
anterior hace que Qt no cargue el componente y, con él, toda la interfaz:
la aplicación se cierra al arrancar sin mostrar nada."""

import pathlib
import re
import unittest

QML_DIR = pathlib.Path(__file__).resolve().parents[1] / "qml"
IMPORT = re.compile(r"^import\s+QtQuick\.Dialogs(?:\s+(\d+)\.(\d+))?\s*$", re.MULTILINE)


class QmlDialogImportsContract(unittest.TestCase):
    def test_folder_dialog_needs_qt_6_3_or_unversioned_import(self) -> None:
        offenders = []
        for path in sorted(QML_DIR.rglob("*.qml")):
            source = path.read_text(encoding="utf-8")
            if not re.search(r"\bFolderDialog\s*\{", source):
                continue
            match = IMPORT.search(source)
            if match is None:
                offenders.append(f"{path.name}: sin import de QtQuick.Dialogs")
                continue
            if match.group(1) is not None and (int(match.group(1)), int(match.group(2))) < (6, 3):
                offenders.append(f"{path.name}: QtQuick.Dialogs {match.group(1)}.{match.group(2)}")
        self.assertEqual([], offenders)


if __name__ == "__main__":
    unittest.main()
