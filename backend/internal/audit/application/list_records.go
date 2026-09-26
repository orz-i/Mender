package application

import (
	"context"
	"fmt"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
)

var ErrInvalidLimit = fmt.Errorf("audit: limit must be between 1 and 200")

type RecordLister interface {
	ListRecords(context.Context, int) ([]auditdomain.Record, error)
}

type ListRecords struct {
	records RecordLister
}

func NewListRecords(records RecordLister) ListRecords {
	return ListRecords{records: records}
}

func (uc ListRecords) Execute(
	ctx context.Context,
	limit int,
) ([]auditdomain.Record, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrInvalidLimit
	}
	records, err := uc.records.ListRecords(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit records: %w", err)
	}
	return records, nil
}
