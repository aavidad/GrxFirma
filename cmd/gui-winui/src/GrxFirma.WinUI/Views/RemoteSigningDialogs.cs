// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

/// <summary>
/// Cuadros de la firma remota CSC. El motor guarda la sesión con el
/// prestador; aquí solo se muestran el estado y los hosts, y se piden el PIN
/// y el OTP en campos de contraseña que se vacían al cerrar.
/// </summary>
internal static class RemoteSigningDialogs
{
    /// <summary>
    /// El botón se muestra si la firma remota está permitida o si la
    /// política la prohíbe (para explicarlo); no, si solo está desactivada.
    /// </summary>
    public static async Task<bool> ShouldShowButtonAsync(
        DesktopOperationSession session,
        CancellationToken cancellationToken)
    {
        if (!session.TryGetOperations(DesktopOperationActions.CscStatus, out var operations))
        {
            return false;
        }
        try
        {
            var status = await operations.GetRemoteSigningStatusAsync(cancellationToken);
            return status.IsSuccess &&
                (status.Data?.Allowed == true || status.Data?.ProhibitedByPolicy == true);
        }
        catch (IpcClientException)
        {
            return false;
        }
    }

    /// <summary>
    /// Configuración y conexión. onChanged se llama cuando cambian los
    /// certificados remotos disponibles (conectar o desconectar), aunque el
    /// cuadro ya se haya cerrado.
    /// </summary>
    public static async Task ShowConfigurationAsync(
        XamlRoot xamlRoot,
        ElementTheme theme,
        DesktopOperationSession session,
        Action onChanged,
        CancellationToken cancellationToken)
    {
        if (!session.TryGetOperations(DesktopOperationActions.CscStatus, out var operations))
        {
            return;
        }

        var description = Paragraph(Localizer.Text("csc.gui.descripcion"));
        var urlBox = new TextBox
        {
            Header = Localizer.Text("csc.gui.url"),
            PlaceholderText = "https://",
            IsSpellCheckEnabled = false,
            MaxLength = RemoteSigningInput.MaximumServiceUrlLength,
        };
        AutomationProperties.SetName(urlBox, Localizer.Text("csc.gui.url"));
        AutomationProperties.SetHelpText(urlBox, Localizer.Text("csc.gui.url_ayuda"));
        var urlHelp = Paragraph(Localizer.Text("csc.gui.url_ayuda"));
        var clientIdBox = new TextBox
        {
            Header = Localizer.Text("csc.gui.client_id"),
            IsSpellCheckEnabled = false,
            MaxLength = RemoteSigningInput.MaximumClientIdLength,
        };
        AutomationProperties.SetName(clientIdBox, Localizer.Text("csc.gui.client_id"));
        var checkButton = new Button { Content = Localizer.Text("csc.gui.comprobar"), MinHeight = 40 };
        AutomationProperties.SetName(checkButton, Localizer.Text("csc.gui.comprobar"));
        var serviceHost = HostLine("csc.gui.host_servicio", out var serviceHostValue);
        var oauthHost = HostLine("csc.gui.host_oauth", out var oauthHostValue);
        var warning = Paragraph(Localizer.Text("csc.gui.aviso_navegador"));
        var connectButton = new Button { Content = Localizer.Text("csc.gui.conectar"), MinHeight = 40 };
        AutomationProperties.SetName(connectButton, Localizer.Text("csc.gui.conectar"));
        var disconnectButton = new Button { Content = Localizer.Text("csc.gui.desconectar"), MinHeight = 40 };
        AutomationProperties.SetName(disconnectButton, Localizer.Text("csc.gui.desconectar"));
        var status = Paragraph(string.Empty);
        AutomationProperties.SetLiveSetting(status, AutomationLiveSetting.Polite);

        var panel = new StackPanel { Spacing = 10, MinWidth = 320, MaxWidth = 520 };
        foreach (var element in new UIElement[]
        {
            description, urlBox, urlHelp, clientIdBox, checkButton,
            serviceHost, oauthHost, warning, connectButton, disconnectButton, status,
        })
        {
            panel.Children.Add(element);
        }

        var busy = false;
        RemoteSigningStatus? current = null;
        RemoteSigningDiscovery? discovery = null;

        void Render()
        {
            var connected = current?.Connected == true;
            var discovered = discovery is not null;
            urlBox.IsEnabled = !busy && !connected;
            clientIdBox.IsEnabled = !busy && !connected;
            checkButton.Visibility = connected ? Visibility.Collapsed : Visibility.Visible;
            checkButton.IsEnabled = !busy &&
                urlBox.Text.Trim().Length > 0 && clientIdBox.Text.Trim().Length > 0;
            serviceHostValue.Text = discovery?.ServiceHost ?? string.Empty;
            oauthHostValue.Text = discovery?.OAuthHost ?? string.Empty;
            serviceHost.Visibility = discovered ? Visibility.Visible : Visibility.Collapsed;
            oauthHost.Visibility = discovered ? Visibility.Visible : Visibility.Collapsed;
            warning.Visibility = discovered && !connected ? Visibility.Visible : Visibility.Collapsed;
            connectButton.Visibility = discovered && !connected ? Visibility.Visible : Visibility.Collapsed;
            connectButton.IsEnabled = !busy;
            disconnectButton.Visibility = connected ? Visibility.Visible : Visibility.Collapsed;
            disconnectButton.IsEnabled = !busy;
            status.Visibility = status.Text.Length > 0 ? Visibility.Visible : Visibility.Collapsed;
        }

        void Say(string text)
        {
            status.Text = text;
            Render();
            if (text.Length > 0 &&
                FrameworkElementAutomationPeer.FromElement(status) is { } peer)
            {
                peer.RaiseAutomationEvent(AutomationEvents.LiveRegionChanged);
            }
        }

        async Task RefreshStatusAsync()
        {
            var result = await operations.GetRemoteSigningStatusAsync(cancellationToken);
            current = result.IsSuccess ? result.Data : null;
            discovery = current is { Discovered: true }
                ? new RemoteSigningDiscovery
                {
                    ServiceHost = current.ServiceHost,
                    OAuthHost = current.OAuthHost,
                    ServiceName = current.ServiceName,
                }
                : null;
            Render();
        }

        urlBox.TextChanged += (_, _) => { discovery = null; Render(); };
        clientIdBox.TextChanged += (_, _) => { discovery = null; Render(); };

        checkButton.Click += async (_, _) =>
        {
            busy = true;
            discovery = null;
            Say(Localizer.Text("csc.gui.comprobando"));
            try
            {
                var result = await operations.ConfigureRemoteSigningAsync(
                    urlBox.Text, clientIdBox.Text, cancellationToken);
                busy = false;
                if (result.IsSuccess && result.Data is not null)
                {
                    discovery = result.Data;
                    Say(string.Empty);
                }
                else
                {
                    Say(FailureText(result.ErrorCode));
                }
            }
            catch (ArgumentException exception)
            {
                busy = false;
                Say(Localizer.Text(exception.Message.Split(' ')[0]));
            }
            catch (IpcClientException)
            {
                busy = false;
                Say(Localizer.Text("csc.error.generico"));
            }
            catch (OperationCanceledException)
            {
                busy = false;
            }
        };

        connectButton.Click += async (_, _) =>
        {
            busy = true;
            Say(Localizer.Text("csc.gui.conectando"));
            try
            {
                var result = await operations.ConnectRemoteSigningAsync(cancellationToken);
                busy = false;
                if (result.IsSuccess && result.Data is not null)
                {
                    var text = result.Data.Credentials.Count == 0
                        ? Localizer.Text("csc.gui.sin_credenciales")
                        : Localizer.Text("csc.gui.conectada");
                    if (result.Data.Omitted > 0)
                    {
                        text += " " + Localizer.Text("csc.gui.omitidas");
                    }
                    await RefreshStatusAsync();
                    Say(text);
                    onChanged();
                }
                else
                {
                    Say(FailureText(result.ErrorCode));
                    await RefreshStatusAsync();
                }
            }
            catch (IpcClientException)
            {
                busy = false;
                Say(Localizer.Text("csc.error.generico"));
            }
            catch (OperationCanceledException)
            {
                busy = false;
            }
        };

        disconnectButton.Click += async (_, _) =>
        {
            busy = true;
            Render();
            try
            {
                var result = await operations.DisconnectRemoteSigningAsync(cancellationToken);
                busy = false;
                await RefreshStatusAsync();
                Say(result.IsSuccess ? string.Empty : FailureText(result.ErrorCode));
                onChanged();
            }
            catch (IpcClientException)
            {
                busy = false;
                Say(Localizer.Text("csc.error.generico"));
            }
            catch (OperationCanceledException)
            {
                busy = false;
            }
        };

        var dialog = new ContentDialog
        {
            XamlRoot = xamlRoot,
            RequestedTheme = theme,
            Title = Localizer.Text("csc.gui.titulo"),
            Content = new ScrollViewer { Content = panel },
            CloseButtonText = Localizer.Text("winui.comun.cerrar"),
            DefaultButton = ContentDialogButton.Close,
        };

        try
        {
            await RefreshStatusAsync();
            if (current is { ProhibitedByPolicy: true })
            {
                await ShowProhibitedAsync(xamlRoot, theme);
                return;
            }
            if (current is not { Allowed: true })
            {
                return;
            }
            urlBox.Text = current.ServiceUrl;
            clientIdBox.Text = current.ClientId;
            await RefreshStatusAsync();
        }
        catch (IpcClientException)
        {
            return;
        }
        await Localizer.ShowAsync(dialog);
    }

