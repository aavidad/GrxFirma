// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

// Ejemplos del apartado 9 de la especificación del QR de Veri*Factu y la
// respuesta real recibida en el recorrido de Windows 0.0.117.
[TestClass]
public sealed class VeriFactuQrResponseTests
{
    private static string Answer(string result, string message) =>
        "{\"status\":\"OK\",\"mensaje\":\"" + message + "\",\"visible\":\"N\",\"crashlytics\":\"N\"," +
        "\"respuesta\":{\"resultado\":\"" + result + "\",\"nif\":\"89890001K\",\"numserie\":\"12345678\"," +
        "\"fecha\":\"01-09-2024\",\"importe\":\"241.40\"}}";

    [TestMethod]
    public void RecognisesAFoundInvoice() =>
        Assert.AreEqual(VeriFactuQrResponse.FoundKey, VeriFactuQrResponse.Classify(Answer("00", "Encontrada")));

    [TestMethod]
    public void RecognisesAMissingInvoice() =>
        Assert.AreEqual(VeriFactuQrResponse.NotFoundKey, VeriFactuQrResponse.Classify(Answer("01", "No encontrada")));

    [TestMethod]
    public void NoContrastableIsNeverReportedAsRegistered() =>
        Assert.AreEqual(VeriFactuQrResponse.NotVerifiableKey, VeriFactuQrResponse.Classify(Answer("02", "No contrastable")));

    [TestMethod]
    public void RejectedQueriesAreNotInvoiceResults() =>
        Assert.AreEqual(VeriFactuQrResponse.RejectedKey, VeriFactuQrResponse.Classify(
            "{\"status\":\"KO\",\"mensaje\":\"El importe tiene un formato incorrecto\",\"visible\":\"S\",\"crashlytics\":\"N\",\"codigo_error\":\"2005\"}"));

    [TestMethod]
    [DataRow("{\"status\":\"OK\"}")]
    [DataRow("{\"status\":\"OK\",\"mensaje\":\"Encontrada\"}")]
    [DataRow("{\"status\":\"OK\",\"respuesta\":{\"resultado\":\"99\"}}")]
    [DataRow("{\"status\":\"OK\",\"respuesta\":{\"resultado\":0}}")]
    [DataRow("{\"resultado\":\"ok\"}")]
    [DataRow("{\"resultado\":\"Factura encontrada\"}")]
    [DataRow("[1]")]
    [DataRow("no es json")]
    [DataRow("")]
    [DataRow(null)]
    public void DoesNotGuessUnknownAnswers(string? json) =>
        Assert.AreEqual(VeriFactuQrResponse.UnknownKey, VeriFactuQrResponse.Classify(json));
}
