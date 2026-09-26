package cli

import (
	"encoding/json"
	"fmt"
)

func (a *app) writeValue(text string, value any) error {
	if a.output == "json" {
		encoder := json.NewEncoder(a.options.Stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(value); err != nil {
			return fmt.Errorf("encode JSON output: %w", err)
		}
		return nil
	}
	_, err := fmt.Fprintln(a.options.Stdout, text)
	return err
}
