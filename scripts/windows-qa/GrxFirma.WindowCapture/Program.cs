// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Threading.Channels;
using Windows.Graphics.Capture;
using Windows.Graphics.DirectX;
using Windows.Graphics.DirectX.Direct3D11;
using Windows.Graphics.Imaging;
using Windows.Storage.Streams;
using WinRT;

namespace GrxFirma.WindowCapture;

internal static class Program
{
    private const int MinimumGraphicsCaptureBuild = 18362;
    private const int MaximumDimension = 8192;
    private const long MaximumPixels = 33_177_600;
    private const uint D3D11CreateDeviceBgraSupport = 0x20;
    private const uint D3D11SdkVersion = 7;
    private const int D3DDriverTypeHardware = 1;
    private const int D3DDriverTypeWarp = 5;

    private static readonly Guid GraphicsCaptureItemInterface =
        new("79C3F95B-31F7-4EC2-A464-632EF5D30760");
    private static readonly Guid GraphicsCaptureItemInteropInterface =
        new("3628E81B-3CAC-4C60-B7F4-23CE0E0C3356");
    private static readonly Guid DxgiDeviceInterface =
        new("54EC77FA-1377-44E6-8C32-88FD5F44C84C");
    private static readonly HashSet<string> AllowedProcessNames =
        new(StringComparer.OrdinalIgnoreCase)
        {
            "grxfirma",
            "grxfirma-afirmauri",
            "grxfirma-gui",
            "grxfirma-gui-qml",
            "grxfirma-winui",
        };

    private static async Task<int> Main(string[] args)
    {
        try
        {
            var options = CaptureOptions.Parse(args);
            await CaptureWindowAsync(options);
            return 0;
        }
        catch (CaptureFailure exception)
        {
            Console.Error.WriteLine(exception.Code);
            return 2;
        }
        catch
        {
            Console.Error.WriteLine("unexpected-capture-failure");
            return 3;
        }
    }

    private static async Task CaptureWindowAsync(CaptureOptions options)
    {
        if (!OperatingSystem.IsWindowsVersionAtLeast(
                10,
                0,
                MinimumGraphicsCaptureBuild))
        {
            throw new CaptureFailure("graphics-capture-unavailable");
        }
        if (!GraphicsCaptureSession.IsSupported())
        {
            throw new CaptureFailure("graphics-capture-unsupported");
        }

        ValidateTargetWindow(options);
        await CaptureGraphicsItemAsync(
            options,
            options.WindowHandle);
    }

    private static async Task CaptureGraphicsItemAsync(
        CaptureOptions options,
        IntPtr captureHandle)
    {
        using var direct3DDevice = CreateDirect3DDevice();
        GraphicsCaptureItem item;
        try
        {
            item = CreateCaptureItem(captureHandle);
        }
        catch
        {
            throw new CaptureFailure(
                "graphics-capture-item-failed");
        }
        ValidateItemSize(item.Size.Width, item.Size.Height, options);

        Direct3D11CaptureFramePool framePool;
        try
        {
            framePool =
                Direct3D11CaptureFramePool.CreateFreeThreaded(
                    direct3DDevice,
                    DirectXPixelFormat.B8G8R8A8UIntNormalized,
                    1,
                    item.Size);
        }
        catch
        {
            throw new CaptureFailure(
                "graphics-capture-frame-pool-failed");
        }
        using (framePool)
        {
            await CaptureFramesAsync(
                options,
                item,
                framePool);
        }
    }

