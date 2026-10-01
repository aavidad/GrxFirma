# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
SERVICES = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
    / "Services"
)


class SecurePasswordServiceContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = (
            SERVICES / "ISecurePasswordPromptService.cs"
        ).read_text(encoding="utf-8")
        cls.buffer = (
            SERVICES / "NativePasswordBuffer.cs"
        ).read_text(encoding="utf-8")
        cls.windows = (
            SERVICES / "WindowsSecurePasswordPromptService.cs"
        ).read_text(encoding="utf-8")
        cls.windows_code = re.sub(
            r"^\s*//.*$",
            "",
            cls.windows,
            flags=re.MULTILINE,
        )

    def test_contract_returns_owned_disposable_native_buffer(self) -> None:
        self.assertIn(
            "SecurePasswordPromptRequest request",
            self.contract,
        )
        self.assertIn(
            "public sealed class NativePasswordBuffer : IDisposable",
            self.buffer,
        )
        self.assertIn(
            "NativePasswordConsumer<TResult>",
            self.contract,
        )
        self.assertNotIn("ReadOnlyMemory<char>", self.contract)
        self.assertNotIn("ReadOnlyMemory<byte>", self.contract)

    def test_request_validates_non_secret_ui_metadata_and_limit(self) -> None:
        self.assertIn(
            "public sealed record SecurePasswordPromptRequest",
            self.contract,
        )
        self.assertIn(
            "public const int MaximumSupportedCharacters = 16_384",
            self.contract,
        )
        self.assertIn("ValidateDisplayText(", self.contract)
        self.assertIn("MaximumCharacters = maximumCharacters", self.contract)
        self.assertIn("state.Request.Title", self.windows)
        self.assertIn("state.Request.Label", self.windows)
        self.assertIn(
            "state.Request.MaximumCharacters",
            self.windows,
        )

    def test_native_buffer_is_zeroed_before_free(self) -> None:
        release = re.search(
            r"protected override bool ReleaseHandle\(\)"
            r"\s*\{(?P<body>.*?)\n\s*\}",
            self.buffer,
            re.DOTALL,
        )
        self.assertIsNotNone(release)
        body = release.group("body")
        self.assertLess(
            body.index("NativeMemoryProtection.Zero"),
            body.index("Marshal.FreeHGlobal"),
        )
        self.assertIn('EntryPoint = "RtlZeroMemory"', self.buffer)
        self.assertNotIn('EntryPoint = "RtlSecureZeroMemory"', self.buffer)
        self.assertIn(
            "var handle = Interlocked.Exchange(ref _handle, null)",
            self.buffer,
        )
        self.assertLess(
            self.buffer.index(
                "var handle = Interlocked.Exchange(ref _handle, null)"
            ),
            self.buffer.index(
                "Volatile.Write(ref _characterCount, 0)"
            ),
        )

    def test_secret_is_copied_directly_from_native_edit(self) -> None:
        self.assertIn("EsPassword", self.windows)
        self.assertIn("EmSetLimitText", self.windows)
        self.assertIsNotNone(re.search(
            r"SendMessageW\(\s*edit,\s*WmGetText,"
            r".*?password\.DangerousBuffer",
            self.windows,
            re.DOTALL,
        ))
        for forbidden in (
            "new PasswordBox",
            ".Password",
            "GetWindowTextW",
            "Marshal.PtrToString",
            "Encoding.UTF",
            "SecureString",
        ):
            self.assertNotIn(forbidden, self.windows_code)

    def test_cancellation_crosses_to_modal_loop_without_secret_copy(self) -> None:
        self.assertIn(
            "cancellationToken.Register(",
            self.windows,
        )
        self.assertIn(
            "PostMessageW(\n                    dialog,\n"
            "                    CancellationMessage",
            self.windows,
        )
        self.assertIn(
            "throw new OperationCanceledException(cancellationToken)",
            self.windows,
        )
        self.assertIn(
            "WipeEditContents(\n"
            "            state.DetachEdit(),\n"
            "            state.Request.MaximumCharacters)",
            self.windows,
        )

    def test_user_cancel_and_token_cancel_are_distinct(self) -> None:
        self.assertIn("if (result == IdOk)", self.windows)
        self.assertIn("password?.Dispose();\n            return null;", self.windows)
        token_check = self.windows.index(
            "if (cancellationToken.IsCancellationRequested)"
        )
        user_result = self.windows.index("if (result == IdOk)")
        self.assertLess(token_check, user_result)

    def test_service_enforces_owner_ui_thread_and_single_prompt(self) -> None:
        self.assertIn(
            "Interlocked.CompareExchange(ref _activePrompt, 1, 0)",
            self.windows,
        )
        self.assertIn("ownerWindow.DispatcherQueue", self.windows)
        self.assertIn(
            "!_ownerDispatcher.HasThreadAccess",
            self.windows,
        )
        self.assertIn(
            "_ownerDispatcher.TryEnqueue(",
            self.windows,
        )
        self.assertIn(
            "CaptureOnOwnerThread(request, cancellationToken)",
            self.windows,
        )
        self.assertIn("GetWindowThreadProcessId", self.windows)
        self.assertIn("GetCurrentThreadId()", self.windows)

    def test_failures_expose_only_a_bounded_support_code(self) -> None:
        self.assertIn(
            "internal sealed class SecurePasswordPromptException",
            self.windows,
        )
        self.assertIn("SupportCode = supportCode", self.windows)
        self.assertIn("PASSWORD_DISPATCH_UNAVAILABLE", self.windows)
        self.assertIn('"PASSWORD_OWNER"', self.windows)
        self.assertIn(
            '"PASSWORD_DIALOG_FALLBACK"',
            self.windows,
        )
        self.assertIn(
            "exception.GetType().Name.ToUpperInvariant()",
            self.windows,
        )
        self.assertIn(
            "unchecked((uint)exception.HResult).ToString(\"X8\")",
            self.windows,
        )
        self.assertNotIn("innerException.Message", self.windows)
        self.assertNotIn("exception.Message", self.windows)

    def test_credential_ui_fallback_is_password_only_and_never_persists(self) -> None:
        self.assertIn(
            "CredUIPromptForCredentialsW(",
            self.windows,
        )
        self.assertIn(
            "CredUiFlagsPasswordOnlyOk",
            self.windows,
        )
        self.assertIn(
            "CredUiFlagsDoNotPersist",
            self.windows,
        )
        self.assertIn(
            "CredUiFlagsAlwaysShowUi",
            self.windows,
        )
        self.assertIn(
            "CredUiMaximumPasswordCharacters = 256",
            self.windows,
        )
        self.assertIn(
            "CredUiMaximumCaptionCharacters = 128",
            self.windows,
        )
        self.assertIn(
            "password.DangerousBuffer",
            self.windows,
        )
        self.assertIn(
            "NativeMemoryProtection.Zero(\n"
            "                    userName,\n"
            "                    userNameBytes)",
            self.windows,
        )
        self.assertIn(
            "CredUiCancellationState",
            self.windows,
        )
        self.assertIn(
            "PostMessageW(\n"
            "                            window,\n"
            "                            WmClose",
            self.windows,
        )


if __name__ == "__main__":
    unittest.main()
