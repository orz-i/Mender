package domain

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	ruleIDPrefix = "rule_"
	ruleIDBytes  = 16
	ruleIDHexLen = ruleIDBytes * 2
)

// RuleID is Mender's stable opaque identity for one Policy Rule.
type RuleID string

func ParseRuleID(value string) (RuleID, error) {
	if !strings.HasPrefix(value, ruleIDPrefix) {
		return "", fmt.Errorf("rule_id: invalid prefix")
	}
	encoded := strings.TrimPrefix(value, ruleIDPrefix)
	if len(encoded) != ruleIDHexLen {
		return "", fmt.Errorf("rule_id: invalid length")
	}
	if encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("rule_id: must use lowercase hexadecimal encoding")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("rule_id: must use hexadecimal encoding")
	}
	return RuleID(value), nil
}

func (id RuleID) String() string {
	return string(id)
}
