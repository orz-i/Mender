package identity

import (
	"bytes"
	"testing"
)

func TestGeneratorProducesInstallationID(t *testing.T) {
	t.Parallel()

	generator := &Generator{
		reader: bytes.NewReader([]byte{
			0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
			0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
		}),
	}
	id, err := generator.NewInstallationID()
	if err != nil {
		t.Fatalf("NewInstallationID() error = %v", err)
	}
	if got, want := id.String(), "sinst_00112233445566778899aabbccddeeff"; got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}

	generator = &Generator{
		reader: bytes.NewReader([]byte{
			0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88,
			0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, 0x00,
		}),
	}
	deploymentID, err := generator.NewDeploymentID()
	if err != nil {
		t.Fatalf("NewDeploymentID() error = %v", err)
	}
	if got, want := deploymentID.String(), "sdep_ffeeddccbbaa99887766554433221100"; got != want {
		t.Fatalf("deployment id = %q, want %q", got, want)
	}
}
