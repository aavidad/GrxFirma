// SPDX-License-Identifier: EUPL-1.2
package triphase

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Vectores producidos por java.util.Properties.load(InputStream), OpenJDK 8u512.
// CI no necesita Java: los resultados del intérprete de referencia están en el fixture.
func TestJavaPropertiesCoincideConJDK(t *testing.T) {
	raw, err := os.ReadFile("testdata/java_properties.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name       string            `json:"name"`
		Body       string            `json:"body_base64"`
		Invalid    bool              `json:"invalid"`
		Properties map[string]string `json:"properties"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			body, err := base64.StdEncoding.DecodeString(tc.Body)
			if err != nil {
				t.Fatal(err)
			}
			pairs, err := readJavaProperties(string(body))
			if (err != nil) != tc.Invalid {
				t.Fatalf("error=%v esperado inválido=%v", err, tc.Invalid)
			}
			if tc.Invalid {
				return
			}
			got := map[string]string{}
			for _, p := range pairs {
				got[p[0]] = p[1]
			}
			if !reflect.DeepEqual(got, tc.Properties) {
				t.Fatalf("Go=%q Java=%q", got, tc.Properties)
			}
		})
	}
}
