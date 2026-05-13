package fleetintelligence

import "testing"

func TestNewClientRequiresBaseURL(t *testing.T) {
	_, err := NewClient("", "key")
	if err != ErrMissingBaseURL {
		t.Fatalf("expected ErrMissingBaseURL, got %v", err)
	}
}

func TestNewClientRequiresServiceKey(t *testing.T) {
	_, err := NewClient("https://example.com", "")
	if err != ErrMissingServiceKey {
		t.Fatalf("expected ErrMissingServiceKey, got %v", err)
	}
}

func TestNewClientStoresConfiguration(t *testing.T) {
	client, err := NewClient("https://example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if client.BaseURL() != "https://example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}

	if !client.ServiceKeyConfigured() {
		t.Fatal("expected service key to be configured")
	}
}
