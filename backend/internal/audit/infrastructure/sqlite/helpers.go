package sqlite

import (
	"fmt"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func parseActivationProviderID(raw string) (catalogdomain.ProviderID, error) {
	id, err := catalogdomain.ParseProviderID(raw)
	if err != nil {
		return "", fmt.Errorf("parse provider activation provider id: %w", err)
	}
	return id, nil
}

func parseAuditTime(raw string) (time.Time, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse audit time: %w", err)
	}
	return value, nil
}