    private static async Task CaptureFramesAsync(
        CaptureOptions options,
        GraphicsCaptureItem item,
        Direct3D11CaptureFramePool framePool)
    {
        using var session = framePool.CreateCaptureSession(item);
        var frames = Channel.CreateBounded<Direct3D11CaptureFrame>(
            new BoundedChannelOptions(1)
            {
                FullMode = BoundedChannelFullMode.Wait,
                SingleReader = true,
                SingleWriter = false,
            });
        void OnFrameArrived(
            Direct3D11CaptureFramePool sender,
            object eventArgs)
        {
            try
            {
                var frame = sender.TryGetNextFrame();
                if (frame is not null &&
                    !frames.Writer.TryWrite(frame))
                {
                    frame.Dispose();
                }
            }
            catch
            {
                frames.Writer.TryComplete(
                    new CaptureFailure("graphics-capture-frame-failed"));
            }
        }

        framePool.FrameArrived += OnFrameArrived;
        var receivedFrame = false;
        try
        {
            session.StartCapture();
            using var timeout = new CancellationTokenSource(
                options.TimeoutMilliseconds);
            while (true)
            {
                using var frame = await frames.Reader.ReadAsync(
                    timeout.Token);
                receivedFrame = true;
                ValidateTargetWindow(options);
                ValidateItemSize(
                    frame.ContentSize.Width,
                    frame.ContentSize.Height,
                    options);
                using var bitmap =
                    await SoftwareBitmap.CreateCopyFromSurfaceAsync(
                        frame.Surface,
                        BitmapAlphaMode.Premultiplied);
                if (!HasUsefulClientContent(bitmap))
                {
                    continue;
                }

                ValidateTargetWindow(options);
                await SavePngAtomicallyAsync(
                    bitmap,
                    options.OutputPath);
                ValidateTargetWindow(options);
                return;
            }
        }
        catch (OperationCanceledException)
        {
            throw new CaptureFailure(
                receivedFrame
                    ? "window-content-not-ready"
                    : "graphics-capture-timeout");
        }
        finally
        {
            framePool.FrameArrived -= OnFrameArrived;
            frames.Writer.TryComplete();
            while (frames.Reader.TryRead(out var pendingFrame))
            {
                pendingFrame.Dispose();
            }
        }
    }

    private static bool HasUsefulClientContent(
        SoftwareBitmap bitmap)
    {
        if (bitmap.BitmapPixelFormat != BitmapPixelFormat.Bgra8)
        {
            throw new CaptureFailure("invalid-frame-pixel-format");
        }

        var width = bitmap.PixelWidth;
        var height = bitmap.PixelHeight;
        var byteCount = checked((uint)((long)width * height * 4));
        var pixelBuffer = new Windows.Storage.Streams.Buffer(byteCount);
        bitmap.CopyToBuffer(pixelBuffer);
        if (pixelBuffer.Length != byteCount)
        {
            throw new CaptureFailure("invalid-frame-buffer");
        }

        var pixels = new byte[checked((int)byteCount)];
        try
        {
            using (var reader = DataReader.FromBuffer(pixelBuffer))
            {
                reader.ReadBytes(pixels);
            }
            return HasUsefulClientContent(pixels, width, height);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(pixels);
        }
    }

    private static bool HasUsefulClientContent(
        byte[] pixels,
        int width,
        int height)
    {
        // Excluye marco, sombra y barra de titulo. Una captura WinUI no
        // renderizada conserva esas zonas aunque el cliente sea blanco.
        var horizontalMargin = Math.Max(16, width / 50);
        var topMargin = Math.Max(48, height / 10);
        var bottomMargin = Math.Max(16, height / 50);
        if (width <= horizontalMargin * 2 ||
            height <= topMargin + bottomMargin)
        {
            throw new CaptureFailure("invalid-client-content-region");
        }

        var colorCounts = new int[4096];
        var sampleCount = 0;
        var dominantCount = 0;
        var sampleStep = Math.Max(
            1,
            Math.Min(width, height) / 128);
        for (
            var y = topMargin;
            y < height - bottomMargin;
            y += sampleStep)
        {
            for (
                var x = horizontalMargin;
                x < width - horizontalMargin;
                x += sampleStep)
            {
                var offset = checked((y * width + x) * 4);
                if (pixels[offset + 3] < 64)
                {
                    continue;
                }

                var key =
                    ((pixels[offset + 2] >> 4) << 8) |
                    ((pixels[offset + 1] >> 4) << 4) |
                    (pixels[offset] >> 4);
                var count = ++colorCounts[key];
                dominantCount = Math.Max(dominantCount, count);
                sampleCount++;
            }
        }

        if (sampleCount < 100)
        {
            throw new CaptureFailure("insufficient-client-samples");
        }

        // Exige que al menos el 0,5 % del interior difiera del color
        // dominante. Rechaza el cliente uniforme y tolera temas claros.
        var minimumNonDominantSamples = Math.Max(
            24,
            (sampleCount + 199) / 200);
        return sampleCount - dominantCount >=
            minimumNonDominantSamples;
    }

