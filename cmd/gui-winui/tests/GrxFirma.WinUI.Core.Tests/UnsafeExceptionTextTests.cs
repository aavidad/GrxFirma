// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

// Lote 39h: la ayuda se lee por apartados, el modo organización no explica
// cómo activar ni deshacer, cada nota se dice una vez, la fecha lleva el año
// con cuatro cifras y las evidencias no enseñan claves técnicas. Los mismos
// casos que cmd/gui-qml/tests/qml/tst_unsafetext.qml.
[TestClass]
public sealed class UnsafeExceptionTextTests
{
    private const string Help =
        "Qué hace este ajuste: mientras está desactivado, GrxFirma detiene esto.\n\n" +
        "Por qué es inseguro: riesgo.\n\nCuándo tiene sentido: pocas veces.\n\nAlternativa: otra.\n\n" +
        "Qué cambia: al activar, se permite.\n\nCómo deshacerlo: desactive el ajuste.\n\n" +
        "Bloqueo de la organización: consulte al administrador.";

    [TestMethod]
    public void Sections_SplitTitleAndBody()
    {
        var parts = UnsafeExceptionText.Sections(Help);
        Assert.AreEqual(7, parts.Count);
        Assert.AreEqual(new UnsafeHelpSection("Qué hace este ajuste", "Mientras está desactivado, GrxFirma detiene esto."), parts[0]);
        Assert.AreEqual("Bloqueo de la organización", parts[6].Title);
        Assert.AreEqual(new UnsafeHelpSection("Ce qui change", "Si vous activez"),
            UnsafeExceptionText.SplitSection("Ce qui change : si vous activez"));
        Assert.AreEqual(new UnsafeHelpSection("如何撤销", "在“安全”部分停用"),
            UnsafeExceptionText.SplitSection("如何撤销：在“安全”部分停用"));
        Assert.AreEqual(string.Empty, UnsafeExceptionText.SplitSection("Texto sin título").Title);
        Assert.AreEqual(2, UnsafeExceptionText.Sections("A: b\r\n\r\nC: d").Count);
    }

    [TestMethod]
    public void OrganizationHelp_SaysWhoDecidesInsteadOfHowToUndo()
    {
        var parts = UnsafeExceptionText.HelpSections(Help, "Quién lo decide: la organización.", true);
        CollectionAssert.AreEqual(
            new[] { "Qué hace este ajuste", "Por qué es inseguro", "Cuándo tiene sentido", "Alternativa", "Quién lo decide" },
            parts.Select(static p => p.Title).ToArray());
        Assert.AreEqual(7, UnsafeExceptionText.HelpSections(Help, "Quién lo decide: x.", false).Count);
    }

    [TestMethod]
    public void OrganizationDecides_OnlyByPolicy()
    {
        Assert.IsTrue(UnsafeExceptionText.OrganizationDecides(new() { Fixed = true, Enabled = true, Activable = true }));
        Assert.IsTrue(UnsafeExceptionText.OrganizationDecides(new() { Activable = false, OrganizationKey = "excepciones.x.organizacion" }));
        Assert.IsFalse(UnsafeExceptionText.OrganizationDecides(new()
        { Activable = false, ConfigUnavailable = true, OrganizationKey = "excepciones.x.organizacion" }));
        Assert.IsFalse(UnsafeExceptionText.OrganizationDecides(new() { Activable = true }));
    }

    [TestMethod]
    public void PolicyNote_IsSaidOnce()
    {
        Assert.AreEqual("excepciones.pantalla.solo_lectura",
            UnsafeExceptionText.PolicyNoteKey(new() { Id = "sha1_leer", Fixed = true, Enabled = true }));
        Assert.AreEqual("excepciones.sha1_leer.organizacion",
            UnsafeExceptionText.PolicyNoteKey(new() { Id = "sha1_leer", Fixed = true, OrganizationKey = "excepciones.sha1_leer.organizacion" }));
        Assert.AreEqual(string.Empty,
            UnsafeExceptionText.PolicyNoteKey(new() { Id = "sha1_leer", Fixed = true }));
        // El estado ya dice que la directiva no se puede leer.
        Assert.AreEqual(string.Empty,
            UnsafeExceptionText.PolicyNoteKey(new() { Id = "sha1_leer", Fixed = true, PolicyUnreadable = true }));
        Assert.AreEqual("excepciones.directorio_no_disponible",
            UnsafeExceptionText.PolicyNoteKey(new() { Id = "sha1_leer", ConfigUnavailable = true }));
        Assert.AreEqual(string.Empty,
            UnsafeExceptionText.PolicyNoteKey(new() { Id = "sha1_leer", Activable = true, Enabled = true }));
    }

    [TestMethod]
    public void DateText_UsesCatalogueLayoutAndFourDigitYear()
    {
        var instant = new DateTimeOffset(2026, 11, 15, 16, 44, 0, TimeSpan.Zero);
        var zone = TimeZoneInfo.Utc;
        Assert.AreEqual("15/11/2026 16:44", UnsafeExceptionText.DateText(instant, "02/01/2006 15:04", zone));
        Assert.AreEqual("15.11.2026 16:44", UnsafeExceptionText.DateText(instant, "02.01.2006 15:04", zone));
        Assert.AreEqual("2026-11-15 16:44", UnsafeExceptionText.DateText(instant, "2006-01-02 15:04", zone));
        // Sin formato en el catálogo, el del motor.
        Assert.AreEqual("15/11/2026 16:44", UnsafeExceptionText.DateText(instant, "format.datetime_minutes", zone));
    }

    [TestMethod]
    public void EvidenceSummary_ShowsCatalogueTextInsteadOfTechnicalKeys()
    {
        var directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(directory);
        try
        {
            File.WriteAllText(Path.Combine(directory, "es.json"), """
                {
                  "verificacion.detalle.signing_certificate": "Certificado del firmante atado a la firma",
                  "verificacion.detalle.algoritmo_antiguo": "Algoritmo antiguo",
                  "verificacion.detalle.valor.ausente": "no consta",
                  "verificacion.detalle.valor.sha1": "SHA-1"
                }
                """);
            var localizer = new CatalogLocalizer(directory, "es");
            Assert.AreEqual("no consta", VerificationDetailText.EvidenceSummary(localizer, "signing_certificate=ausente"));
            Assert.AreEqual("SHA-1", VerificationDetailText.EvidenceSummary(localizer, "algoritmo_antiguo=sha1"));
            // Sin entrada en el catálogo, tal cual.
            Assert.AreEqual("clave_inventada=1", VerificationDetailText.EvidenceSummary(localizer, "clave_inventada=1"));
        }
        finally
        {
            Directory.Delete(directory, recursive: true);
        }
    }
}
