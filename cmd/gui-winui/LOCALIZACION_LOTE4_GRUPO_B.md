# Lote 4 WinUI — grupo B

## Resultado

- Literales visibles pendientes del grupo B: **0 en XAML y 0 candidatos C#** según el contrato estricto.
- Claves nuevas: **876**, añadidas juntas al final de cada uno de los 11 catálogos. Cada catálogo contiene 3908 claves.
- Los XAML ya se traducen y se vuelven a traducir mediante `Localizer.Apply`; se añadieron `OnContent` y `OffContent` para `ToggleSwitch`.
- Los textos calculados usan `Localizer.Fill` con marcadores nombrados; las etiquetas personalizadas, los selectores de ficheros, los diálogos y la bandeja usan `Localizer.Language`.
- Las 69 viñetas distintas y dos encabezados textuales de `docs/NOVEDADES.md` se traducen al mostrarse en el diálogo de notas de versión.

## Excepciones justificadas

- `SettingsPage.xaml`: `proxy.organizacion.es` y `localhost\n127.0.0.1\n*.organizacion.es` son ejemplos copiables de host y patrones.
- Nombres propios y siglas permitidos: GrxFirma, FNMT, FACe, DIR3, PAdES, CAdES, XAdES, ASiC, ENI, CSV, PKCS#11, PKCS#12 y DNIe.
- Formatos visibles sin traducir: `Facturae 3.2.2`, `CMS EnvelopedData`, `CMS EncryptedData`, `CMS AuthEnvelopedData` y `CMS SignedAndEnvelopedData`.
- Los formatos de fecha, extensiones, rutas, URLs, códigos de operación, nombres de API Win32, claves de datos, identificadores de protocolo y registros técnicos se excluyen mediante reglas o excepciones exactas en el contrato; no son texto de interfaz.

## Archivos modificados

- `cmd/gui-winui/src/GrxFirma.WinUI/App.xaml.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/MainWindow.xaml.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Services/ISecurePasswordPromptService.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Services/Localizer.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Services/WindowsFilePickerService.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Services/WindowsHelpLauncherService.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Services/WindowsPdfPreviewService.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Services/WindowsTrayIcon.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/AboutPageViewModel.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/CertificatesPageViewModel.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/DiagnosticsPageViewModel.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/FacturaePageViewModel.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/MainWindowViewModel.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/SettingsPageViewModel.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Views/AboutPage.xaml.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Views/CertificatesPage.xaml.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Views/DiagnosticsPage.xaml.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Views/EniPage.xaml.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Views/FacturaePage.xaml.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI/Views/SettingsPage.xaml.cs`
- `cmd/gui-winui/tests/test_winui_localizer_contract.py`
- `internal/adapters/outbound/common/localizador/locales/ca.json`
- `internal/adapters/outbound/common/localizador/locales/de.json`
- `internal/adapters/outbound/common/localizador/locales/en.json`
- `internal/adapters/outbound/common/localizador/locales/es.json`
- `internal/adapters/outbound/common/localizador/locales/eu.json`
- `internal/adapters/outbound/common/localizador/locales/fr.json`
- `internal/adapters/outbound/common/localizador/locales/gl.json`
- `internal/adapters/outbound/common/localizador/locales/it.json`
- `internal/adapters/outbound/common/localizador/locales/pt.json`
- `internal/adapters/outbound/common/localizador/locales/va.json`
- `internal/adapters/outbound/common/localizador/locales/zh.json`

## Integración pendiente del grupo A

- XAML: **255** literales en 6 ficheros.
- C#: **1334** candidatos del escáner en 34 ficheros; 696 están en WinUI.Core y 638 en la interfaz. Estos son candidatos sin revisión de visibilidad definitiva.
- Ambos controles del grupo A están marcados como `expectedFailure`; se deben quitar al integrar la otra rama. El contrato estricto ya recorre todo `cmd/gui-winui/src`.

### XAML pendiente

| Literales | Archivo |
|---:|---|
| 11 | `Controls/SigningMethodsHelp.xaml` |
| 24 | `Views/HashPage.xaml` |
| 24 | `Views/HelpPage.xaml` |
| 50 | `Views/ProtectPage.xaml` |
| 124 | `Views/SignPage.xaml` |
| 22 | `Views/VerifyPage.xaml` |

