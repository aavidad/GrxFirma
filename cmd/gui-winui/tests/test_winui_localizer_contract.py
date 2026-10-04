# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
import pathlib
import re
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
SOURCE_ROOT = ROOT / "cmd/gui-winui/src"
UI = SOURCE_ROOT / "GrxFirma.WinUI"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
VISIBLE = {
    "Text", "Content", "Header", "PlaceholderText", "Title", "Message",
    "Description", "PrimaryButtonText", "SecondaryButtonText",
    "CloseButtonText", "OnContent", "OffContent", "ToolTip", "Name",
    "HelpText", "FullDescription",
}
VISIBLE_XAML_ATTRIBUTES = VISIBLE - {"Name", "HelpText", "FullDescription", "ToolTip"}
VISIBLE_XAML_ATTRIBUTES |= {
    "AutomationProperties.Name", "AutomationProperties.HelpText",
    "AutomationProperties.FullDescription", "ToolTipService.ToolTip",
}
GROUP_B_XAML = {
    "MainWindow.xaml", "OperationDiagnosticDialog.xaml",
    "CertificatesPage.xaml", "SettingsPage.xaml", "DiagnosticsPage.xaml",
    "AboutPage.xaml", "FacturaePage.xaml", "EniPage.xaml",
}
GROUP_B_PAGES = {"CertificatesPage", "SettingsPage", "DiagnosticsPage",
                 "AboutPage", "FacturaePage", "EniPage", "MainWindow"}
GROUP_B_CS = {"App.xaml.cs", "MainWindow.xaml.cs"}
PROPER_NAMES = {"GrxFirma", "FNMT", "FACe", "DIR3", "PAdES", "CAdES",
                "XAdES", "ASiC", "ENI", "CSV", "PKCS#11", "PKCS#12", "DNIe"}
# Ejemplos de direcciones y patrones que el usuario puede copiar literalmente.
VISIBLE_FORMAT_EXCEPTIONS = {
    ("Views/SettingsPage.xaml", "PlaceholderText", "proxy.organizacion.es"),
    ("Views/SettingsPage.xaml", "PlaceholderText",
     "localhost\n127.0.0.1\n*.organizacion.es"),
}
C_SHARP_FORMAT_EXCEPTIONS = {
    ("ViewModels/CertificatesPageViewModel.cs", "dd/MM/yyyy HH:mm"),
    ("ViewModels/CertificatesPageViewModel.cs", "dd/MM/yyyy"),
    ("ViewModels/FacturaePageViewModel.cs", "es-ES"),
    ("Views/EniPage.xaml.cs", "yyyy-MM-ddTHH:mm:sszzz"),
    ("Views/EniPage.xaml.cs", "EE01"),
    ("Views/EniPage.xaml.cs", "TD99"),
    ("Views/EniPage.xaml.cs", "E01"),
    ("Services/WindowsFilePickerService.cs", "Facturae 3.2.2"),
    ("Services/WindowsFilePickerService.cs", "CMS EnvelopedData"),
    ("Services/WindowsFilePickerService.cs", "CMS EncryptedData"),
    ("Services/WindowsFilePickerService.cs", "CMS AuthEnvelopedData"),
    ("Services/WindowsFilePickerService.cs", "CMS SignedAndEnvelopedData"),
    ("Services/WindowsStartupRegistration.cs", "--frontend=winui --start-hidden"),
    ("Services/WindowsSecurePasswordPromptService.cs", "GrxFirma/contraseña-transitoria"),
    ("Services/WindowsSecurePasswordPromptService.cs", "STATE"),
    ("Services/WindowsSecurePasswordPromptService.cs", "RANGE"),
    ("Services/WindowsSecurePasswordPromptService.cs", "ARGUMENT"),
    ("Services/WindowsSecurePasswordPromptService.cs", "UNEXPECTED_"),
    ("Services/WindowsHelpLauncherService.cs", "ayuda-{expression}.pdf"),
    ("Services/SingleInstanceSignal.cs", "D:P(A;;0x001F0003;;;{expression})(A;;0x001F0003;;;SY)"),
    ("Controls/OperationDiagnosticDialog.xaml.cs", "diagnostico-grxfirma-{expression}"),
    ("App.xaml.cs", "{DateTimeOffset.Now:O} {error.GetType().FullName} 0x{error.HResult:X8}{Environment.NewLine}{error}{Environment.NewLine}{Environment.NewLine}"),
}
LANGUAGE_SELF_NAMES = {"Español", "Català", "Valencià", "Euskara", "Galego",
                       "English", "Deutsch", "Français", "Português", "Italiano", "中文"}
