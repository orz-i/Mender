package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ToolGetter loads a Tool by stable Mender identity.
type ToolGetter interface {
	GetTool(context.Context, domain.ToolID) (domain.Tool, error)
}

// DescribeTool returns one canonical Tool.
type DescribeTool struct {
	tools ToolGetter
}

func NewDescribeTool(tools ToolGetter) DescribeTool {
	return DescribeTool{tools: tools}
}

func (uc DescribeTool) Execute(
	ctx context.Context,
	id domain.ToolID,
) (domain.Tool, error) {
	tool, err := uc.tools.GetTool(ctx, id)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("describe tool: %w", err)
	}
	return tool, nil
}