### C# pendiente (candidatos)

| Candidatos | Archivo |
|---:|---|
| 3 | `GrxFirma.WinUI.Core/Diagnostics/DiagnosticIncidentReport.cs` |
| 17 | `GrxFirma.WinUI.Core/Diagnostics/OperationDiagnostic.cs` |
| 75 | `GrxFirma.WinUI.Core/Diagnostics/OperationDiagnosticMapper.cs` |
| 21 | `GrxFirma.WinUI.Core/Diagnostics/OperationDiagnosticPresentation.cs` |
| 140 | `GrxFirma.WinUI.Core/Facturae/FacturaeInvoiceGenerator.cs` |
| 2 | `GrxFirma.WinUI.Core/Ipc/CorrelationId.cs` |
| 8 | `GrxFirma.WinUI.Core/Ipc/IpcClientException.cs` |
| 24 | `GrxFirma.WinUI.Core/Ipc/IpcContracts.cs` |
| 7 | `GrxFirma.WinUI.Core/Ipc/NdjsonIpcClient.cs` |
| 4 | `GrxFirma.WinUI.Core/Ipc/WinUiLaunchOptions.cs` |
| 8 | `GrxFirma.WinUI.Core/Operations/DesktopAdministrativeContracts.cs` |
| 8 | `GrxFirma.WinUI.Core/Operations/DesktopCertificateCredentialFile.cs` |
| 21 | `GrxFirma.WinUI.Core/Operations/DesktopDiagnosticContracts.cs` |
| 167 | `GrxFirma.WinUI.Core/Operations/DesktopOperationContracts.cs` |
| 3 | `GrxFirma.WinUI.Core/Operations/DesktopOperationSession.cs` |
| 11 | `GrxFirma.WinUI.Core/Operations/DesktopOperationsClient.cs` |
| 7 | `GrxFirma.WinUI.Core/Operations/DesktopProxyContracts.cs` |
| 116 | `GrxFirma.WinUI.Core/Operations/DesktopSettingsContracts.cs` |
| 5 | `GrxFirma.WinUI.Core/Operations/DesktopUpdateContracts.cs` |
| 16 | `GrxFirma.WinUI.Core/Operations/OfficialUpdateChecker.cs` |
| 12 | `GrxFirma.WinUI.Core/Operations/PortalSealSession.cs` |
| 2 | `GrxFirma.WinUI.Core/Operations/RestServerLaunch.cs` |
| 3 | `GrxFirma.WinUI.Core/Operations/VerificationAssessment.cs` |
| 10 | `GrxFirma.WinUI.Core/Operations/VerificationPresentation.cs` |
| 5 | `GrxFirma.WinUI.Core/Operations/VerificationReport.cs` |
| 1 | `GrxFirma.WinUI.Core/Properties/AssemblyInfo.cs` |
| 75 | `ViewModels/HashPageViewModel.cs` |
| 11 | `ViewModels/HelpPageViewModel.cs` |
| 109 | `ViewModels/ProtectPageViewModel.cs` |
| 369 | `ViewModels/SignPageViewModel.cs` |
| 47 | `ViewModels/VerifyPageViewModel.cs` |
| 1 | `Views/ProtectPage.xaml.cs` |
| 8 | `Views/PublicCertificateExport.cs` |
| 18 | `Views/SignPage.xaml.cs` |

## Verificación

- `python3 -m unittest discover -s cmd/gui-winui/tests -p "test_*.py"`: **211 pruebas, OK (2 expected failures del grupo A)**.
- `python3 -m unittest discover -s cmd/gui-qml/tests -p "test_*.py"`: **127 pruebas, OK**.
- `GOCACHE=/tmp/grxfirma-go-cache GOFLAGS=-buildvcs=false go test ./internal/adapters/outbound/common/localizador/`: **OK**. Se usó una caché escribible porque la caché habitual era de solo lectura.
- `git diff --check`: **sin errores**. Catálogos JSON válidos, mismo conjunto de claves y marcadores de plantilla conservados en los 11 idiomas.
- No hay `dotnet` disponible; los cambios C# no se han compilado.
- Revisión focal `security-audit` del cambio de idioma en credenciales y destinos: sin hallazgos en el código modificado. No se alteraron URLs oficiales ni el tratamiento del secreto.
- No se creó ningún commit.