# Identificadores de protocolo, estado, operación, campo JSON y sustitución de
# plantillas. Son valores de máquina, no etiquetas presentadas a una persona.
MACHINE_WORDS = {
    "about", "administracion", "admission", "artifacts", "available",
    "avidad", "browser", "ca", "cades", "certificates", "ciudadano",
    "count", "current", "date", "days", "de", "diagnostico", "diagnostics",
    "disabled", "empty", "en", "eni", "environment", "es", "eu", "exit",
    "facturae", "failure", "firma", "fisica", "fr", "gl", "hash", "help",
    "inconclusive", "issuer", "it", "keychain", "keys", "latest", "locales",
    "logs", "managers", "manual", "maximum", "none", "odf", "ooxml",
    "owner", "pades", "protect", "protocol", "pt", "reason", "remaining",
    "renewal", "representacion", "resident", "revoked", "sello", "service",
    "settings", "shown", "sign", "signable", "store", "subject", "success",
    "support", "system", "targets", "tls", "total", "unavailable", "unknown",
    "usable", "va", "valid", "verify", "version", "xades", "xmldsig", "zh",
}
# Nombres de clases nativas Win32 y funciones importadas; nunca son texto UI.
WIN32_NAMES = {
    "RtlZeroMemory", "WideCharToMultiByte",
    "ConvertStringSecurityDescriptorToSecurityDescriptorW", "CreateEventW",
    "Shell_NotifyIconW", "LoadImageW", "CredUIPromptForCredentialsW",
    "EnumThreadWindows", "GetWindow", "GetClassNameW",
    "DialogBoxIndirectParamW", "CreateWindowExW", "SetWindowLongPtrW",
    "GetWindowLongPtrW", "SendMessageW", "PostMessageW", "EndDialog",
    "SetWindowTextW", "GetClientRect", "SetFocus", "GetWindowThreadProcessId",
    "GetCurrentThreadId", "GetModuleHandleW", "GetStockObject",
    "AppendMenuW", "RegisterWindowMessageW", "TaskbarCreated",
    "STATIC", "EDIT", "BUTTON",
}
# Campos de certificados y valores de interoperabilidad que se comparan por su
# valor exacto. Los títulos traducibles ya figuran en los catálogos.
MACHINE_EXACT = {
    "Valido", "CN=", "@firma", "XMLDSig", "F", "J", "C2", "N", "CLI",
    "Programs", "Suite", "DesktopLauncher", "Assets",
    "GrxFirma:DesktopLauncher", "lastSeenVersion", "checkForUpdates",
    "lastAttempt", "WIN32_{expression}",
}


def static_xaml_literals(paths):
    for path in paths:
        for element in ET.parse(path).getroot().iter():
            value = (element.text or "").strip()
            if (value and not value.startswith("{")
                    and re.search(r"[A-Za-zÀ-ÿ]", value)
                    and value not in PROPER_NAMES):
                yield path.relative_to(UI).as_posix(), "inner text", value
            for raw_name, value in element.attrib.items():
                name = raw_name.rsplit("}", 1)[-1]
                if (name not in VISIBLE_XAML_ATTRIBUTES or value.startswith("{")
                        or not re.search(r"[A-Za-zÀ-ÿ]", value)
                        or value in PROPER_NAMES):
                    continue
                relative = path.relative_to(UI).as_posix()
                if (relative, name, value) not in VISIBLE_FORMAT_EXCEPTIONS:
                    yield relative, name, value


def untranslated_xaml(paths):
    spanish = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
    spanish_values = {value: key for key, value in spanish.items()}
    catalogs = [json.loads(path.read_text(encoding="utf-8"))
                for path in LOCALES.glob("*.json")]
    return [(path, name, value) for path, name, value in static_xaml_literals(paths)
            if (key := value if value in spanish else spanish_values.get(value)) is None
            or any(not catalog.get(key) for catalog in catalogs)]


def belongs_to_group_b_cs(path):
    if not path.is_relative_to(UI):
        return False
    relative = path.relative_to(UI)
    if relative.name in GROUP_B_CS:
        return True
    if relative.parts[0] in {"Views", "ViewModels"}:
        return any(relative.name.startswith(name) for name in GROUP_B_PAGES)
    if relative.parts[0] == "Controls":
        return "Dialog" in relative.name
    return relative.parts[0] == "Services"


