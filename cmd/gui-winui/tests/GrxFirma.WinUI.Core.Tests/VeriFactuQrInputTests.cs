// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class VeriFactuQrInputTests
{
    [TestMethod]
    [DataRow("https://www2.agenciatributaria.gob.es.evil.test/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z")]
    [DataRow("https://www2.agenciatributaria.gob.es@evil.test/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z")]
    [DataRow("https://www2.agenciatributaria.gob.es:443/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z")]
    [DataRow("https://www2.agenciatributaria.gob.es./wlpl/TIKE-CONT/ValidarQR?nif=12345678Z")]
    [DataRow("https://ｗｗｗ2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z")]
    [DataRow("http://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z")]
    [DataRow("https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z#fragment")]
    public void RejectsAmbiguousAuthorities(string url) =>
        Assert.ThrowsExactly<ArgumentException>(() => VeriFactuQrInput.EnsureAllowedAuthority(url));

    [TestMethod]
    public void RejectsNull() =>
        Assert.ThrowsExactly<ArgumentNullException>(() => VeriFactuQrInput.EnsureAllowedAuthority(null!));

    [TestMethod]
    [DataRow("www2.agenciatributaria.gob.es", "ValidarQR")]
    [DataRow("prewww2.aeat.es", "ValidarQRNoVerifactu")]
    public void AcceptsOfficialAuthorities(string host, string path) =>
        VeriFactuQrInput.EnsureAllowedAuthority("https://" + host + "/wlpl/TIKE-CONT/" + path + "?nif=12345678Z&numserie=A&fecha=01-01-2025&importe=1.00");
}
