// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;

namespace GrxFirma.WinUI.Core.Localization;

// Puerta de las pasadas diferidas sobre el árbol de una ventana, como la
// traducción que se encola tras LayoutUpdated. Al cerrarse la ventana, el
// despachador aún ejecuta lo que quedó en cola mientras se apaga; tocar
// entonces el árbol XAML lanza COMException (E_UNEXPECTED) desde un async
// void y el proceso terminaba con un fallo. En 0.0.118 ocurría al mover el
// sello del portal y pulsar enseguida «Firmar con el sello aquí»: el
// resultado ya estaba escrito, pero el proceso acababa con error.
public sealed class DeferredTreePass
{
    private int _closed;

    public bool IsClosed => Volatile.Read(ref _closed) != 0;

    // Se llama al cerrarse la ventana: ninguna pasada posterior toca el árbol.
    public void Close() => Interlocked.Exchange(ref _closed, 1);

    // Ejecuta la pasada si la ventana sigue abierta. Si el árbol ya no
    // responde, la pasada se descarta en vez de propagar el error.
    public bool TryRun(Action pass)
    {
        ArgumentNullException.ThrowIfNull(pass);
        if (IsClosed) return false;
        try
        {
            pass();
            return true;
        }
        catch (COMException)
        {
            return false;
        }
    }
}
