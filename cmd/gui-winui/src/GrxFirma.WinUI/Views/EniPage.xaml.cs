// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class EniPage : Page
{
    private readonly App _app;
    private readonly TextBox _docOrgan, _capture, _docState, _docType, _docId, _sourceId, _format;
    private readonly TextBox _fileOrgan, _opened, _classification, _fileState, _fileId, _interested;
    private readonly ComboBox _origin = new(), _certificates = new();
    private readonly TextBlock _signaturePath = new(), _originalPath = new(), _folderPath = new(), _status = new();
    private readonly Button _createDocument = new(), _createFile = new();
    private bool _busy;

    private static string T(string key) => SealUiCatalog.Text(Localizer.Language, key);
    private static TextBox Field(string key, string value = "", int maxLength = 128) => new()
    {
        Header = T(key), Text = value, MaxLength = maxLength, HorizontalAlignment = HorizontalAlignment.Stretch,
    };
    private static Button Action(string key, RoutedEventHandler handler)
    {
        var button = new Button { Content = T(key), MinHeight = 40 };
        button.Click += handler;
        return button;
    }

    public EniPage()
    {
        _app = (App)Application.Current;
        InitializeComponent();
        var now = DateTimeOffset.Now.ToString("yyyy-MM-ddTHH:mm:sszzz", CultureInfo.InvariantCulture);
        _docOrgan = Field("paridad.lote3.eni.organ");
        _capture = Field("paridad.lote3.eni.capture_date", now, 35);
        _docState = Field("paridad.lote3.eni.state", "EE01", 4);
        _docType = Field("paridad.lote3.eni.doc_type", "TD99", 4);
        _docId = Field("paridad.lote3.eni.identifier_optional", maxLength: 80);
        _sourceId = Field("paridad.lote3.eni.source_optional", maxLength: 80);
        _format = Field("paridad.lote3.eni.format_optional", maxLength: 32);
        _fileOrgan = Field("paridad.lote3.eni.organ");
        _opened = Field("paridad.lote3.eni.open_date", now, 35);
        _classification = Field("paridad.lote3.eni.classification", maxLength: 80);
        _fileState = Field("paridad.lote3.eni.file_state", "E01", 3);
        _fileId = Field("paridad.lote3.eni.identifier_optional", maxLength: 80);
        _interested = Field("paridad.lote3.eni.interested_optional", maxLength: 256);

        PageTitle.Text = T("paridad.lote3.eni.title");
        var document = new StackPanel { Spacing = 8 };
        var documentTitle = new TextBlock { Text = T("paridad.lote3.eni.document"), FontSize = 20 };
        AutomationProperties.SetHeadingLevel(documentTitle, AutomationHeadingLevel.Level2);
        document.Children.Add(documentTitle);
        document.Children.Add(Action("paridad.lote3.eni.signature", PickSignature));
        document.Children.Add(_signaturePath);
        document.Children.Add(Action("paridad.lote3.eni.original", PickOriginal));
        document.Children.Add(Action("paridad.lote3.eni.clear_original", (_, _) => _originalPath.Text = string.Empty));
        document.Children.Add(_originalPath);
        document.Children.Add(_docOrgan);
        _origin.Header = T("paridad.lote3.eni.origin");
        _origin.Items.Add(T("paridad.lote3.eni.administration"));
        _origin.Items.Add(T("paridad.lote3.eni.citizen"));
        _origin.SelectedIndex = 0;
        document.Children.Add(_origin);
        foreach (var field in new[] { _capture, _docState, _docType, _docId, _sourceId, _format }) document.Children.Add(field);
        _createDocument.Content = T("paridad.lote3.eni.create_document");
        _createDocument.MinHeight = 40;
        _createDocument.Click += CreateDocument;
        document.Children.Add(_createDocument);
        EniActions.Children.Add(new Border { Child = document, Padding = new Thickness(16), Style = (Style)Application.Current.Resources["AppCardStyle"] });

        var file = new StackPanel { Spacing = 8 };
        var fileTitle = new TextBlock { Text = T("paridad.lote3.eni.file"), FontSize = 20 };
        AutomationProperties.SetHeadingLevel(fileTitle, AutomationHeadingLevel.Level2);
        file.Children.Add(fileTitle);
        file.Children.Add(Action("paridad.lote3.eni.folder", PickFolder));
        file.Children.Add(_folderPath);
        foreach (var field in new[] { _fileOrgan, _opened, _classification, _fileState, _fileId, _interested }) file.Children.Add(field);
        _certificates.Header = T("paridad.lote3.eni.certificate");
        _certificates.DisplayMemberPath = nameof(CertificateInfo.SubjectName);
        file.Children.Add(_certificates);
        _createFile.Content = T("paridad.lote3.eni.create_file");
        _createFile.MinHeight = 40;
        _createFile.Click += CreateFile;
        file.Children.Add(_createFile);
        EniActions.Children.Add(new Border { Child = file, Padding = new Thickness(16), Style = (Style)Application.Current.Resources["AppCardStyle"] });
        _status.TextWrapping = TextWrapping.Wrap;
        AutomationProperties.SetLiveSetting(_status, AutomationLiveSetting.Polite);
        EniActions.Children.Add(_status);
        var tabIndex = 0;
        foreach (var section in new[] { document, file })
        {
            foreach (var child in section.Children)
            {
                if (child is not Control control) continue;
                control.TabIndex = tabIndex++;
                var name = control switch
                {
                    TextBox box => box.Header?.ToString(),
                    ComboBox combo => combo.Header?.ToString(),
                    Button button => button.Content?.ToString(),
                    _ => null,
                };
                if (!string.IsNullOrWhiteSpace(name)) AutomationProperties.SetName(control, name);
            }
        }
        Loaded += async (_, _) => await LoadCertificates();
    }

    private async Task LoadCertificates()
    {
        if (!_app.OperationSession.TryGetOperations(DesktopOperationActions.Certificates, out var operations)) return;
        try
        {
            var result = await operations.GetCertificatesAsync();
            if (result.IsSuccess && result.Data is not null)
            {
                _certificates.ItemsSource = result.Data;
                if (result.Data.Count > 0) _certificates.SelectedIndex = 0;
            }
        }
        catch (Exception) { _status.Text = T("paridad.lote3.eni.certificates_failed"); }
    }

    private async void PickSignature(object sender, RoutedEventArgs args)
    {
        _signaturePath.Text = await _app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument) ?? _signaturePath.Text;
    }
    private async void PickOriginal(object sender, RoutedEventArgs args)
    {
        _originalPath.Text = await _app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument) ?? _originalPath.Text;
    }
    private async void PickFolder(object sender, RoutedEventArgs args)
    {
        _folderPath.Text = await _app.FilePickerService.PickFolderAsync() ?? _folderPath.Text;
    }

    private void SetBusy(bool busy)
    {
        _busy = busy;
        _createDocument.IsEnabled = !busy;
        _createFile.IsEnabled = !busy;
    }

    private async void CreateDocument(object sender, RoutedEventArgs args)
    {
        if (_busy) return;
        if (string.IsNullOrWhiteSpace(_signaturePath.Text) || string.IsNullOrWhiteSpace(_docOrgan.Text))
        { _status.Text = T("paridad.lote3.eni.required"); return; }
        var output = await _app.FilePickerService.PickSaveFileAsync(SaveFilePickerProfile.EniXml);
        if (output is null) return;
        if (!_app.OperationSession.TryGetOperations(DesktopOperationActions.GenerateEniDocument, out var operations))
        { _status.Text = T("paridad.lote3.eni.unavailable"); return; }
        var options = new Dictionary<string, string>
        {
            ["eni.organo"] = _docOrgan.Text.Trim(), ["eni.origen"] = _origin.SelectedIndex == 0 ? "administracion" : "ciudadano",
            ["eni.fechaCaptura"] = _capture.Text.Trim(), ["eni.estado"] = _docState.Text.Trim(),
            ["eni.tipoDocumental"] = _docType.Text.Trim(), ["eni.identificador"] = _docId.Text.Trim(),
            ["eni.documentoOrigen"] = _sourceId.Text.Trim(), ["eni.formato"] = _format.Text.Trim(),
        };
        SetBusy(true);
        _status.Text = T("paridad.lote3.eni.creating");
        try
        {
            var result = await operations.GenerateEniDocumentAsync(_signaturePath.Text, _originalPath.Text, output, options);
            _status.Text = result.IsSuccess && result.Data is not null
                ? T("paridad.lote3.eni.created").Replace("%1", result.Data.OutputPath)
                : T("paridad.lote3.eni.failed").Replace("%1", result.SafeUserMessage);
        }
        catch (Exception) { _status.Text = T("paridad.lote3.eni.retry"); }
        finally { SetBusy(false); }
    }

    private async void CreateFile(object sender, RoutedEventArgs args)
    {
        if (_busy) return;
        if (string.IsNullOrWhiteSpace(_folderPath.Text) || string.IsNullOrWhiteSpace(_fileOrgan.Text) ||
            string.IsNullOrWhiteSpace(_classification.Text) || _certificates.SelectedItem is not CertificateInfo certificate)
        { _status.Text = T("paridad.lote3.eni.required"); return; }
        var output = await _app.FilePickerService.PickSaveFileAsync(SaveFilePickerProfile.EniXml);
        if (output is null) return;
        if (!_app.OperationSession.TryGetOperations(DesktopOperationActions.GenerateEniFile, out var operations))
        { _status.Text = T("paridad.lote3.eni.unavailable"); return; }
        var options = new Dictionary<string, string>
        {
            ["exp.organo"] = _fileOrgan.Text.Trim(), ["exp.fechaApertura"] = _opened.Text.Trim(),
            ["exp.clasificacion"] = _classification.Text.Trim(), ["exp.estado"] = _fileState.Text.Trim(),
            ["exp.identificador"] = _fileId.Text.Trim(), ["exp.interesado"] = _interested.Text.Trim(),
        };
        SetBusy(true);
        _status.Text = T("paridad.lote3.eni.creating");
        try
        {
            var result = await operations.GenerateEniFileAsync(_folderPath.Text, output, certificate.Id, options);
            _status.Text = result.IsSuccess && result.Data is not null
                ? T("paridad.lote3.eni.created").Replace("%1", result.Data.OutputPath)
                : T("paridad.lote3.eni.failed").Replace("%1", result.SafeUserMessage);
        }
        catch (Exception) { _status.Text = T("paridad.lote3.eni.retry"); }
        finally { SetBusy(false); }
    }
}
