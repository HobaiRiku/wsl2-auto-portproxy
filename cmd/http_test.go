package cmd

import "testing"

func TestEndpointRejectsCredentialRedirects(t *testing.T) {
	for _, address := range []string{"http://example.com:80", "https://127.0.0.1:443", "http://user@127.0.0.1:80", "http://127.0.0.1:80/other", "http://127.0.0.1:80?x=y", "http://127.0.0.1:0"} {
		if validateEndpoint(address) == nil {
			t.Fatalf("accepted %s", address)
		}
	}
	for _, address := range []string{"http://127.0.0.1:47831", "http://[::1]:47831"} {
		if err := validateEndpoint(address); err != nil {
			t.Fatal(err)
		}
	}
}
