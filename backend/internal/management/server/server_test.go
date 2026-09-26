package server

import "testing"

func TestValidateLoopbackAddress(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"127.0.0.1:8080",
		"[::1]:8080",
		"localhost:8080",
	} {
		if err := ValidateLoopbackAddress(address); err != nil {
			t.Errorf("ValidateLoopbackAddress(%q) error = %v", address, err)
		}
	}

	for _, address := range []string{
		":8080",
		"0.0.0.0:8080",
		"192.0.2.1:8080",
	} {
		if err := ValidateLoopbackAddress(address); err == nil {
			t.Errorf("ValidateLoopbackAddress(%q) expected error", address)
		}
	}
}
