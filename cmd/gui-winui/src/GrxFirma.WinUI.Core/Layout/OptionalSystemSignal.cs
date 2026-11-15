// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Layout;

// Señales del sistema que solo sirven para repintar (alto contraste, colores
// de Windows). No todas las versiones de Windows las ofrecen a una app de
// escritorio: en Windows 10 22H2, suscribirse a
// AccessibilitySettings.HighContrastChanged lanza COMException 0x80070490
// (ERROR_NOT_FOUND) y la app se cerraba antes de mostrar la ventana. Sin la
// señal la app sigue y se repinta por otra vía; nunca se cae por esto.
public static class OptionalSystemSignal
{
    /// <summary>
    /// Ejecuta la suscripción y devuelve si quedó hecha. Cualquier fallo de la
    /// plataforma se absorbe; solo se deja pasar la falta de memoria.
    /// </summary>
    public static bool TrySubscribe(Action subscribe)
    {
        ArgumentNullException.ThrowIfNull(subscribe);
        try
        {
            subscribe();
            return true;
        }
        catch (Exception exception) when (exception is not OutOfMemoryException)
        {
            return false;
        }
    }
}
