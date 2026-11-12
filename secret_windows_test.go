//go:build windows

package main

import "testing"

func TestDPAPIRoundTrip(t *testing.T) {
	const plain = "sk-test-秘密"
	sealed, err := protectSecret(plain)
	if err != nil {
		t.Fatalf("protectSecret: %v", err)
	}
	if sealed == "" || sealed == plain {
		t.Fatalf("sealed value = %q", sealed)
	}
	got, err := unprotectSecret(sealed)
	if err != nil {
		t.Fatalf("unprotectSecret: %v", err)
	}
	if got != plain {
		t.Fatalf("round trip = %q, want %q", got, plain)
	}
}

func TestDPAPIEmptyAndCorrupt(t *testing.T) {
	if got, err := protectSecret(""); err != nil || got != "" {
		t.Fatalf("protectSecret(empty) = %q, %v", got, err)
	}
	if got, err := unprotectSecret(""); err != nil || got != "" {
		t.Fatalf("unprotectSecret(empty) = %q, %v", got, err)
	}
	if _, err := unprotectSecret("not-base64"); err == nil {
		t.Fatal("corrupt blob was accepted")
	}
}
