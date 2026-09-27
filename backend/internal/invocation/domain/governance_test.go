package domain

import "testing"

func TestGovernanceFactsUnknownAndKnown(t *testing.T) {
	t.Parallel()

	unknown := UnknownGovernanceFacts()
	if unknown.Knowledge() != FactKnowledgeUnknown {
		t.Fatalf("unknown knowledge = %q", unknown.Knowledge())
	}
	if len(unknown.Effects()) != 0 || len(unknown.Resources()) != 0 {
		t.Fatalf("unknown facts unexpectedly contained values")
	}

	resource, err := NewResourceFact("network.host", "api.example.test")
	if err != nil {
		t.Fatal(err)
	}
	known, err := KnownGovernanceFacts(
		[]EffectFact{"write", "read", "write"},
		[]ResourceFact{resource},
	)
	if err != nil {
		t.Fatal(err)
	}
	if known.Knowledge() != FactKnowledgeKnown {
		t.Fatalf("known knowledge = %q", known.Knowledge())
	}
	effects := known.Effects()
	if len(effects) != 2 || effects[0] != "read" || effects[1] != "write" {
		t.Fatalf("effects = %v", effects)
	}
	resources := known.Resources()
	if len(resources) != 1 ||
		resources[0].Kind() != "network.host" ||
		resources[0].Value() != "api.example.test" {
		t.Fatalf("resources = %#v", resources)
	}
}

func TestGovernanceFactsRejectUnboundedValues(t *testing.T) {
	t.Parallel()

	if _, err := NewResourceFact("", "value"); err == nil {
		t.Fatal("NewResourceFact() accepted empty kind")
	}
	if _, err := KnownGovernanceFacts([]EffectFact{""}, nil); err == nil {
		t.Fatal("KnownGovernanceFacts() accepted empty effect")
	}
}
