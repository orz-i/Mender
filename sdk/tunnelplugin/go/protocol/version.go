// Package protocol defines Tunnel Transport Protocol v1 version negotiation.
package protocol

import (
	"fmt"
	"slices"
)

const Version1 uint32 = 1

var supportedVersions = []uint32{Version1}

func SupportedVersions() []uint32 { return slices.Clone(supportedVersions) }
func Negotiate(peer []uint32) (uint32, error) {
	for _, supported := range supportedVersions {
		if slices.Contains(peer, supported) {
			return supported, nil
		}
	}
	return 0, fmt.Errorf("tunnel transport protocol: no mutually supported version")
}