def source_relative(path):
    if path.is_relative_to(UI):
        return path.relative_to(UI).as_posix()
    return path.relative_to(SOURCE_ROOT).as_posix()


def is_machine_literal(path, value):
    relative = source_relative(path)
    if path.name == "Localizer.cs":
        return True  # Nombres de DependencyProperty y claves de catálogo.
    if path.name == "CatalogLocalizer.cs":
        return True  # Grupos de captura y patrones de las plantillas.
    if ((relative, value) in C_SHARP_FORMAT_EXCEPTIONS
            or value in PROPER_NAMES | LANGUAGE_SELF_NAMES | MACHINE_WORDS |
            WIN32_NAMES | MACHINE_EXACT):
        return True
    without_interpolation = re.sub(r"\{[^{}]*\}", "", value)
    if (not re.search(r"[A-Za-zÀ-ÿ]", without_interpolation)
            or re.fullmatch(r"[\s0-9xX:.+\-]*", without_interpolation)):
        return True
    # Rutas, dominios, formatos de fichero y destinos oficiales fijos.
    if (value.startswith(("http://", "https://", "mailto:", "/", "."))
            or "\\" in value
            or re.fullmatch(r"(?:[a-z0-9-]+\.)+[a-z]{2,}", value)
            or re.fullmatch(r"[\w.-]+\.(?:dll|exe|json|md|txt|ico|pdf|pem|crt|cer|p12|pfx|log)", value, re.I)):
        return True
    # Códigos de operación, claves de datos, evidencias, formatos y recursos UI.
    if (re.fullmatch(r"[A-Z][A-Z0-9_]*_[A-Z0-9_]+", value)
            or re.fullmatch(r"(?:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])", value)
            or re.fullmatch(r"[a-z0-9]+(?:[-_][a-z0-9]+)+", value)
            or value.startswith(("eni.", "exp.", "winui.", "phase:"))
            or value.endswith(("Brush", "Style"))):
        return True
    return False


