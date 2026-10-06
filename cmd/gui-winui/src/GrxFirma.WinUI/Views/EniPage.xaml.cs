// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Controls;
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
    // DatePicker (día, mes y año en desplegables) se maneja con el teclado como
    // el selector de hora; el calendario abría con el foco en el encabezado.
    private readonly DatePicker _capture, _opened;
    private readonly TimePicker _captureTime = new(), _openedTime = new();
    private readonly Dictionary<Control, Func<string?>> _validators = new();
    private readonly Dictionary<Control, TextBlock> _messages = new();
    private readonly HashSet<Control> _touched = new();
    private readonly ComboBox _origin = new(), _certificates = new();
    private readonly TextBlock _signaturePath = new(), _originalPath = new(), _folderPath = new(), _status = new();
    private readonly TextBlock _documentMessage = new(), _fileMessage = new();
    private readonly Button _createDocument = new(), _createFile = new();
    private Button? _pickSignature, _pickFolder;
    // Rutas elegidas; los TextBlock muestran la ruta o el estado vacío.
    private string _signatureFile = string.Empty, _originalFile = string.Empty, _folder = string.Empty;
    private bool _busy;

    private static string T(string key) => SealUiCatalog.Text(Localizer.Language, key);
    private static TextBox Field(string key, string value = "", int maxLength = 128) => new()
    {
        Header = T(key), Text = value, MaxLength = maxLength, HorizontalAlignment = HorizontalAlignment.Stretch,
    };
    // El texto de los botones, títulos y ayudas se ajusta al ancho: a 1280 px
    // la página se cortaba por la derecha sin poder desplazarse (WCAG 1.4.10).
    private static TextBlock Wrapped(string text) => new() { Text = text, TextWrapping = TextWrapping.Wrap };
    private static Button Action(string key, RoutedEventHandler handler)
    {
        var button = new Button { Content = Wrapped(T(key)), MinHeight = 40 };
        button.Click += handler;
        return button;
    }
    // Botón «?» junto a un control: el control conserva el ancho disponible.
    // topicKey es la etiqueta visible del control y forma el nombre accesible.
    private static HelpRow WithHelp(FrameworkElement main, string topicKey, string helpKey,
        VerticalAlignment alignment = VerticalAlignment.Bottom, double top = 0)
    {
        var row = new HelpRow();
        row.Children.Add(main);
        row.Children.Add(new HelpButton
        {
            Topic = topicKey, HelpKey = helpKey, VerticalAlignment = alignment, Margin = new Thickness(0, top, 0, 0),
        });
        return row;
    }
    private static string? ButtonText(Button button) =>
        (button.Content as TextBlock)?.Text ?? button.Content?.ToString();

    public EniPage()
    {
        _app = (App)Application.Current;
        InitializeComponent();
        var now = DateTimeOffset.Now;
        _docOrgan = Field("paridad.lote3.eni.organ_document");
        _docOrgan.Description = Wrapped(T("paridad.lote3.eni.organ_hint"));
        // El idioma de la aplicación también para mes y día en caliente.
        var controlLanguage = AppCultureTag();
        _capture = new DatePicker { Header = T("paridad.lote3.eni.capture_date"), SelectedDate = now, Language = controlLanguage };
        _captureTime.Language = controlLanguage;
        _openedTime.Language = controlLanguage;
        _captureTime.Time = now.TimeOfDay;
        _captureTime.Header = T("paridad.lote3.eni.capture_time");
        _docState = Codes("paridad.lote3.eni.state", EniCatalog.EstadosElaboracion, "EE01");
        _docType = Codes("paridad.lote3.eni.doc_type", EniCatalog.TiposDocumentales, "TD99");
        _docId = Field("paridad.lote3.eni.identifier_optional", maxLength: 48);
        _sourceId = Field("paridad.lote3.eni.source_optional", maxLength: 48);
        _format = Field("paridad.lote3.eni.format_optional", maxLength: 32);
        _fileOrgan = Field("paridad.lote3.eni.organ_file");
        _fileOrgan.Description = Wrapped(T("paridad.lote3.eni.organ_hint"));
        _opened = new DatePicker { Header = T("paridad.lote3.eni.open_date"), SelectedDate = now, Language = controlLanguage };
        _openedTime.Time = now.TimeOfDay;
        _openedTime.Header = T("paridad.lote3.eni.open_time");
        _classification = Field("paridad.lote3.eni.classification", maxLength: 44);
        _fileState = Codes("paridad.lote3.eni.file_state", EniCatalog.EstadosExpediente, "E01");
        _fileId = Field("paridad.lote3.eni.identifier_file", maxLength: 48);
        _interested = Field("paridad.lote3.eni.interested_optional", maxLength: 256);

        PageTitle.Text = T("paridad.lote3.eni.title");
        var document = new FitWidthStackPanel { Spacing = 8 };
        var documentTitle = new TextBlock { Text = T("paridad.lote3.eni.document"), FontSize = 20, TextWrapping = TextWrapping.Wrap, HorizontalAlignment = HorizontalAlignment.Left };
        AutomationProperties.SetHeadingLevel(documentTitle, AutomationHeadingLevel.Level2);
        document.Children.Add(WithHelp(documentTitle, "paridad.lote3.eni.document", "ayuda.eni.documento", VerticalAlignment.Center));
        _pickSignature = Action("paridad.lote3.eni.signature", PickSignature);
        document.Children.Add(_pickSignature);
        document.Children.Add(_signaturePath);
        document.Children.Add(Action("paridad.lote3.eni.original", PickOriginal));
        document.Children.Add(Action("paridad.lote3.eni.clear_original", (_, _) => ShowPath(_originalPath, _originalFile = string.Empty, "paridad.lote3.eni.no_file")));
        document.Children.Add(_originalPath);
        ShowPath(_signaturePath, _signatureFile, "paridad.lote3.eni.no_file");
        ShowPath(_originalPath, _originalFile, "paridad.lote3.eni.no_file");
        // Con descripción debajo, el «?» se alinea con la caja y no con el pie.
        AddValidated(document, _docOrgan, () => EniValidation.OrganError(_docOrgan.Text),
            WithHelp(_docOrgan, "paridad.lote3.eni.organ_document", "ayuda.eni.metadatos", VerticalAlignment.Top, top: 24));
        _origin.Header = T("paridad.lote3.eni.origin");
        _origin.HorizontalAlignment = HorizontalAlignment.Stretch;
        _origin.MinWidth = 0;
        _origin.Items.Add(T("paridad.lote3.eni.administration"));
        _origin.Items.Add(T("paridad.lote3.eni.citizen"));
        _origin.SelectedIndex = 0;
        document.Children.Add(WithHelp(_origin, "paridad.lote3.eni.origin", "ayuda.eni.origen"));
        AddValidated(document, _capture, () => EniValidation.DateError(_capture.SelectedDate, _captureTime.Time));
        document.Children.Add(_captureTime);
        document.Children.Add(WithHelp(_docState, "paridad.lote3.eni.state", "ayuda.eni.estado_elaboracion"));
        document.Children.Add(_docType);
        AddValidated(document, _docId, () => EniValidation.IdentifierError(_docId.Text));
        AddValidated(document, _sourceId, () => EniValidation.IdentifierError(_sourceId.Text,
            Code(_docState) is "EE02" or "EE03" or "EE04"));
        AddValidated(document, _format, () => EniValidation.FormatError(_format.Text));
        _docState.SelectionChanged += (_, _) => { _touched.Add(_sourceId); Refresh(_sourceId); };
        _captureTime.TimeChanged += (_, _) => Refresh(_capture);
        _createDocument.Content = Wrapped(T("paridad.lote3.eni.create_document"));
        _createDocument.MinHeight = 40;
        _createDocument.Click += CreateDocument;
        document.Children.Add(_createDocument);
        document.Children.Add(Message(_documentMessage));
        EniActions.Children.Add(new Border { Child = document, Padding = new Thickness(16), Style = (Style)Application.Current.Resources["AppCardStyle"] });

        var file = new FitWidthStackPanel { Spacing = 8 };
        var fileTitle = new TextBlock { Text = T("paridad.lote3.eni.file"), FontSize = 20, TextWrapping = TextWrapping.Wrap, HorizontalAlignment = HorizontalAlignment.Left };
        AutomationProperties.SetHeadingLevel(fileTitle, AutomationHeadingLevel.Level2);
        file.Children.Add(WithHelp(fileTitle, "paridad.lote3.eni.file", "ayuda.eni.expediente", VerticalAlignment.Center));
        _pickFolder = Action("paridad.lote3.eni.folder", PickFolder);
        file.Children.Add(_pickFolder);
        file.Children.Add(_folderPath);
        ShowPath(_folderPath, _folder, "paridad.lote3.eni.no_folder");
        AddValidated(file, _fileOrgan, () => EniValidation.OrganError(_fileOrgan.Text));
        AddValidated(file, _opened, () => EniValidation.DateError(_opened.SelectedDate, _openedTime.Time));
        file.Children.Add(_openedTime);
        AddValidated(file, _classification, () => EniValidation.ClassificationError(_classification.Text));
        file.Children.Add(_fileState);
        AddValidated(file, _fileId, () => EniValidation.IdentifierError(_fileId.Text));
        AddValidated(file, _interested, () => EniValidation.InterestedError(_interested.Text));
        _openedTime.TimeChanged += (_, _) => Refresh(_opened);
        _certificates.Header = T("paridad.lote3.eni.certificate");
        _certificates.DisplayMemberPath = nameof(CertificateInfo.SubjectName);
        _certificates.HorizontalAlignment = HorizontalAlignment.Stretch;
        _certificates.MinWidth = 0;
        file.Children.Add(_certificates);
        _createFile.Content = Wrapped(T("paridad.lote3.eni.create_file"));
        _createFile.MinHeight = 40;
        _createFile.Click += CreateFile;
        file.Children.Add(_createFile);
        file.Children.Add(Message(_fileMessage));
        EniActions.Children.Add(new Border { Child = file, Padding = new Thickness(16), Style = (Style)Application.Current.Resources["AppCardStyle"] });
        _status.TextWrapping = TextWrapping.Wrap;
        AutomationProperties.SetLiveSetting(_status, AutomationLiveSetting.Polite);
        EniActions.Children.Add(_status);
        var tabIndex = 0;
        foreach (var section in new[] { document, file })
        {
            // Los controles con «?» van dentro de una HelpRow: primero el
            // control y después su botón de ayuda.
            var controls = section.Children.SelectMany(child => child is HelpRow row
                ? row.Children.AsEnumerable()
                : new[] { child });
            foreach (var child in controls)
            {
                if (child is not Control control) continue;
                control.TabIndex = tabIndex++;
                var name = control switch
                {
                    // HelpButton forma su propio nombre accesible.
                    HelpButton => null,
                    TextBox box => box.Header?.ToString(),
                    ComboBox combo => combo.Header?.ToString(),
                    DatePicker datePicker => datePicker.Header?.ToString(),
                    TimePicker timePicker => timePicker.Header?.ToString(),
                    Button button => ButtonText(button),
                    _ => null,
                };
                if (!string.IsNullOrWhiteSpace(name)) AutomationProperties.SetName(control, name);
            }
        }
        foreach (var datePicker in new[] { _capture, _opened }) PickerLanguage.Attach(datePicker);
        foreach (var timePicker in new[] { _captureTime, _openedTime }) PickerLanguage.Attach(timePicker);
        Loaded += async (_, _) => await LoadCertificates();
    }

    private static string AppCultureTag() =>
        GrxFirma.WinUI.Core.Localization.AppCulture.Tag(Localizer.Language);

    private static ComboBox Codes(string key, IReadOnlyList<string> codes, string selected)
    {
        var combo = new ComboBox { Header = T(key), HorizontalAlignment = HorizontalAlignment.Stretch, MinWidth = 0 };
        foreach (var code in codes)
            // Primero el nombre y después el código NTI: «Otros (TD99)».
            combo.Items.Add(new ComboBoxItem { Content = T("eni.codigo." + code) + " (" + code + ")", Tag = code });
        combo.SelectedIndex = codes.ToList().IndexOf(selected);
        return combo;
    }

    private static TextBlock Message(TextBlock message)
    {
        message.TextWrapping = TextWrapping.Wrap;
        message.Visibility = Visibility.Collapsed;
        AutomationProperties.SetLiveSetting(message, AutomationLiveSetting.Polite);
        return message;
    }

    private static void ShowMessage(TextBlock message, string text)
    {
        message.Text = text;
        message.Visibility = text.Length == 0 ? Visibility.Collapsed : Visibility.Visible;
    }

    // El resultado aparece junto al botón que lo produjo, a la vista, y recibe
    // el foco para que el lector lo lea. Se aplaza: al cerrarse el diálogo de
    // guardar, Windows devuelve el foco a la ventana y se perdía.
    private void ShowResult(TextBlock message, Button button, string text)
    {
        _status.Text = string.Empty;
        ShowMessage(message, text);
        message.IsTextSelectionEnabled = true;
        DispatcherQueue.TryEnqueue(Microsoft.UI.Dispatching.DispatcherQueuePriority.Low, () =>
        {
            if (XamlRoot is null || message.Visibility != Visibility.Visible) return;
            message.StartBringIntoView(new BringIntoViewOptions { AnimationDesired = false });
            if (!message.Focus(FocusState.Programmatic)) button.Focus(FocusState.Programmatic);
            FrameworkElementAutomationPeer.FromElement(message)?
                .RaiseAutomationEvent(AutomationEvents.LiveRegionChanged);
        });
    }

    private static void ShowPath(TextBlock label, string path, string emptyKey)
    {
        label.TextWrapping = TextWrapping.Wrap;
        label.Text = path.Length > 0 ? path : T(emptyKey);
        if (path.Length > 0) label.ClearValue(TextBlock.ForegroundProperty);
        else label.Foreground = (Brush)Application.Current.Resources["AppMutedTextBrush"];
    }

    private static string Code(ComboBox combo) => (combo.SelectedItem as ComboBoxItem)?.Tag as string ?? string.Empty;

    private void AddValidated(Panel parent, Control field, Func<string?> validate,
        UIElement? container = null)
    {
        _validators[field] = validate;
        var message = new TextBlock { TextWrapping = TextWrapping.Wrap, Visibility = Visibility.Collapsed,
            Foreground = (Brush)Application.Current.Resources["AppDiagnosticFailureBrush"] };
        AutomationProperties.SetLiveSetting(message, AutomationLiveSetting.Polite);
        _messages[field] = message;
        parent.Children.Add(container ?? field);
        parent.Children.Add(message);
        field.LostFocus += (_, _) => { _touched.Add(field); Refresh(field); };
        if (field is TextBox box) box.TextChanged += (_, _) => { if (_touched.Contains(field)) Refresh(field); };
        if (field is DatePicker date) date.SelectedDateChanged += (_, _) => { _touched.Add(field); Refresh(field); };
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
        _signatureFile = await _app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument) ?? _signatureFile;
        ShowPath(_signaturePath, _signatureFile, "paridad.lote3.eni.no_file");
        if (_signatureFile.Length > 0) ShowMessage(_documentMessage, string.Empty);
    }
    private async void PickOriginal(object sender, RoutedEventArgs args)
    {
        _originalFile = await _app.FilePickerService.PickOpenFileAsync(OpenFilePickerProfile.SignedOrOriginalDocument) ?? _originalFile;
        ShowPath(_originalPath, _originalFile, "paridad.lote3.eni.no_file");
    }
    private async void PickFolder(object sender, RoutedEventArgs args)
    {
        _folder = await _app.FilePickerService.PickFolderAsync() ?? _folder;
        ShowPath(_folderPath, _folder, "paridad.lote3.eni.no_folder");
        if (_folder.Length > 0) ShowMessage(_fileMessage, string.Empty);
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
        // Lo que falta se explica junto al botón y el foco va a la acción que lo resuelve.
        if (string.IsNullOrWhiteSpace(_signatureFile))
        {
            ShowMessage(_documentMessage, T("paridad.lote3.eni.required_signature"));
            _pickSignature?.Focus(FocusState.Programmatic);
            return;
        }
        ShowMessage(_documentMessage, string.Empty);
        if (!ValidateFields(_docOrgan, _capture, _docId, _sourceId, _format)) return;
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
            var result = await operations.GenerateEniDocumentAsync(_signatureFile, _originalFile, output, options, overwriteConfirmed: true);
            ShowResult(_documentMessage, _createDocument, result.IsSuccess && result.Data is not null
                ? T("paridad.lote3.eni.created").Replace("%1", result.Data.OutputPath)
                : T("paridad.lote3.eni.failed").Replace("%1", result.SafeUserMessage));
        }
        catch (Exception) { ShowResult(_documentMessage, _createDocument, T("paridad.lote3.eni.retry")); }
        finally { SetBusy(false); }
    }

    private async void CreateFile(object sender, RoutedEventArgs args)
    {
        if (_busy) return;
        if (string.IsNullOrWhiteSpace(_folder))
        {
            ShowMessage(_fileMessage, T("paridad.lote3.eni.required_folder"));
            _pickFolder?.Focus(FocusState.Programmatic);
            return;
        }
        if (_certificates.SelectedItem is not CertificateInfo certificate)
        {
            ShowMessage(_fileMessage, T("paridad.lote3.eni.required_certificate"));
            _certificates.Focus(FocusState.Programmatic);
            return;
        }
        ShowMessage(_fileMessage, string.Empty);
        if (!ValidateFields(_fileOrgan, _opened, _classification, _fileId, _interested)) return;
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
            var result = await operations.GenerateEniFileAsync(_folder, output, certificate.Id, options, overwriteConfirmed: true);
            ShowResult(_fileMessage, _createFile, result.IsSuccess && result.Data is not null
                ? T("paridad.lote3.eni.created").Replace("%1", result.Data.OutputPath)
                : T("paridad.lote3.eni.failed").Replace("%1", result.SafeUserMessage));
        }
        catch (Exception) { ShowResult(_fileMessage, _createFile, T("paridad.lote3.eni.retry")); }
        finally { SetBusy(false); }
    }
}
