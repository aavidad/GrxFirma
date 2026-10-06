// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;
using System.Globalization;
using WinRT.Interop;

namespace GrxFirma.WinUI.Services;

internal sealed class WindowsTrayIcon : IDisposable
{
    private const uint CallbackMessage = 0x8001;
    private const uint LeftDoubleClick = 0x0203;
    private const uint RightButtonUp = 0x0205;
    private const uint BalloonUserClick = 0x0405;
    private const uint ContextMenu = 0x007B;
    private const uint Add = 0;
    private const uint Modify = 1;
    private const uint Delete = 2;
    private const uint SetVersion = 4;
    private const uint MessageFlag = 1;
    private const uint IconFlag = 2;
    private const uint TipFlag = 4;
    private const uint InfoFlag = 16;
    private const uint IconVersion4 = 4;
    private const uint LoadFromFile = 0x10;
    private const uint DefaultSize = 0x40;
    private const uint MenuString = 0;
    private const uint MenuDisabled = 2;
    private const uint MenuSeparator = 0x800;
    private const uint MenuPopup = 0x10;
    private const uint ReturnCommand = 0x100;
    private const uint RightButton = 2;
    private const nuint SubclassId = 1;
    private const uint IconId = 1;

    private readonly nint _hwnd;
    private readonly nint _icon;
    private readonly uint _taskbarCreatedMessage;
    private readonly SubclassProc _subclass;
    private readonly Action _open;
    private readonly Action _settings;
    private readonly Action _help;
    private readonly Action _releaseNotes;
    private readonly Action _about;
    private readonly Action _pendingReleaseNotes;
    private readonly Action _exit;
    private bool _installed;
    private bool _releaseNotesNotificationPending;
    private bool _updateNotificationPending;
    private bool _subclassInstalled;

    public WindowsTrayIcon(
        Microsoft.UI.Xaml.Window window,
        Action open,
        Action settings,
        Action help,
        Action releaseNotes,
        Action about,
        Action pendingReleaseNotes,
        Action exit)
    {
        _hwnd = WindowNative.GetWindowHandle(window);
        _taskbarCreatedMessage = RegisterWindowMessage("TaskbarCreated");
        _open = open;
        _settings = settings;
        _help = help;
        _releaseNotes = releaseNotes;
        _about = about;
        _pendingReleaseNotes = pendingReleaseNotes;
        _exit = exit;
        _subclass = WindowProc;
        var iconPath = Path.Combine(AppContext.BaseDirectory,
            "Assets", "grxfirma-grx.ico");
        _icon = LoadImage(0, iconPath, 1, 0, 0,
            LoadFromFile | DefaultSize);
        if (_icon == 0)
        {
            return;
        }

        if (!SetWindowSubclass(_hwnd, _subclass, SubclassId, 0))
        {
            return;
        }
        _subclassInstalled = true;

        var data = CreateData(MessageFlag | IconFlag | TipFlag);
        if (!ShellNotifyIcon(Add, ref data))
        {
            _ = RemoveWindowSubclass(_hwnd, _subclass, SubclassId);
            _subclassInstalled = false;
            return;
        }
        _installed = true;
        data.VersionOrTimeout = IconVersion4;
        _ = ShellNotifyIcon(SetVersion, ref data);
    }

    public bool Installed => _installed;

    public void RefreshLanguage()
    {
        if (!_installed) return;
        var data = CreateData(TipFlag);
        _ = ShellNotifyIcon(Modify, ref data);
    }

    public void ExplainFirstHide()
    {
        if (!_installed)
        {
            return;
        }
        _releaseNotesNotificationPending = false;
        var data = CreateData(InfoFlag);
        data.InfoTitle = Localizer.Text("winui.bandeja.grxfirma_sigue_disponible");
        data.Info = Localizer.Text("winui.bandeja.se_ha_ocultado_en_la_bandeja_abra_su");
        _ = ShellNotifyIcon(Modify, ref data);
    }

    public void ShowReleaseNotesNotification(string message)
    {
        if (!_installed) return;
        _releaseNotesNotificationPending = true;
        var data = CreateData(InfoFlag);
        data.InfoTitle = "GrxFirma";
        data.Info = message;
        _ = ShellNotifyIcon(Modify, ref data);
    }

    public void ShowUpdateNotification(string message)
    {
        if (!_installed) return;
        _updateNotificationPending = true;
        _releaseNotesNotificationPending = false;
        var data = CreateData(InfoFlag);
        data.InfoTitle = "GrxFirma";
        data.Info = message;
        _ = ShellNotifyIcon(Modify, ref data);
    }

    private NotifyIconData CreateData(uint flags) => new()
    {
        Size = (uint)Marshal.SizeOf<NotifyIconData>(),
        Window = _hwnd,
        Id = IconId,
        Flags = flags,
        CallbackMessage = CallbackMessage,
        Icon = _icon,
        Tip = Localizer.Text("winui.bandeja.grxfirma_firmas_desde_portales_activo"),
        Info = string.Empty,
        InfoTitle = string.Empty,
    };

