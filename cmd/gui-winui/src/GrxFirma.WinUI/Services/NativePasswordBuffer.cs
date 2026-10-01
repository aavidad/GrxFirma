// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

namespace GrxFirma.WinUI.Services;

/// <summary>
/// Propietario de una contraseña UTF-16 almacenada exclusivamente en memoria
/// nativa borrable. No expone ninguna conversión a texto administrado.
/// </summary>
public sealed class NativePasswordBuffer : IDisposable
{
    private SafeNativePasswordHandle? _handle;
    private int _characterCount;

    private NativePasswordBuffer(
        SafeNativePasswordHandle handle,
        int capacityCharacters)
    {
        _handle = handle;
        CapacityCharacters = capacityCharacters;
    }

    /// <summary>
    /// Número de caracteres válidos, sin incluir el terminador UTF-16.
    /// </summary>
    public int CharacterCount
    {
        get
        {
            ThrowIfDisposed();
            return Volatile.Read(ref _characterCount);
        }
    }

    internal int CapacityCharacters { get; }

    internal nint DangerousBuffer =>
        Volatile.Read(ref _handle)?.DangerousGetHandle() ??
        throw new ObjectDisposedException(nameof(NativePasswordBuffer));

    internal static NativePasswordBuffer Allocate(int capacityCharacters)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(capacityCharacters);
        var byteCapacity = checked((capacityCharacters + 1) * sizeof(char));
        return new(
            SafeNativePasswordHandle.Allocate(byteCapacity),
            capacityCharacters);
    }

    internal void SetCharacterCount(int characterCount)
    {
        ObjectDisposedException.ThrowIf(_handle is null, this);
        if (characterCount < 0 || characterCount > CapacityCharacters)
        {
            throw new ArgumentOutOfRangeException(nameof(characterCount));
        }

        Volatile.Write(ref _characterCount, characterCount);
    }

    /// <summary>
    /// Presta el puntero durante la llamada al consumidor y mantiene viva su
    /// asignación aunque otro hilo solicite la liberación simultáneamente.
    /// </summary>
    public TResult Use<TResult>(
        NativePasswordConsumer<TResult> consumer)
    {
        ArgumentNullException.ThrowIfNull(consumer);
        var handle = Volatile.Read(ref _handle) ??
            throw new ObjectDisposedException(nameof(NativePasswordBuffer));
        var addedReference = false;
        try
        {
            handle.DangerousAddRef(ref addedReference);
            var characterCount = Volatile.Read(ref _characterCount);
            if (Volatile.Read(ref _handle) is null)
            {
                throw new ObjectDisposedException(
                    nameof(NativePasswordBuffer));
            }

            return consumer(
                handle.DangerousGetHandle(),
                characterCount);
        }
        finally
        {
            if (addedReference)
            {
                handle.DangerousRelease();
            }
        }
    }

    /// <summary>
    /// Sobrescribe la capacidad completa, no solo la longitud utilizada, antes
    /// de devolver la asignación al sistema operativo.
    /// </summary>
    public void Dispose()
    {
        var handle = Interlocked.Exchange(ref _handle, null);
        Volatile.Write(ref _characterCount, 0);
        handle?.Dispose();
        GC.SuppressFinalize(this);
    }

    private void ThrowIfDisposed() =>
        ObjectDisposedException.ThrowIf(
            Volatile.Read(ref _handle) is null,
            this);
}

internal sealed class SafeNativePasswordHandle
    : SafeHandleZeroOrMinusOneIsInvalid
{
    private readonly int _byteCapacity;

    private SafeNativePasswordHandle(
        nint pointer,
        int byteCapacity)
        : base(ownsHandle: true)
    {
        _byteCapacity = byteCapacity;
        SetHandle(pointer);
    }

    internal static SafeNativePasswordHandle Allocate(int byteCapacity)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(byteCapacity, sizeof(char));
        var pointer = Marshal.AllocHGlobal(byteCapacity);
        NativeMemoryProtection.Zero(pointer, byteCapacity);
        return new(pointer, byteCapacity);
    }

    protected override bool ReleaseHandle()
    {
        NativeMemoryProtection.Zero(handle, _byteCapacity);
        Marshal.FreeHGlobal(handle);
        return true;
    }
}

internal static class NativeMemoryProtection
{
    internal static void Zero(nint pointer, int byteCount)
    {
        if (pointer == 0 || byteCount <= 0)
        {
            return;
        }

        // SecureZeroMemory/RtlSecureZeroMemory son macros de los SDK de
        // Windows, no puntos de entrada exportados por kernel32.dll. Una
        // llamada P/Invoke con ese nombre compila, pero falla en ejecución.
        // La llamada externa a RtlZeroMemory sí está exportada y no puede ser
        // eliminada por el JIT como una escritura administrada sin uso.
        RtlZeroMemory(pointer, checked((nuint)byteCount));
    }

    [DllImport(
        "kernel32.dll",
        EntryPoint = "RtlZeroMemory",
        ExactSpelling = true)]
    private static extern void RtlZeroMemory(
        nint destination,
        nuint length);
}