    private static void ValidateTargetWindow(CaptureOptions options)
    {
        if (!IsWindow(options.WindowHandle) ||
            !IsWindowVisible(options.WindowHandle) ||
            IsIconic(options.WindowHandle))
        {
            throw new CaptureFailure("target-window-unavailable");
        }

        _ = GetWindowThreadProcessId(
            options.WindowHandle,
            out var processId);
        if (processId != options.ExpectedProcessId)
        {
            throw new CaptureFailure("target-window-process-mismatch");
        }
        try
        {
            using var process = Process.GetProcessById(
                checked((int)processId));
            if (!AllowedProcessNames.Contains(process.ProcessName))
            {
                throw new CaptureFailure(
                    "target-window-process-not-allowed");
            }
            if (process.StartTime.ToUniversalTime().Ticks !=
                options.ExpectedProcessStartUtcTicks)
            {
                throw new CaptureFailure(
                    "target-window-process-start-mismatch");
            }
            var processPath = Path.GetFullPath(
                process.MainModule?.FileName ??
                throw new CaptureFailure(
                    "target-window-process-path-mismatch"));
            if (!string.Equals(
                    processPath,
                    options.ExpectedProcessPath,
                    StringComparison.OrdinalIgnoreCase))
            {
                throw new CaptureFailure(
                    "target-window-process-path-mismatch");
            }
            ValidateProcessHash(
                processPath,
                options.ExpectedProcessSha256);
        }
        catch (CaptureFailure)
        {
            throw;
        }
        catch
        {
            throw new CaptureFailure("target-window-process-unavailable");
        }

        if (!GetWindowRect(options.WindowHandle, out var rectangle))
        {
            throw new CaptureFailure("target-window-bounds-unavailable");
        }
        var width = rectangle.Right - rectangle.Left;
        var height = rectangle.Bottom - rectangle.Top;
        ValidateItemSize(width, height, options);
        if (width != options.ExpectedWidth ||
            height != options.ExpectedHeight)
        {
            throw new CaptureFailure("target-window-bounds-changed");
        }
    }

    private static void ValidateProcessHash(
        string processPath,
        byte[] expectedHash)
    {
        byte[] actualHash;
        using (var stream = new FileStream(
                   processPath,
                   FileMode.Open,
                   FileAccess.Read,
                   FileShare.Read))
        {
            actualHash = SHA256.HashData(stream);
        }
        try
        {
            if (!CryptographicOperations.FixedTimeEquals(
                    actualHash,
                    expectedHash))
            {
                throw new CaptureFailure(
                    "target-window-process-hash-mismatch");
            }
        }
        finally
        {
            CryptographicOperations.ZeroMemory(actualHash);
        }
    }

    private static void ValidateItemSize(
        int width,
        int height,
        CaptureOptions options)
    {
        if (width < 1 ||
            height < 1 ||
            width > MaximumDimension ||
            height > MaximumDimension ||
            (long)width * height > MaximumPixels ||
            width > options.ExpectedWidth ||
            height > options.ExpectedHeight)
        {
            throw new CaptureFailure("invalid-capture-dimensions");
        }
    }

