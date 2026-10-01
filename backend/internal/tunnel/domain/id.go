package domain

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	tunnelIDPrefix = "tun_"
	tunnelIDBytes  = 16
	tunnelIDHexLen = tunnelIDBytes * 2
)

// TunnelID is Mender's stable opaque identity for one Tunnel configuration.
type TunnelID string

func ParseTunnelID(value string) (TunnelID, error) {
	if !strings.HasPrefix(value, tunnelIDPrefix) {
		return "", fmt.Errorf("tunnel_id: invalid prefix")
	}
	encoded := strings.TrimPrefix(value, tunnelIDPrefix)
	if len(encoded) != tunnelIDHexLen {
		return "", fmt.Errorf("tunnel_id: invalid length")
	}
	if encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("tunnel_id: must use lowercase hexadecimal encoding")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("tunnel_id: must use hexadecimal encoding")
	}
	return TunnelID(value), nil
}

func (id TunnelID) String() string {
	return string(id)
}
