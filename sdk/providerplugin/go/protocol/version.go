package protocol

import (
	"fmt"
	"slices"
)

const (
	// Version1 is the first Mender Provider Plugin Protocol version.
	Version1 uint32 = 1
)

var supportedVersions = []uint32{Version1}

// SupportedVersions returns the protocol versions implemented by this SDK.
func SupportedVersions() []uint32 {
	return slices.Clone(supportedVersions)
}

// Negotiate selects the highest mutually supported protocol version.
func Negotiate(peer []uint32) (uint32, error) {
	seen := make(map[uint32]struct{}, len(peer))
	for _, version := range peer {
		if version == 0 {
			continue
		}
		seen[version] = struct{}{}
	}
	for i := len(supportedVersions) - 1; i >= 0; i-- {
		version := supportedVersions[i]
		if _, ok := seen[version]; ok {
			return version, nil
		}
	}
	return 0, fmt.Errorf("provider plugin protocol: no mutually supported version")
}
