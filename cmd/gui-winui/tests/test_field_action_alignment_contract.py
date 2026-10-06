# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato de alineación de acciones adyacentes a campos con Header."""

from pathlib import Path
import unittest
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[1] / "src/GrxFirma.WinUI"
XAML_NAME = "{http://schemas.microsoft.com/winfx/2006/xaml}Name"
FIELDS = {"TextBox", "NumberBox", "ComboBox"}
ACTIONS = {"Button", "ResponsiveActionPanel", "StackPanel", "HelpRow"}


def local_name(element):
    return element.tag.rsplit("}", 1)[-1]


def wide_positions(root):
    """Resuelve las posiciones que cambian en AdaptiveTrigger Wide."""
    positions = {}
    for state in root.iter():
        if local_name(state) != "VisualState" or state.get(XAML_NAME) != "Wide":
            continue
        for setter in state.iter():
            if local_name(setter) != "Setter":
                continue
            target = setter.get("Target", "")
            for property_name in ("Grid.Row", "Grid.Column"):
                suffix = f".({property_name})"
                if target.endswith(suffix):
                    positions[(target[: -len(suffix)], property_name)] = int(setter.get("Value"))
    return positions


def position(element, property_name, wide):
    name = element.get(XAML_NAME)
    if name and (name, property_name) in wide:
        return wide[(name, property_name)]
    return int(element.get(property_name, "0"))


class FieldActionAlignmentContractTests(unittest.TestCase):
    def test_all_adjoining_buttons_align_in_wide_and_narrow_layouts(self):
        found = []
        for folder in (ROOT / "Views", ROOT / "Controls"):
            for path in folder.glob("*.xaml"):
                root = ET.parse(path).getroot()
                wide = wide_positions(root)
                for grid in root.iter():
                    if local_name(grid) != "Grid":
                        continue
                    children = list(grid)
                    # Un campo con su botón «?» va dentro de HelpRow: la fila
                    # ocupa la celda y el campo conserva Header y MinHeight.
                    fields = []
                    for e in children:
                        inner = e
                        if local_name(e) == "HelpRow" and len(e):
                            inner = list(e)[0]
                        if local_name(inner) in FIELDS and inner.get("Header"):
                            fields.append((e, inner))
                    # Una HelpRow es acción solo si envuelve un botón (no un campo).
                    actions = [e for e in children if local_name(e) in ACTIONS
                               and (local_name(e) != "HelpRow"
                                    or (len(e) and local_name(list(e)[0]) == "Button"))]
                    for cell, field in fields:
                        for action in actions:
                            if position(action, "Grid.Row", wide) != position(cell, "Grid.Row", wide):
                                continue
                            if position(action, "Grid.Column", wide) <= position(cell, "Grid.Column", wide):
                                continue
                            label = f"{path.name}: {field.get('Header')} / {action.get(XAML_NAME, local_name(action))}"
                            found.append((path.name, label))
                            with self.subTest(pair=label):
                                self.assertGreaterEqual(float(grid.get("ColumnSpacing", "0")), 8)
                                self.assertGreaterEqual(float(field.get("MinHeight", "0")), 40)
                                buttons = [action] if local_name(action) == "Button" else [
                                    e for e in action.iter() if local_name(e) == "Button"
                                ]
                                self.assertTrue(buttons)
                                if local_name(action) != "Button":
                                    self.assertEqual(action.get("VerticalAlignment"), "Bottom")
                                for button in buttons:
                                    self.assertEqual(button.get("VerticalAlignment"), "Bottom")
                                    self.assertEqual(float(button.get("MinHeight", "0")), float(field.get("MinHeight")))
                                narrow_row = int(action.get("Grid.Row", "0"))
                                field_row = int(cell.get("Grid.Row", "0"))
                                if narrow_row > field_row:
                                    self.assertGreaterEqual(float(grid.get("RowSpacing", "0")), 8)
                                    row_defs = next(e for e in children if local_name(e) == "Grid.RowDefinitions")
                                    rows = [e for e in row_defs if local_name(e) == "RowDefinition"]
                                    self.assertEqual(rows[narrow_row].get("Height"), "Auto")
                                else:
                                    self.assertEqual(narrow_row, field_row)
                                    self.assertGreater(
                                        int(action.get("Grid.Column", "0")),
                                        int(cell.get("Grid.Column", "0")),
                                        f"{label}: la acción se superpone al campo en el estado estrecho",
                                    )

        self.assertGreaterEqual(len(found), 11, found)
        self.assertEqual(
            {page for page, _ in found},
            {"CertificatesPage.xaml", "HashPage.xaml", "ProtectPage.xaml", "SignPage.xaml", "VerifyPage.xaml"},
        )


if __name__ == "__main__":
    unittest.main()