    private static IDirect3DDevice CreateDirect3DDevice()
    {
        var nativeDevice = IntPtr.Zero;
        var nativeContext = IntPtr.Zero;
        var dxgiDevice = IntPtr.Zero;
        var inspectableDevice = IntPtr.Zero;
        try
        {
            var result = D3D11CreateDevice(
                IntPtr.Zero,
                D3DDriverTypeHardware,
                IntPtr.Zero,
                D3D11CreateDeviceBgraSupport,
                IntPtr.Zero,
                0,
                D3D11SdkVersion,
                out nativeDevice,
                out _,
                out nativeContext);
            if (result < 0)
            {
                result = D3D11CreateDevice(
                    IntPtr.Zero,
                    D3DDriverTypeWarp,
                    IntPtr.Zero,
                    D3D11CreateDeviceBgraSupport,
                    IntPtr.Zero,
                    0,
                    D3D11SdkVersion,
                    out nativeDevice,
                    out _,
                    out nativeContext);
            }
            Marshal.ThrowExceptionForHR(result);

            var iid = DxgiDeviceInterface;
            Marshal.ThrowExceptionForHR(
                Marshal.QueryInterface(
                    nativeDevice,
                    in iid,
                    out dxgiDevice));
            Marshal.ThrowExceptionForHR(
                CreateDirect3D11DeviceFromDXGIDevice(
                    dxgiDevice,
                    out inspectableDevice));
            return MarshalInterface<IDirect3DDevice>.FromAbi(
                inspectableDevice);
        }
        finally
        {
            ReleaseIfNotZero(inspectableDevice);
            ReleaseIfNotZero(dxgiDevice);
            ReleaseIfNotZero(nativeContext);
            ReleaseIfNotZero(nativeDevice);
        }
    }

    private static GraphicsCaptureItem CreateCaptureItem(
        IntPtr windowHandle)
    {
        var className = IntPtr.Zero;
        var factory = IntPtr.Zero;
        var itemPointer = IntPtr.Zero;
        try
        {
            const string runtimeClass =
                "Windows.Graphics.Capture.GraphicsCaptureItem";
            Marshal.ThrowExceptionForHR(
                WindowsCreateString(
                    runtimeClass,
                    runtimeClass.Length,
                    out className));

            var factoryIid = GraphicsCaptureItemInteropInterface;
            Marshal.ThrowExceptionForHR(
                RoGetActivationFactory(
                    className,
                    ref factoryIid,
                    out factory));

            var vtable = Marshal.ReadIntPtr(factory);
            var createForWindowPointer = Marshal.ReadIntPtr(
                vtable,
                3 * IntPtr.Size);
            var createForWindow =
                Marshal.GetDelegateForFunctionPointer<
                    CreateForWindowDelegate>(
                    createForWindowPointer);
            var itemIid = GraphicsCaptureItemInterface;
            Marshal.ThrowExceptionForHR(
                createForWindow(
                    factory,
                    windowHandle,
                    ref itemIid,
                    out itemPointer));
            return MarshalInspectable<GraphicsCaptureItem>.FromAbi(
                itemPointer);
        }
        finally
        {
            ReleaseIfNotZero(itemPointer);
            ReleaseIfNotZero(factory);
            if (className != IntPtr.Zero)
            {
                _ = WindowsDeleteString(className);
            }
        }
    }

    private static async Task SavePngAtomicallyAsync(
        SoftwareBitmap bitmap,
        string outputPath)
    {
        var parent = Path.GetDirectoryName(outputPath);
        if (string.IsNullOrWhiteSpace(parent) ||
            !Directory.Exists(parent))
        {
            throw new CaptureFailure("invalid-output-directory");
        }
        ValidateLocalOutputDirectory(parent);
        var temporaryPath = Path.Combine(
            parent,
            $".{Path.GetFileName(outputPath)}.{Guid.NewGuid():N}.tmp");
        byte[]? bytes = null;
        try
        {
            using var memory = new InMemoryRandomAccessStream();
            var encoder = await BitmapEncoder.CreateAsync(
                BitmapEncoder.PngEncoderId,
                memory);
            encoder.SetSoftwareBitmap(bitmap);
            await encoder.FlushAsync();

            if (memory.Size < 1 || memory.Size > int.MaxValue)
            {
                throw new CaptureFailure("invalid-png-size");
            }
            memory.Seek(0);
            using var reader = new DataReader(
                memory.GetInputStreamAt(0));
            var size = checked((uint)memory.Size);
            _ = await reader.LoadAsync(size);
            bytes = new byte[size];
            reader.ReadBytes(bytes);
            await File.WriteAllBytesAsync(temporaryPath, bytes);
            File.Move(temporaryPath, outputPath, overwrite: false);
        }
        finally
        {
            if (bytes is not null)
            {
                CryptographicOperations.ZeroMemory(bytes);
            }
            if (File.Exists(temporaryPath))
            {
                File.Delete(temporaryPath);
            }
        }
    }

