package curated

import (
	"testing"
)

func TestReviewOnlyAnySearchAndClone(t *testing.T) {
	offerings := List()
	if len(offerings) != 1 {
		t.Fatalf("unexpected offerings %d", len(offerings))
	}
	o := offerings[0]
	if o.OfferingID != "io.mender.integration.anysearch" || o.Endpoint != "https://api.anysearch.com/mcp" ||
		o.DriverPluginID != "io.mender.provider.mcp" || o.Authentication != "optional-bearer" {
		t.Fatalf("invalid AnySearch template: %+v", o)
	}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	o.Headers["Authorization"] = "do-not-preserve"
	again := List()
	if again[0].Headers["Authorization"] != "" {
		t.Fatal("mutation leaked into static catalogue")
	}
}