def is_machine_context(path, source, line, value):
    """Exclude protocol/data tokens by syntax and use, never by Spanish vocabulary.

    Keep this separate from is_machine_literal so a one-word UI label remains
    visible when it occurs in a return value or a view-model assignment.
    """
    lines = source.splitlines()
    current = lines[line - 1] if line <= len(lines) else ""
    literal_at = current.find('"' + value + '"')
    before_literal = current[:literal_at] if literal_at >= 0 else current
    preceding = "\n".join(lines[max(0, line - 3):line])
    call_context = "\n".join(lines[max(0, line - 8):line])
    # A closed set of contract field names is initialized as a HashSet. The
    # collection syntax, rather than the spelling of each field, identifies it.
    if re.fullmatch(r'[A-Za-z][A-Za-z0-9_.-]*', value):
        before = "\n".join(lines[:line])
        collection_start = before.rfind('new HashSet<string>')
        if collection_start >= 0 and ']);' not in before[collection_start:]:
            return True
    # Serialization metadata and assembly access declarations define wire names.
    if re.search(r"\[(?:JsonPropertyName|JsonStringEnumMemberName|assembly:\s*InternalsVisibleTo)\s*\(", current):
        return True
    # Standard format specifiers, content types and HTTP header names are data.
    if re.search(r"(?:ToString|ParseExact|TryParseExact|GetDateTime)\s*\(\s*\$?@?$", before_literal):
        return True
    if re.search(r"(?:Headers\.|\.Headers|MediaType|HttpRequestHeaders|Accept\.|UserAgent\.)", current):
        return True
    # Comparisons and switch arms inspect stable protocol values; the string on
    # the right side of => may instead be a user-facing label.
    if re.search(r'(?:==|!=|\bis\b|\.Equals\s*\(|\.StartsWith\s*\(|\.EndsWith\s*\(|\.Contains\s*\(|IsStatus\s*\()\s*$', before_literal):
        return True
    if re.search(r'^\s*\"[^\"]+\"\s*(?:=>|:)', current) and current.strip().startswith(f'"{value}"'):
        return True
    # Indexers and named property lookups use strings as keys, not copy.
    if (re.search(r'\[\s*$', before_literal) or
            re.search(r'(?:GetProperty|TryGetProperty|GetPropertyOrDefault|TryGetValue|ContainsKey)\s*\(\s*$', before_literal)):
        return True
    # Log/trace event names and logging templates are developer diagnostics.
    if re.search(r'(?:Log(?:Debug|Trace|Information|Warning|Error|Critical)?|Trace|WriteLine|TrackEvent|EventSource)\s*\(\s*$', before_literal):
        return True
    if re.search(r'\b(?:File|Directory|Path)\.(?:Exists|Combine|GetFileName|GetExtension|ChangeExtension)\s*\(\s*$', before_literal):
        return True
    # IPC action identifiers, CLI switches, codes and MIME/file patterns are
    # stable machine vocabulary, including values inside object initializers.
    if (value.startswith('--') or
            re.fullmatch(r'[\w.-]+\.(?:xml|pdf|json|pem|cer|p12|pfx|csv|txt|dsig|xsig|p7s)', value, re.I) or
            re.fullmatch(r'[\w.-]*\{expression\}[\w.-]*\.(?:xml|pdf|json)', value, re.I)):
        return True
    if (value.startswith(('(?', '^')) or
            re.fullmatch(r'(?:yyyy|yy|MM|dd|HH|mm|ss|zzz|T|Z|[-/:.])+', value) or
            re.fullmatch(r'\{expression\}[_./-][A-Za-z0-9_.-]+', value)):
        return True
    # The update checker deliberately catches its parsing/network exceptions at
    # the UI boundary and presents a separate localized, safe status there.
    if path.name == 'OfficialUpdateChecker.cs' and re.search(r'throw new (?:InvalidDataException|HttpRequestException)\s*\(', preceding):
        return True
    # These typed option constructors pair a visible label with an invariant
    # backend code. A lowercase single token is the code; the label stays in
    # the catalog even when it is a single word.
    if (path.name in {'SignPageViewModel.cs', 'ProtectPageViewModel.cs',
                      'HashPageViewModel.cs', 'SignPage.xaml.cs',
                      'ProtectPage.xaml.cs', 'PublicCertificateExport.cs'}
            and re.fullmatch(r'[a-z][a-z0-9]*', value)):
        return True
    if (path.name in {'VerificationAssessment.cs', 'PortalSealSession.cs',
                      'DesktopOperationsClient.cs', 'CorrelationId.cs',
                      'OperationDiagnostic.cs', 'DiagnosticIncidentReport.cs'}
            and re.fullmatch(r'[a-z][a-z0-9]*', value)):
        return True
    if path.name == 'OperationDiagnosticPresentation.cs' and re.fullmatch(r'[a-z][a-z0-9]*', value):
        return True  # Categorías recibidas del protocolo de diagnóstico.
    if path.name == 'DesktopOperationContracts.cs' and re.search(r'public string Overwrite\b', preceding):
        return True  # Modo de sobrescritura serializado.
    if path.name == 'SignPageViewModel.cs' and re.fullmatch(r'(?:Baseline|T|LT|LTA)', value):
        return True  # Nombres normativos de los perfiles de firma.
    if path.name == 'SignPage.xaml.cs' and '.Contains(' in preceding:
        return True  # Subcadena de una comparación de estado existente.
    if (path.name == 'FacturaeInvoiceGenerator.cs' and
            re.fullmatch(r'[a-záéíóúñ]+(?:\s+[a-záéíóúñ]+)*', value) and
            re.search(r'Validate(?:Party|Dir3)\s*\(', call_context)):
        return True  # Etiqueta interna que se inserta en el error completo.
    if (path.name == 'FacturaeInvoiceGenerator.cs' and
            re.search(r'new AdministrativeCentre\s*\(', call_context) and
            re.fullmatch(r'[A-ZÁÉÍÓÚ][a-záéíóúñ]+(?:\s+[a-záéíóúñ]+)*', value)):
        return True  # Nombre normativo del rol en el XML Facturae.
    if re.fullmatch(r'[A-Za-z][A-Za-z0-9_.-]*', value) and not re.search(r'[À-ÿ]', value):
        if re.search(r'(?:Action|Method|Status|Phase|Code|Format|Algorithm|Mode|Type|Kind|Profile|Target|Store|Source|Owner|Namespace|XName)\s*=\s*\"', current):
            return True
        if re.search(r'(?:XElement|XAttribute|XName|XNamespace|XmlElement|XmlAttribute|PropertyName|CreateElement|CreateAttribute|GetProperty)\s*\(', current):
            return True
        if re.search(r'\b(?:case|or)\s+\"', current):
            return True
        # XML element/attribute names inside the Facturae serializer are wire
        # vocabulary. Human field labels in validation code remain candidates.
        if (path.name == 'FacturaeInvoiceGenerator.cs' and
                (re.fullmatch(r'[A-Z][A-Za-z0-9]*', value) or
                 re.search(r'(?:Element|Attribute|BuildParty|AmountElement)\s*\(', preceding))
                and line < 600):
            return True
        if (path.name == 'FacturaeInvoiceGenerator.cs' and
                re.search(r'new AdministrativeCentre\s*\(', call_context)):
            return True
    # Regex bodies and invariant report/JSON field names are not displayed.
    if re.search(r'\b(?:new Regex|Regex\.Matches|Regex\.IsMatch)\s*\(', current):
        return True
    if re.search(r'\b(?:JsonPropertyName|JsonSerializer|Utf8JsonWriter)\b', current):
        return True
    return False


