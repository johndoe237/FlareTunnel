package main

import "testing"

func TestParseTransportSANs(t *testing.T) {
	dns, ips, err := parseTransportSANs("proxy.example.test 203.0.113.42 2001:db8::42")
	if err != nil {
		t.Fatal(err)
	}
	if len(dns) != 1 || dns[0] != "proxy.example.test" {
		t.Fatalf("dns=%v", dns)
	}
	if len(ips) != 2 {
		t.Fatalf("ips=%v", ips)
	}
}

func TestParseTransportSANsRejectsUnspecifiedAndMissing(t *testing.T) {
	for _, raw := range []string{"", "0.0.0.0", "::", "proxy.example.test/24"} {
		if _, _, err := parseTransportSANs(raw); err == nil {
			t.Errorf("parseTransportSANs(%q) accepted invalid SAN", raw)
		}
	}
}
