package application

import (
	"strings"
	"testing"
)

func TestToolSupplyRequiresCanonicalLocalCapability(t *testing.T) {
	valid := ToolSupply{Delivery: ToolDeliveryMCPHTTP, Endpoint: "http://127.0.0.1:54321/mcp",
		BearerToken: strings.Repeat("a", 64)}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ToolSupply{}).Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []ToolSupply{
		{Delivery: "shell", Endpoint: valid.Endpoint, BearerToken: valid.BearerToken},
		{Delivery: valid.Delivery, Endpoint: "http://localhost:54321/mcp", BearerToken: valid.BearerToken},
		{Delivery: valid.Delivery, Endpoint: "http://127.0.0.1:54321/%6dcp", BearerToken: valid.BearerToken},
		{Delivery: valid.Delivery, Endpoint: "http://127.0.0.1:54321/mcp?redirect=http://host", BearerToken: valid.BearerToken},
		{Delivery: valid.Delivery, Endpoint: "http://127.0.0.1:54321/other", BearerToken: valid.BearerToken},
		{Delivery: valid.Delivery, Endpoint: "http://127.0.0.1:abc/mcp", BearerToken: valid.BearerToken},
		{Delivery: valid.Delivery, Endpoint: "http://127.0.0.1:99999/mcp", BearerToken: valid.BearerToken},
		{Delivery: valid.Delivery, Endpoint: valid.Endpoint, BearerToken: "insecure"},
		{Delivery: valid.Delivery, Endpoint: valid.Endpoint, BearerToken: strings.Repeat("g", 64)},
		{Delivery: valid.Delivery, Endpoint: "https://127.0.0.1:54321/mcp", BearerToken: valid.BearerToken},
	}
	for _, v := range tests {
		if err := v.Validate(); err == nil {
			t.Fatalf("invalid transport allowed: delivery=%q endpoint=%q", v.Delivery, v.Endpoint)
		}
	}
}