    private static void ValidateLocalOutputDirectory(string parent)
    {
        var root = Path.GetPathRoot(parent);
        if (string.IsNullOrWhiteSpace(root))
        {
            throw new CaptureFailure("invalid-output-directory");
        }
        try
        {
            if (new DriveInfo(root).DriveType ==
                DriveType.Network)
            {
                throw new CaptureFailure("network-output-forbidden");
            }
        }
        catch (CaptureFailure)
        {
            throw;
        }
        catch
        {
            throw new CaptureFailure("output-volume-unavailable");
        }

        var current = Path.GetFullPath(parent).TrimEnd(
            Path.DirectorySeparatorChar,
            Path.AltDirectorySeparatorChar);
        while (!string.IsNullOrWhiteSpace(current))
        {
            var attributes = File.GetAttributes(current);
            if ((attributes & FileAttributes.ReparsePoint) != 0)
            {
                throw new CaptureFailure("reparse-output-forbidden");
            }
            var next = Path.GetDirectoryName(current);
            if (string.IsNullOrWhiteSpace(next) ||
                string.Equals(
                    next,
                    current,
                    StringComparison.OrdinalIgnoreCase))
            {
                break;
            }
            current = next.TrimEnd(
                Path.DirectorySeparatorChar,
                Path.AltDirectorySeparatorChar);
        }
    }

    private static void ReleaseIfNotZero(IntPtr value)
    {
        if (value != IntPtr.Zero)
        {
            _ = Marshal.Release(value);
        }
    }

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int CreateForWindowDelegate(
        IntPtr factory,
        IntPtr window,
        ref Guid iid,
        out IntPtr result);

    [StructLayout(LayoutKind.Sequential)]
    private struct Rect
    {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }

    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool IsWindow(IntPtr window);

    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool IsWindowVisible(IntPtr window);

    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool IsIconic(IntPtr window);

    [DllImport("user32.dll")]
    private static extern uint GetWindowThreadProcessId(
        IntPtr window,
        out uint processId);

    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetWindowRect(
        IntPtr window,
        out Rect rectangle);

    [DllImport("d3d11.dll")]
    private static extern int D3D11CreateDevice(
        IntPtr adapter,
        int driverType,
        IntPtr software,
        uint flags,
        IntPtr featureLevels,
        uint featureLevelCount,
        uint sdkVersion,
        out IntPtr device,
        out uint selectedFeatureLevel,
        out IntPtr immediateContext);

    [DllImport("d3d11.dll")]
    private static extern int CreateDirect3D11DeviceFromDXGIDevice(
        IntPtr dxgiDevice,
        out IntPtr graphicsDevice);

    [DllImport("combase.dll")]
    private static extern int WindowsCreateString(
        [MarshalAs(UnmanagedType.LPWStr)] string source,
        int length,
        out IntPtr value);

    [DllImport("combase.dll")]
    private static extern int WindowsDeleteString(IntPtr value);

    [DllImport("combase.dll")]
    private static extern int RoGetActivationFactory(
        IntPtr activatableClassId,
        ref Guid iid,
        out IntPtr factory);

