package main

import (
	"testing"
)

func TestExtractSPIFFEID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "mTLS x-forwarded-client-cert with URI",
			input:    "By=spiffe://cluster.local/ns/zta-research/sa/frontend;URI=spiffe://cluster.local/ns/zta-research/sa/frontend;Cert=...",
			expected: "spiffe://cluster.local/ns/zta-research/sa/frontend",
		},
		{
			name:     "plain SPIFFE URI",
			input:    "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
			expected: "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
		},
		{
			name:     "mTLS x-forwarded-client-cert with quoted URI",
			input:    `By="spiffe://cluster.local/ns/zta-research/sa/ingress";URI="spiffe://cluster.local/ns/zta-research/sa/frontend"`,
			expected: "spiffe://cluster.local/ns/zta-research/sa/frontend",
		},
		{
			name:     "arbitrary non-spiffe string is rejected",
			input:    "custom-identity",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSPIFFEID(tt.input)
			if got != tt.expected {
				t.Errorf("extractSPIFFEID(%q) = %q, expected %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeTargetIdentity(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"productcatalogservice.default.svc.cluster.local:8080", "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice"},
		{"cartservice:8080", "spiffe://cluster.local/ns/zta-research/sa/cartservice"},
		{"currencyservice", "spiffe://cluster.local/ns/zta-research/sa/currencyservice"},
		{"paymentservice", "spiffe://cluster.local/ns/zta-research/sa/paymentservice"},
		{"checkoutservice", "spiffe://cluster.local/ns/zta-research/sa/checkoutservice"},
		{"recommendationservice", "spiffe://cluster.local/ns/zta-research/sa/recommendationservice"},
		{"spiffe://cluster.local/ns/other/sa/custom", "spiffe://cluster.local/ns/other/sa/custom"},
	}

	for _, tt := range tests {
		got := normalizeTargetIdentity(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeTargetIdentity(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}
