// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class VeriFactuQrResponseTests
{
    [TestMethod]
    [DataRow("{\"resultado\":\"ok\"}")]
    [DataRow("{\"resultado\":\"Factura encontrada\"}")]
    [DataRow("{\"estado\":\"Registrada\"}")]
    public void RecognisesARegisteredInvoice(string json) =>
        Assert.AreEqual(VeriFactuQrResponse.FoundKey, VeriFactuQrResponse.Classify(json));

    [TestMethod]
    [DataRow("{\"resultado\":\"Factura no encontrada\"}")]
    [DataRow("{\"estado\":\"KO\"}")]
    [DataRow("{\"mensaje\":\"No se ha identificado la factura\"}")]
    [DataRow("{\"result\":\"NOT_FOUND\"}")]
    public void RecognisesAMissingInvoice(string json) =>
        Assert.AreEqual(VeriFactuQrResponse.NotFoundKey, VeriFactuQrResponse.Classify(json));

    [TestMethod]
    [DataRow("{\"x\":1}")]
    [DataRow("")]
    [DataRow(null)]
    public void DoesNotGuessUnknownAnswers(string? json) =>
        Assert.AreEqual(VeriFactuQrResponse.UnknownKey, VeriFactuQrResponse.Classify(json));
}
