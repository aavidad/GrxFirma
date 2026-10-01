// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.ViewModels;

public abstract class WorkspacePageViewModel(
    string title,
    string description,
    string disconnectedMessage) : ObservableObject
{
    private readonly string _disconnectedMessage = disconnectedMessage;
    private string _pendingMessage = disconnectedMessage;
    private bool _isOperationConnected;

    public string Title { get; } = title;
    public string Description { get; } = description;

    public string PendingMessage
    {
        get => _pendingMessage;
        private set => SetProperty(ref _pendingMessage, value);
    }

    public bool IsOperationConnected
    {
        get => _isOperationConnected;
        private set
        {
            if (SetProperty(ref _isOperationConnected, value))
            {
                RaisePropertyChanged(nameof(IsAvailabilityNoticeOpen));
            }
        }
    }

    public bool IsAvailabilityNoticeOpen => !IsOperationConnected;

    public void SetOperationAvailability(
        bool isConnected,
        string connectedMessage)
    {
        IsOperationConnected = isConnected;
        PendingMessage = isConnected
            ? connectedMessage
            : _disconnectedMessage;
    }
}
