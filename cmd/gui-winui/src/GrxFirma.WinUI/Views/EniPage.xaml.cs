// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace GrxFirma.WinUI.Views;

public sealed partial class EniPage : Page
{
    private readonly App _app;
    private readonly TextBox _docOrgan, _docId, _sourceId, _format;
    private readonly TextBox _fileOrgan, _classification, _fileId, _interested;
    private readonly ComboBox _docState, _docType, _fileState;
    private readonly CalendarDatePicker _capture, _opened;
    private readonly TimePicker _captureTime = new(), _openedTime = new();
    private readonly Dictionary<Control, Func<string?>> _validators = new();
    private readonly Dictionary<Control, TextBlock> _messages = new();
    private readonly HashSet<Control> _touched = new();
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
        var now = DateTimeOffset.Now;
        _docOrgan = Field("paridad.lote3.eni.organ");
        _capture = new CalendarDatePicker { Header = T("paridad.lote3.eni.capture_date"), Date = now };
        _captureTime.Time = now.TimeOfDay;
        _docState = Codes("paridad.lote3.eni.state", EniCatalog.EstadosElaboracion, "EE01");
        _docType = Codes("paridad.lote3.eni.doc_type", EniCatalog.TiposDocumentales, "TD99");
        _docId = Field("paridad.lote3.eni.identifier_optional", maxLength: 48);
        _sourceId = Field("paridad.lote3.eni.source_optional", maxLength: 48);
        _format = Field("paridad.lote3.eni.format_optional", maxLength: 32);
        _fileOrgan = Field("paridad.lote3.eni.organ");
        _opened = new CalendarDatePicker { Header = T("paridad.lote3.eni.open_date"), Date = now };
        _openedTime.Time = now.TimeOfDay;
        _classification = Field("paridad.lote3.eni.classification", maxLength: 44);
        _fileState = Codes("paridad.lote3.eni.file_state", EniCatalog.EstadosExpediente, "E01");
        _fileId = Field("paridad.lote3.eni.identifier_optional", maxLength: 48);
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
        AddValidated(document, _docOrgan, () => EniValidation.OrganError(_docOrgan.Text));
        _origin.Header = T("paridad.lote3.eni.origin");
        _origin.Items.Add(T("paridad.lote3.eni.administration"));
        _origin.Items.Add(T("paridad.lote3.eni.citizen"));
        _origin.SelectedIndex = 0;
        document.Children.Add(_origin);
        AddValidated(document, _capture, () => EniValidation.DateError(_capture.Date, _captureTime.Time));
        document.Children.Add(_captureTime);
        document.Children.Add(_docState);
        document.Children.Add(_docType);
        AddValidated(document, _docId, () => EniValidation.IdentifierError(_docId.Text));
        AddValidated(document, _sourceId, () => EniValidation.IdentifierError(_sourceId.Text,
            Code(_docState) is "EE02" or "EE03" or "EE04"));
        AddValidated(document, _format, () => EniValidation.FormatError(_format.Text));
        _docState.SelectionChanged += (_, _) => { _touched.Add(_sourceId); Refresh(_sourceId); };
        _captureTime.TimeChanged += (_, _) => Refresh(_capture);
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
        AddValidated(file, _fileOrgan, () => EniValidation.OrganError(_fileOrgan.Text));
        AddValidated(file, _opened, () => EniValidation.DateError(_opened.Date, _openedTime.Time));
        file.Children.Add(_openedTime);
        AddValidated(file, _classification, () => EniValidation.ClassificationError(_classification.Text));
        file.Children.Add(_fileState);
        AddValidated(file, _fileId, () => EniValidation.IdentifierError(_fileId.Text));
        AddValidated(file, _interested, () => EniValidation.InterestedError(_interested.Text));
        _openedTime.TimeChanged += (_, _) => Refresh(_opened);
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
                    CalendarDatePicker calendar => calendar.Header?.ToString(),
                    TimePicker timePicker => ReferenceEquals(timePicker, _captureTime)
                        ? T("paridad.lote3.eni.capture_date") : T("paridad.lote3.eni.open_date"),
                    Button button => button.Content?.ToString(),
                    _ => null,
                };
                if (!string.IsNullOrWhiteSpace(name)) AutomationProperties.SetName(control, name);
            }
        }
        Loaded += async (_, _) => await LoadCertificates();
    }

    private static ComboBox Codes(string key, IReadOnlyList<string> codes, string selected)
    {
        var combo = new ComboBox { Header = T(key), HorizontalAlignment = HorizontalAlignment.Stretch };
        foreach (var code in codes)
            combo.Items.Add(new ComboBoxItem { Content = code + " — " + T("eni.codigo." + code), Tag = code });
        combo.SelectedIndex = codes.ToList().IndexOf(selected);
        return combo;
    }

    private static string Code(ComboBox combo) => (combo.SelectedItem as ComboBoxItem)?.Tag as string ?? string.Empty;

    private void AddValidated(StackPanel parent, Control field, Func<string?> validate)
    {
        _validators[field] = validate;
        var message = new TextBlock { TextWrapping = TextWrapping.Wrap, Visibility = Visibility.Collapsed,
            Foreground = (Brush)Application.Current.Resources["AppDiagnosticFailureBrush"] };
        AutomationProperties.SetLiveSetting(message, AutomationLiveSetting.Polite);
        _messages[field] = message;
        parent.Children.Add(field);
        parent.Children.Add(message);
        field.LostFocus += (_, _) => { _touched.Add(field); Refresh(field); };
        if (field is TextBox box) box.TextChanged += (_, _) => { if (_touched.Contains(field)) Refresh(field); };
        if (field is CalendarDatePicker date) date.DateChanged += (_, _) => { _touched.Add(field); Refresh(field); };
    }

    private string? Refresh(Control field)
    {
        var key = _validators[field]();
        var detail = key is null ? string.Empty : T(key);
        var message = _messages[field];
        message.Text = detail;
        message.Visibility = key is null ? Visibility.Collapsed : Visibility.Visible;
        AutomationProperties.SetHelpText(field, detail);
        if (key is null) field.ClearValue(Control.BorderBrushProperty);
        else field.BorderBrush = (Brush)Application.Current.Resources["AppDiagnosticFailureBrush"];
        return key;
    }

    private bool ValidateFields(params Control[] fields)
    {
        Control? first = null;
        foreach (var field in fields)
        {
            _touched.Add(field);
            if (Refresh(field) is not null) first ??= field;
        }
        if (first is null) return true;
        first.StartBringIntoView();
        first.Focus(FocusState.Programmatic);
        return false;
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
        if (!ValidateFields(_docOrgan, _capture, _docId, _sourceId, _format)) return;
        if (string.IsNullOrWhiteSpace(_signaturePath.Text))
        { _status.Text = T("paridad.lote3.eni.required"); return; }
        var output = await _app.FilePickerService.PickSaveFileAsync(SaveFilePickerProfile.EniXml);
        if (output is null || !ValidateFields(_docOrgan, _capture, _docId, _sourceId, _format)) return;
        if (!_app.OperationSession.TryGetOperations(DesktopOperationActions.GenerateEniDocument, out var operations))
        { _status.Text = T("paridad.lote3.eni.unavailable"); return; }
        var options = new Dictionary<string, string>
        {
            ["eni.organo"] = _docOrgan.Text.Trim(), ["eni.origen"] = _origin.SelectedIndex == 0 ? "administracion" : "ciudadano",
            ["eni.fechaCaptura"] = EniValidation.SerializeDate(_capture.Date, _captureTime.Time), ["eni.estado"] = Code(_docState),
            ["eni.tipoDocumental"] = Code(_docType), ["eni.identificador"] = _docId.Text.Trim(),
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
        if (!ValidateFields(_fileOrgan, _opened, _classification, _fileId, _interested)) return;
        if (string.IsNullOrWhiteSpace(_folderPath.Text) || _certificates.SelectedItem is not CertificateInfo certificate)
        { _status.Text = T("paridad.lote3.eni.required"); return; }
        var output = await _app.FilePickerService.PickSaveFileAsync(SaveFilePickerProfile.EniXml);
        if (output is null || !ValidateFields(_fileOrgan, _opened, _classification, _fileId, _interested)) return;
        if (!_app.OperationSession.TryGetOperations(DesktopOperationActions.GenerateEniFile, out var operations))
        { _status.Text = T("paridad.lote3.eni.unavailable"); return; }
        var options = new Dictionary<string, string>
        {
            ["exp.organo"] = _fileOrgan.Text.Trim(), ["exp.fechaApertura"] = EniValidation.SerializeDate(_opened.Date, _openedTime.Time),
            ["exp.clasificacion"] = _classification.Text.Trim(), ["exp.estado"] = Code(_fileState),
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
