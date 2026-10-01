// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Buffers;
using System.Text;

namespace GrxFirma.WinUI.Core.Ipc;

internal sealed class LimitedNdjsonReader
{
    private static readonly Encoding StrictUtf8 =
        new UTF8Encoding(encoderShouldEmitUTF8Identifier: false, throwOnInvalidBytes: true);

    private readonly Stream _stream;
    private readonly byte[] _buffer = new byte[8192];
    private int _offset;
    private int _available;

    public LimitedNdjsonReader(Stream stream)
    {
        _stream = stream;
    }

    public async Task<string> ReadLineAsync(
        int maximumBytes,
        CancellationToken cancellationToken)
    {
        var writer = new ArrayBufferWriter<byte>(Math.Min(maximumBytes, _buffer.Length));

        while (true)
        {
            if (_available == 0)
            {
                _offset = 0;
                _available = await _stream
                    .ReadAsync(_buffer.AsMemory(), cancellationToken)
                    .ConfigureAwait(false);
                if (_available == 0)
                {
                    throw IpcClientException.Closed();
                }
            }

            var newlineIndex = Array.IndexOf(_buffer, (byte)'\n', _offset, _available);
            var bytesToCopy = newlineIndex >= 0 ? newlineIndex - _offset : _available;
            if (writer.WrittenCount + bytesToCopy > maximumBytes)
            {
                throw IpcClientException.FrameTooLarge();
            }

            if (bytesToCopy > 0)
            {
                var destination = writer.GetSpan(bytesToCopy);
                _buffer.AsSpan(_offset, bytesToCopy).CopyTo(destination);
                writer.Advance(bytesToCopy);
                _offset += bytesToCopy;
                _available -= bytesToCopy;
            }

            if (newlineIndex < 0)
            {
                continue;
            }

            _offset++;
            _available--;
            var line = writer.WrittenSpan;
            if (!line.IsEmpty && line[^1] == (byte)'\r')
            {
                line = line[..^1];
            }

            if (line.IsEmpty)
            {
                throw IpcClientException.Protocol();
            }

            try
            {
                return StrictUtf8.GetString(line);
            }
            catch (DecoderFallbackException)
            {
                throw IpcClientException.Protocol();
            }
        }
    }
}