    private sealed record CaptureOptions(
        IntPtr WindowHandle,
        uint ExpectedProcessId,
        long ExpectedProcessStartUtcTicks,
        string ExpectedProcessPath,
        byte[] ExpectedProcessSha256,
        int ExpectedWidth,
        int ExpectedHeight,
        string OutputPath,
        int TimeoutMilliseconds)
    {
        public static CaptureOptions Parse(string[] args)
        {
            if (args.Length != 18)
            {
                throw new CaptureFailure("invalid-arguments");
            }
            var values = new Dictionary<string, string>(
                StringComparer.Ordinal);
            for (var index = 0; index < args.Length; index += 2)
            {
                if (!args[index].StartsWith(
                        "--",
                        StringComparison.Ordinal) ||
                    !values.TryAdd(args[index], args[index + 1]))
                {
                    throw new CaptureFailure("invalid-arguments");
                }
            }
            var expectedKeys = new[]
            {
                "--window-handle",
                "--expected-process-id",
                "--expected-process-start-utc-ticks",
                "--expected-process-path",
                "--expected-process-sha256",
                "--expected-width",
                "--expected-height",
                "--output",
                "--timeout-ms",
            };
            if (values.Count != expectedKeys.Length ||
                expectedKeys.Any(key => !values.ContainsKey(key)))
            {
                throw new CaptureFailure("invalid-arguments");
            }

            var handleText = values["--window-handle"];
            if (!handleText.StartsWith(
                    "0x",
                    StringComparison.Ordinal) ||
                !long.TryParse(
                    handleText.AsSpan(2),
                    NumberStyles.AllowHexSpecifier,
                    CultureInfo.InvariantCulture,
                    out var handle) ||
                handle <= 0)
            {
                throw new CaptureFailure("invalid-window-handle");
            }
            if (!uint.TryParse(
                    values["--expected-process-id"],
                    NumberStyles.None,
                    CultureInfo.InvariantCulture,
                    out var processId) ||
                processId < 1)
            {
                throw new CaptureFailure("invalid-process-id");
            }
            if (!long.TryParse(
                    values["--expected-process-start-utc-ticks"],
                    NumberStyles.None,
                    CultureInfo.InvariantCulture,
                    out var processStartTicks) ||
                processStartTicks < DateTime.UnixEpoch.Ticks)
            {
                throw new CaptureFailure("invalid-process-id");
            }
            var processPath = Path.GetFullPath(
                values["--expected-process-path"]);
            if (!File.Exists(processPath) ||
                !AllowedProcessNames.Contains(
                    Path.GetFileNameWithoutExtension(processPath)))
            {
                throw new CaptureFailure("invalid-process-id");
            }
            byte[] processHash;
            try
            {
                processHash = Convert.FromHexString(
                    values["--expected-process-sha256"]);
            }
            catch
            {
                throw new CaptureFailure("invalid-process-id");
            }
            if (processHash.Length != SHA256.HashSizeInBytes)
            {
                CryptographicOperations.ZeroMemory(processHash);
                throw new CaptureFailure("invalid-process-id");
            }
            if (!int.TryParse(
                    values["--expected-width"],
                    NumberStyles.None,
                    CultureInfo.InvariantCulture,
                    out var width) ||
                !int.TryParse(
                    values["--expected-height"],
                    NumberStyles.None,
                    CultureInfo.InvariantCulture,
                    out var height))
            {
                throw new CaptureFailure("invalid-capture-dimensions");
            }
            var outputPath = Path.GetFullPath(values["--output"]);
            if (!string.Equals(
                    Path.GetExtension(outputPath),
                    ".png",
                    StringComparison.OrdinalIgnoreCase) ||
                File.Exists(outputPath))
            {
                throw new CaptureFailure("invalid-output-path");
            }
            if (!int.TryParse(
                    values["--timeout-ms"],
                    NumberStyles.None,
                    CultureInfo.InvariantCulture,
                    out var timeout) ||
                timeout < 250 ||
                timeout > 30_000)
            {
                throw new CaptureFailure("invalid-timeout");
            }
            var options = new CaptureOptions(
                new IntPtr(handle),
                processId,
                processStartTicks,
                processPath,
                processHash,
                width,
                height,
                outputPath,
                timeout);
            ValidateItemSize(width, height, options);
            return options;
        }
    }

    private sealed class CaptureFailure(string code) : Exception
    {
        public string Code { get; } = code;
    }
}