def csharp_string_literals(source):
    """Scan normal, verbatim, interpolated and raw C# strings without comments."""

    def skip_interpolation(index):
        depth = 1
        while index < len(source) and depth:
            if source.startswith("//", index):
                end = source.find("\n", index)
                index = len(source) if end < 0 else end
            elif source.startswith("/*", index):
                end = source.find("*/", index + 2)
                index = len(source) if end < 0 else end + 2
            elif source[index] == "'":
                index += 1
                while index < len(source):
                    if source[index] == "\\":
                        index += 2
                    elif source[index] == "'":
                        index += 1
                        break
                    else:
                        index += 1
            elif source[index] == '"':
                # Una cadena dentro de la expresión no cierra la interpolada.
                index += 1
                while index < len(source):
                    if source[index] == "\\":
                        index += 2
                    elif source[index] == '"':
                        index += 1
                        break
                    else:
                        index += 1
            elif source[index] == "{":
                depth += 1
                index += 1
            elif source[index] == "}":
                depth -= 1
                index += 1
            else:
                index += 1
        return index

    index = 0
    line = 1
    while index < len(source):
        if source.startswith("//", index):
            end = source.find("\n", index)
            index = len(source) if end < 0 else end
            continue
        if source.startswith("/*", index):
            end = source.find("*/", index + 2)
            end = len(source) if end < 0 else end + 2
            line += source.count("\n", index, end)
            index = end
            continue
        if source[index] == "'":
            index += 1
            while index < len(source):
                if source[index] == "\\":
                    index += 2
                elif source[index] == "'":
                    index += 1
                    break
                else:
                    line += source[index] == "\n"
                    index += 1
            continue
        prefix = re.match(r'(?:\$@|@\$|\$+|@)?(?:"{3,}|")', source[index:])
        if prefix:
            start_line = line
            token = prefix.group()
            verbatim = "@" in token
            interpolated = "$" in token
            quotes = len(token) - len(token.rstrip('"'))
            index += len(token)
            if quotes >= 3:
                ending = '"' * quotes
                end = source.find(ending, index)
                end = len(source) if end < 0 else end
                value = source[index:end]
                next_index = min(len(source), end + quotes)
                line += source.count("\n", index, next_index)
                index = next_index
                yield start_line, value
                continue
            value = []
            while index < len(source):
                if verbatim and source.startswith('""', index):
                    value.append('""')
                    index += 2
                elif interpolated and source.startswith("{{", index):
                    value.append("{{")
                    index += 2
                elif interpolated and source[index] == "{":
                    end = skip_interpolation(index + 1)
                    value.append("{expression}")
                    line += source.count("\n", index, end)
                    index = end
                elif source[index] == '"':
                    index += 1
                    break
                elif not verbatim and source[index] == "\\":
                    value.append(source[index:index + 2])
                    index += 2
                else:
                    value.append(source[index])
                    line += source[index] == "\n"
                    index += 1
            yield start_line, "".join(value)
            continue
        line += source[index] == "\n"
        index += 1


def numbered_template(value):
    parts = value.split("{expression}")
    return "".join(part + ("{" + str(index) + "}" if index < len(parts) - 1 else "")
                   for index, part in enumerate(parts))


