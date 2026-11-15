// SPDX-License-Identifier: EUPL-1.2
package excepciones

import (
	"context"
	"net/url"
	"strings"
)

// RequireProxyCredentials protege el tramo hasta el proxy, no el destino.
// HTTPS cifra las credenciales; HTTP, SOCKS5 y SOCKS5H necesitan la excepción.
func RequireProxyCredentials(ctx context.Context, proxy *url.URL) error {
	if proxy == nil || proxy.User == nil || strings.EqualFold(proxy.Scheme, "https") {
		return nil
	}
	return Require(ctx, "proxy_http")
}