    /// <summary>
    /// Pide el PIN y el OTP de un certificado remoto. Devuelve null si la
    /// persona cancela. Los campos se vacían siempre al cerrar.
    /// </summary>
    public static async Task<RemoteSigningSecrets?> PromptSecretsAsync(
        XamlRoot xamlRoot,
        ElementTheme theme,
        DesktopOperationSession session,
        CertificateInfo certificate,
        CancellationToken cancellationToken)
    {
        var explanation = Paragraph(Localizer.Text("csc.gui.dialogo_texto"));
        var pinBox = new PasswordBox
        {
            Header = Localizer.Text("csc.gui.pin"),
            MaxLength = RemoteSigningInput.MaximumSecretBytes,
            Visibility = certificate.RemotePin ? Visibility.Visible : Visibility.Collapsed,
        };
        AutomationProperties.SetName(pinBox, Localizer.Text("csc.gui.pin"));
        var otpBox = new PasswordBox
        {
            Header = Localizer.Text("csc.gui.otp"),
            MaxLength = RemoteSigningInput.MaximumSecretBytes,
            Visibility = certificate.RemoteOtp ? Visibility.Visible : Visibility.Collapsed,
        };
        AutomationProperties.SetName(otpBox, Localizer.Text("csc.gui.otp"));
        var sendCodeButton = new Button
        {
            Content = Localizer.Text("csc.gui.enviar_codigo"),
            MinHeight = 40,
            Visibility = certificate.RemoteOtp && certificate.RemoteOtpOnline
                ? Visibility.Visible
                : Visibility.Collapsed,
        };
        AutomationProperties.SetName(sendCodeButton, Localizer.Text("csc.gui.enviar_codigo"));
        var status = Paragraph(string.Empty);
        status.Visibility = Visibility.Collapsed;
        AutomationProperties.SetLiveSetting(status, AutomationLiveSetting.Assertive);

        void Say(string text)
        {
            status.Text = text;
            status.Visibility = text.Length > 0 ? Visibility.Visible : Visibility.Collapsed;
            if (text.Length > 0 &&
                FrameworkElementAutomationPeer.FromElement(status) is { } peer)
            {
                peer.RaiseAutomationEvent(AutomationEvents.LiveRegionChanged);
            }
        }

        sendCodeButton.Click += async (_, _) =>
        {
            if (!session.TryGetOperations(DesktopOperationActions.CscSendOtp, out var operations))
            {
                Say(Localizer.Text("csc.error.no_conectada"));
                return;
            }
            sendCodeButton.IsEnabled = false;
            try
            {
                var result = await operations.SendRemoteSigningOtpAsync(certificate.Id, cancellationToken);
                Say(result.IsSuccess
                    ? Localizer.Text("csc.gui.codigo_enviado")
                    : FailureText(result.ErrorCode));
            }
            catch (ArgumentException)
            {
                Say(Localizer.Text("csc.error.parametro_invalido"));
            }
            catch (IpcClientException)
            {
                Say(Localizer.Text("csc.error.generico"));
            }
            catch (OperationCanceledException)
            {
            }
            finally
            {
                sendCodeButton.IsEnabled = true;
            }
        };

        var panel = new StackPanel { Spacing = 10, MinWidth = 300, MaxWidth = 460 };
        panel.Children.Add(explanation);
        panel.Children.Add(pinBox);
        panel.Children.Add(otpBox);
        panel.Children.Add(sendCodeButton);
        panel.Children.Add(status);

        var dialog = new ContentDialog
        {
            XamlRoot = xamlRoot,
            RequestedTheme = theme,
            Title = Localizer.Text("csc.gui.dialogo_titulo"),
            Content = panel,
            PrimaryButtonText = Localizer.Text("csc.gui.firmar"),
            CloseButtonText = Localizer.Text("csc.gui.cancelar"),
            DefaultButton = ContentDialogButton.Primary,
        };
        dialog.PrimaryButtonClick += (_, args) =>
        {
            if ((certificate.RemotePin && pinBox.Password.Length == 0) ||
                (certificate.RemoteOtp && otpBox.Password.Length == 0))
            {
                args.Cancel = true;
                Say(Localizer.Text("csc.gui.falta_dato"));
            }
        };
        dialog.Opened += (_, _) =>
        {
            if (certificate.RemotePin) pinBox.Focus(FocusState.Programmatic);
            else otpBox.Focus(FocusState.Programmatic);
        };

        try
        {
            var result = await Localizer.ShowAsync(dialog);
            if (result != ContentDialogResult.Primary || cancellationToken.IsCancellationRequested)
            {
                return null;
            }
            var pin = certificate.RemotePin ? Encoding.UTF8.GetBytes(pinBox.Password) : null;
            var otp = certificate.RemoteOtp ? Encoding.UTF8.GetBytes(otpBox.Password) : null;
            return new RemoteSigningSecrets(pin, otp);
        }
        finally
        {
            // El string de PasswordBox no se puede borrar; al menos no queda
            // ninguna referencia viva en el control.
            pinBox.Password = string.Empty;
            otpBox.Password = string.Empty;
        }
    }