def untranslated_csharp(paths):
    spanish = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
    value_to_key = {value: key for key, value in spanish.items()}
    catalogs = [json.loads(path.read_text(encoding="utf-8"))
                for path in LOCALES.glob("*.json")]
    missing = []
    for path in paths:
        source = path.read_text(encoding="utf-8")
        for line, value in csharp_string_literals(source):
            template = numbered_template(value)
            key = (value if value in spanish else value_to_key.get(value)
                   or (template if template in spanish else value_to_key.get(template)))
            if ((key is not None and all(catalog.get(key) for catalog in catalogs))
                    or is_machine_literal(path, value)
                    or is_machine_context(path, source, line, value)):
                continue
            missing.append((source_relative(path), line, value))
    return missing


class WinUiLocalizerContractTests(unittest.TestCase):
    def test_csharp_scanner_ignores_comments_and_character_literals(self):
        source = """// \"comentario\"\nvar quote = '\"';\nvar label = \"Texto visible\";"""
        self.assertEqual(list(csharp_string_literals(source)),
                         [(3, "Texto visible")])

    def test_csharp_scanner_handles_interpolation_and_raw_strings(self):
        source = ('var a = ""; var b = $"Caducado el {FormatDate(x, '
                  '"dd/MM/yyyy")}"; var c = "Texto"; var d = """raw""";')
        self.assertEqual(list(csharp_string_literals(source)),
                         [(1, ""), (1, "Caducado el {expression}"),
                          (1, "Texto"), (1, "raw")])

    def test_unclassified_one_word_ui_label_is_not_exempt(self):
        self.assertFalse(is_machine_literal(UI / "Views/SettingsPage.xaml.cs", "PendienteNuevo"))
        self.assertTrue(is_machine_literal(UI / "ViewModels/DiagnosticsPageViewModel.cs",
                                           "certificate_inventory"))
        source = '[JsonPropertyName("userMessage")]\nvar title = "PendienteNuevo";'
        path = SOURCE_ROOT / "GrxFirma.WinUI.Core/Ipc/IpcContracts.cs"
        self.assertTrue(is_machine_context(path, source, 1, "userMessage"))
        self.assertFalse(is_machine_context(path, source, 2, "PendienteNuevo"))
        switch = '"invalid" => "Firma íntegra, pero no confiable",'
        self.assertTrue(is_machine_context(path, switch, 1, "invalid"))
        self.assertFalse(is_machine_context(path, switch, 1,
                                            "Firma íntegra, pero no confiable"))
        combined = 'if (status == "invalid") label = "PendienteNuevo";'
        self.assertTrue(is_machine_context(path, combined, 1, "invalid"))
        self.assertFalse(is_machine_context(path, combined, 1, "PendienteNuevo"))
        self.assertEqual(numbered_template("Error en {expression}: {expression}"),
                         "Error en {0}: {1}")

    def test_group_b_visible_xaml_literals_are_in_the_shared_catalog(self):
        paths = (path for path in UI.rglob("*.xaml") if path.name in GROUP_B_XAML)
        missing = untranslated_xaml(paths)
        if missing:
            self.fail(f"{len(missing)} visible group B literals lack a catalog key: {missing[:20]}")

    def test_group_b_csharp_human_text_is_in_the_shared_catalog(self):
        paths = (path for path in UI.rglob("*.cs") if belongs_to_group_b_cs(path))
        missing = untranslated_csharp(paths)
        if missing:
            self.fail(f"{len(missing)} group B C# text candidates lack a catalog key: {missing[:20]}")

    def test_pending_group_a_visible_xaml_literals_are_in_the_shared_catalog(self):
        paths = (path for path in UI.rglob("*.xaml") if path.name not in GROUP_B_XAML)
        missing = untranslated_xaml(paths)
        if missing:
            self.fail(f"{len(missing)} pending group A literals: {missing[:20]}")

    def test_pending_other_csharp_human_text_is_in_the_shared_catalog(self):
        paths = (path for path in SOURCE_ROOT.rglob("*.cs")
                 if not belongs_to_group_b_cs(path))
        missing = untranslated_csharp(paths)
        if missing:
            self.fail(f"{len(missing)} pending C# text candidates: {missing[:20]}")

    def test_visible_xaml_property_types_are_observed(self):
        implementation = (UI / "Services/Localizer.cs").read_text(encoding="utf-8")
        for path in UI.rglob("*.xaml"):
            for element in ET.parse(path).getroot().iter():
                for raw_name, value in element.attrib.items():
                    name = raw_name.rsplit("}", 1)[-1]
                    if name.startswith("AutomationProperties."):
                        name = name.split(".", 1)[1]
                    if name not in VISIBLE or not re.search(r"[A-Za-zÀ-ÿ]", value):
                        continue
                    if value.startswith("{") or value == "GrxFirma":
                        continue
                    with self.subTest(path=path.name, property=name, text=value[:60]):
                        if name == "ToolTip":
                            self.assertIn("ToolTipService.ToolTipProperty", implementation)
                        elif name in {"Name", "HelpText", "FullDescription"}:
                            self.assertIn(f"AutomationProperties.{name}Property", implementation)
                        else:
                            self.assertIn(f'"{name}"', implementation)

    def test_dynamic_dialogs_pass_through_localizer(self):
        for path in UI.rglob("*.cs"):
            if path.name == "Localizer.cs":
                continue
            source = path.read_text(encoding="utf-8")
            with self.subTest(path=str(path.relative_to(UI))):
                self.assertNotRegex(source, r"\b(?:dialog|confirmation)\.ShowAsync\(\)")

    def test_selected_language_refreshes_page_and_menu(self):
        window = (UI / "MainWindow.xaml.cs").read_text(encoding="utf-8")
        settings = (UI / "Views/SettingsPage.xaml.cs").read_text(encoding="utf-8")
        catalog = (UI / "Services/SealUiCatalog.cs").read_text(encoding="utf-8")
        self.assertIn("ApplyLanguagePreference(result.Data.Language)", window)
        self.assertIn("RootNavigation.FooterMenuItems", window)
        self.assertIn("ContentFrame.Navigate(pageType)", window)
        self.assertIn("_app.RefreshTrayLanguage()", window)
        self.assertIn("RefreshProgrammaticLanguage()", window)
        self.assertIn("app.ApplyLanguagePreference(ViewModel.SelectedLanguage.Value)", settings)
        self.assertIn("Localizer.Text(key)", catalog)
        self.assertNotIn("se muestra actualmente en español", (UI / "Views/SettingsPage.xaml").read_text(encoding="utf-8"))

    def test_custom_winui_labels_use_selected_language(self):
        paths = [UI / "MainWindow.xaml.cs", UI / "App.xaml.cs"]
        paths += [UI / "Views" / f"{name}.xaml.cs" for name in
                  ("EniPage", "FacturaePage", "AboutPage", "DiagnosticsPage",
                   "SettingsPage", "CertificatesPage")]
        paths += [UI / "Services" / f"{name}.cs" for name in
                  ("WindowsTrayIcon", "WindowsHelpLauncherService",
                   "WindowsFilePickerService")]
        for path in paths:
            with self.subTest(file=path.name):
                self.assertNotIn("CurrentUICulture.TwoLetterISOLanguageName",
                                 path.read_text(encoding="utf-8"))

    def test_release_notes_shown_in_winui_have_catalog_translations(self):
        window = (UI / "MainWindow.xaml.cs").read_text(encoding="utf-8")
        self.assertIn("line = Localizer.Text(line);", window)
        notes = (ROOT / "docs/NOVEDADES.md").read_text(encoding="utf-8")
        visible = set()
        for raw in notes.splitlines():
            if raw.startswith("- "):
                visible.add(raw[2:].replace("**", "").replace("`", ""))
            elif raw.startswith("## ") and not re.match(r"## [0-9]", raw):
                visible.add(raw[3:])
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            with self.subTest(locale=path.stem):
                self.assertFalse(visible - catalog.keys(),
                                 sorted(visible - catalog.keys()))

    def test_explicit_localizer_keys_exist_in_every_catalog(self):
        used = set()
        templates = set()
        for path in UI.rglob("*.cs"):
            if path.name == "Localizer.cs":
                continue
            source = path.read_text(encoding="utf-8")
            used.update(re.findall(r'Localizer\.(?:Text|Fill)\(\s*"([^"\n]+)"', source))
            templates.update(re.findall(r'Localizer\.Fill\(\s*"([^"\n]+)"', source))
        self.assertTrue(used)
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            with self.subTest(locale=path.stem):
                self.assertFalse(used - catalog.keys(), sorted(used - catalog.keys()))
                for key in templates:
                    self.assertEqual(
                        set(re.findall(r"\{[a-z]+\}", key)),
                        set(re.findall(r"\{[a-z]+\}", catalog[key])),
                        f"{path.stem}: placeholders changed in {key!r}")


if __name__ == "__main__":
    unittest.main()
