package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

const randomIDBytes = 16

type Generator struct {
	reader io.Reader
}

func NewGenerator() *Generator {
	return &Generator{reader: rand.Reader}
}

func (g *Generator) NewRuleID() (policydomain.RuleID, error) {
	random := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(g.reader, random); err != nil {
		return "", fmt.Errorf("generate policy rule id: %w", err)
	}
	id, err := policydomain.ParseRuleID("rule_" + hex.EncodeToString(random))
	if err != nil {
		return "", fmt.Errorf("generate policy rule id: validate: %w", err)
	}
	return id, nil
}
