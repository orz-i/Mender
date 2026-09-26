package application

import (
	"context"
	"errors"
	"testing"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
)

func TestListRecordsValidatesLimit(t *testing.T) {
	t.Parallel()

	useCase := NewListRecords(fakeLister{})
	if _, err := useCase.Execute(context.Background(), 0); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("Execute(0) error = %v", err)
	}
	if _, err := useCase.Execute(context.Background(), 201); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("Execute(201) error = %v", err)
	}
}

type fakeLister struct{}

func (fakeLister) ListRecords(
	context.Context,
	int,
) ([]auditdomain.Record, error) {
	return nil, nil
}
