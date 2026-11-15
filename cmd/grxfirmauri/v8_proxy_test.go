// SPDX-License-Identifier: EUPL-1.2
package main

import "testing"

func TestSplitProxyHostPortHTTPHTTPSYIPv6(t *testing.T) {
	for _, tc := range []struct {
		url, host string
		port      int
	}{
		{"http://127.0.0.1:8080", "127.0.0.1", 8080},
		{"https://127.0.0.1:8443", "127.0.0.1", 8443},
		{"http://[::1]:9090", "::1", 9090},
		{"https://[::1]:9443", "::1", 9443},
	} {
		t.Run(tc.url, func(t *testing.T) {
			host, port := splitProxyHostPort(t, tc.url)
			if host != tc.host || port != tc.port {
				t.Fatalf("host=%q puerto=%d; esperado %q %d", host, port, tc.host, tc.port)
			}
		})
	}
}