    private nint WindowProc(
        nint hwnd, uint message, nint wParam, nint lParam,
        nuint subclassId, nint reference)
    {
        if (_installed && _taskbarCreatedMessage != 0 &&
            message == _taskbarCreatedMessage)
        {
            var data = CreateData(MessageFlag | IconFlag | TipFlag);
            if (ShellNotifyIcon(Add, ref data))
            {
                data.VersionOrTimeout = IconVersion4;
                _ = ShellNotifyIcon(SetVersion, ref data);
            }
            else
            {
                _installed = false;
                _open();
            }
            return 0;
        }
        if (message == CallbackMessage)
        {
            var eventCode = (uint)(lParam.ToInt64() & 0xFFFF);
            if (eventCode == LeftDoubleClick)
            {
                _open();
            }
            else if (eventCode == BalloonUserClick)
            {
                if (_updateNotificationPending)
                {
                    _updateNotificationPending = false;
                    _open();
                }
                else if (_releaseNotesNotificationPending)
                {
                    _releaseNotesNotificationPending = false;
                    _pendingReleaseNotes();
                }
            }
            else if (eventCode is RightButtonUp or ContextMenu)
            {
                ShowMenu();
            }
            return 0;
        }
        return DefSubclassProc(hwnd, message, wParam, lParam);
    }

    private void ShowMenu()
    {
        var menu = CreatePopupMenu();
        if (menu == 0)
        {
            return;
        }
        try
        {
            var language = Localizer.Language;
            string Label(string key) => SealUiCatalog.Text(language, key);
            _ = AppendMenu(menu, MenuString, 1, Label("winui.bandeja.abrir_grxfirma"));
            _ = AppendMenu(menu, MenuString | MenuDisabled, 2,
                Label("winui.bandeja.firmas_desde_portales_activo"));
            _ = AppendMenu(menu, MenuString, 3, Label("winui.bandeja.ajustes"));
            _ = AppendMenu(menu, MenuSeparator, 0, null);
            var helpMenu = CreatePopupMenu();
            if (helpMenu != 0)
            {
                _ = AppendMenu(helpMenu, MenuString, 5, Label("winui.bandeja.manual_de_ayuda"));
                _ = AppendMenu(helpMenu, MenuString, 7, Label("winui.comun.novedades"));
                _ = AppendMenu(helpMenu, MenuString, 6, Label("winui.comun.acerca_de_grxfirma"));
                if (!AppendMenu(menu, MenuPopup, (nuint)helpMenu, Label("winui.comun.ayuda")))
                {
                    _ = DestroyMenu(helpMenu);
                }
            }
            _ = AppendMenu(menu, MenuSeparator, 0, null);
            _ = AppendMenu(menu, MenuString, 4, Label("winui.bandeja.salir"));
            _ = SetForegroundWindow(_hwnd);
            _ = GetCursorPos(out var point);
            var selected = TrackPopupMenuEx(menu,
                ReturnCommand | RightButton, point.X, point.Y, _hwnd, 0);
            switch (selected)
            {
                case 1: _open(); break;
                case 3: _settings(); break;
                case 5: _help(); break;
                case 7:
                    _releaseNotesNotificationPending = false;
                    _releaseNotes();
                    break;
                case 6: _about(); break;
                case 4: _exit(); break;
            }
        }
        finally
        {
            _ = DestroyMenu(menu);
        }
    }

    public void Dispose()
    {
        if (_installed)
        {
            var data = CreateData(0);
            _ = ShellNotifyIcon(Delete, ref data);
            _installed = false;
        }
        if (_subclassInstalled)
        {
            _ = RemoveWindowSubclass(_hwnd, _subclass, SubclassId);
            _subclassInstalled = false;
        }
        if (_icon != 0)
        {
            _ = DestroyIcon(_icon);
        }
    }

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct NotifyIconData
    {
        public uint Size;
        public nint Window;
        public uint Id;
        public uint Flags;
        public uint CallbackMessage;
        public nint Icon;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)]
        public string Tip;
        public uint State;
        public uint StateMask;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)]
        public string Info;
        public uint VersionOrTimeout;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 64)]
        public string InfoTitle;
        public uint InfoFlags;
        public Guid Guid;
        public nint BalloonIcon;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct Point { public int X; public int Y; }

    private delegate nint SubclassProc(nint hwnd, uint message,
        nint wParam, nint lParam, nuint subclassId, nint reference);

    [DllImport("shell32.dll", CharSet = CharSet.Unicode, EntryPoint = "Shell_NotifyIconW")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool ShellNotifyIcon(uint action, ref NotifyIconData data);
    [DllImport("user32.dll", CharSet = CharSet.Unicode, EntryPoint = "LoadImageW")]
    private static extern nint LoadImage(nint instance, string name,
        uint type, int width, int height, uint flags);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool DestroyIcon(nint icon);
    [DllImport("comctl32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetWindowSubclass(nint hwnd, SubclassProc proc,
        nuint id, nint reference);
    [DllImport("comctl32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool RemoveWindowSubclass(nint hwnd, SubclassProc proc,
        nuint id);
    [DllImport("comctl32.dll")]
    private static extern nint DefSubclassProc(nint hwnd, uint message,
        nint wParam, nint lParam);
    [DllImport("user32.dll")]
    private static extern nint CreatePopupMenu();
    [DllImport("user32.dll", CharSet = CharSet.Unicode, EntryPoint = "AppendMenuW")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AppendMenu(nint menu, uint flags,
        nuint id, string? label);
    [DllImport("user32.dll")]
    private static extern uint TrackPopupMenuEx(nint menu, uint flags,
        int x, int y, nint hwnd, nint parameters);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool DestroyMenu(nint menu);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetCursorPos(out Point point);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetForegroundWindow(nint hwnd);
    [DllImport("user32.dll", CharSet = CharSet.Unicode,
        EntryPoint = "RegisterWindowMessageW")]
    private static extern uint RegisterWindowMessage(string name);
}
