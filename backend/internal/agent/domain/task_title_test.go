package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateTaskTitleExplicitSingleLineOnly(t *testing.T) {
	for _, title := range []string{
		"", "审计 UI 信息密度", "Review plugin catalogue", strings.Repeat("任", MaxTaskTitleRunes),
	} {
		if err := ValidateTaskTitle(title); err != nil {
			t.Fatalf("valid title %q rejected: %v", title, err)
		}
	}
	for _, title := range []string{
		" ", " Review", "Review ", "Line\nbreak", "Tab\tchar", "\u2028", "\u202eHidden", strings.Repeat("任", MaxTaskTitleRunes+1), string([]byte{0xff}),
	} {
		if err := ValidateTaskTitle(title); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid title %q accepted: %v", title, err)
		}
	}
}
