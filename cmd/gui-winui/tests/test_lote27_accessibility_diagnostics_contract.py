# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Fronteras de seguridad y estructura accesible del lote 27."""
from pathlib import Path
import re
import unittest
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[3]
SRC = ROOT / 'cmd/gui-winui/src'
UI = SRC / 'GrxFirma.WinUI'
CORE = SRC / 'GrxFirma.WinUI.Core'
XNAME = '{http://schemas.microsoft.com/winfx/2006/xaml}Name'


class Lote27Contracts(unittest.TestCase):
    def test_seal_handles_share_canvas_and_are_outside_the_scaled_seal_region(self):
        root = ET.parse(UI / 'Views/SignPage.xaml').getroot()
        parents = {child: parent for parent in root.iter() for child in parent}
        named = {element.get(XNAME): element for element in root.iter() if element.get(XNAME)}
        region = named['VisibleSealPreviewRegion']
        for name in ('VisibleSealRotateHandle', 'VisibleSealResizeHandle'):
            self.assertIs(parents[named[name]], parents[region])
            self.assertEqual(parents[named[name]].tag.rsplit('}', 1)[-1], 'Canvas')
            self.assertIn('HasSealOnPreviewPage', named[name].get('Visibility'))

    def test_batch_live_progress_is_polite_and_periodic_even_without_per_file_events(self):
        root = ET.parse(UI / 'Views/SignPage.xaml').getroot()
        live = next(element for element in root.iter() if element.get(XNAME) == 'BatchLiveProgress')
        self.assertEqual(live.get('AutomationProperties.LiveSetting'), 'Polite')
        code = (UI / 'Views/SignPage.xaml.cs').read_text()
        self.assertIn('TimeSpan.FromSeconds(10)', code)
        batch = code[code.index('private async void OnSignBatchClick'):code.index('private void OnCancelClick')]
        self.assertIn('finally', batch)
        self.assertIn('timer.Stop()', batch)
        self.assertIn('LiveAnnouncer.Announce(BatchLiveProgress)', batch)

    def test_local_checks_accept_no_addresses_and_use_only_two_read_actions(self):
        code = (CORE / 'Diagnostics/ContextualFailureAssistant.cs').read_text()
        actions = re.findall(r'TryGetOperations\(DesktopOperationActions\.(\w+)', code)
        self.assertEqual(actions, ['CheckCertificates', 'ClockDiagnostics'])
        for forbidden in ('HttpClient', 'WebRequest', 'Process.Start', 'Uri.TryCreate', 'SignAsync('):
            self.assertNotIn(forbidden, code)
        self.assertIn('new ClockDiagnosticsParameters()', (CORE / 'Operations/DesktopOperationsClient.cs').read_text())

    def test_contextual_check_has_native_button_status_and_bounded_lifetime(self):
        root = ET.parse(UI / 'Controls/OperationDiagnosticDialog.xaml').getroot()
        named = {element.get(XNAME): element for element in root.iter() if element.get(XNAME)}
        self.assertEqual(named['CheckLocalButton'].tag.rsplit('}', 1)[-1], 'Button')
        self.assertEqual(named['LocalCheckStatus'].get('AutomationProperties.LiveSetting'), 'Polite')
        code = (UI / 'Controls/OperationDiagnosticDialog.xaml.cs').read_text()
        self.assertIn('TimeSpan.FromSeconds(15)', code)
        self.assertIn('_localCheckCancellation?.Cancel()', code)
        self.assertNotIn('exception.Message', code)

    def test_code_brushes_observe_theme_and_system_high_contrast(self):
        code = (UI / 'Controls/ThemeBrushes.cs').read_text()
        self.assertIn('ActualThemeChanged +=', code)
        self.assertIn('HighContrastChanged +=', code)
        self.assertIn('ConditionalWeakTable<FrameworkElement, BrushSet>', code)
        self.assertIn('brush.Color = source.Color', code)
        self.assertIn('DispatcherQueue.TryEnqueue', code)


if __name__ == '__main__':
    unittest.main()