    private static async Task ShowProhibitedAsync(XamlRoot xamlRoot, ElementTheme theme)
    {
        var message = Paragraph(Localizer.Text("csc.error.prohibida"));
        AutomationProperties.SetLiveSetting(message, AutomationLiveSetting.Assertive);
        var dialog = new ContentDialog
        {
            XamlRoot = xamlRoot,
            RequestedTheme = theme,
            Title = Localizer.Text("csc.gui.titulo"),
            Content = message,
            CloseButtonText = Localizer.Text("winui.comun.cerrar"),
            DefaultButton = ContentDialogButton.Close,
        };
        await Localizer.ShowAsync(dialog);
    }

    private static string FailureText(string? errorCode) =>
        Localizer.Text(RemoteSigningInput.MessageKey(errorCode) ?? "csc.error.generico");

    private static TextBlock Paragraph(string text) =>
        new() { Text = text, TextWrapping = TextWrapping.Wrap, IsTextSelectionEnabled = false };

    private static StackPanel HostLine(string labelKey, out TextBlock value)
    {
        value = new TextBlock
        {
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            TextWrapping = TextWrapping.WrapWholeWords,
            IsTextSelectionEnabled = true,
        };
        var line = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 6, Visibility = Visibility.Collapsed };
        line.Children.Add(new TextBlock { Text = Localizer.Text(labelKey) });
        line.Children.Add(value);
        return line;
    }
}
