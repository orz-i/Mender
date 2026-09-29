package domain

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	skillIDPrefix        = "skill_"
	installationIDPrefix = "sinst_"
	deploymentIDPrefix   = "sdep_"
	opaqueIDBytes        = 16
	opaqueIDHexLength    = opaqueIDBytes * 2
)

// SkillID is Mender's stable opaque identity for one logical Skill lineage.
type SkillID string

// ParseSkillID validates and returns a SkillID.
func ParseSkillID(value string) (SkillID, error) {
	if err := validateOpaqueID(value, skillIDPrefix, "skill_id"); err != nil {
		return "", err
	}
	return SkillID(value), nil
}

func (id SkillID) String() string {
	return string(id)
}

// InstallationID is Mender's stable opaque identity for one exact Skill
// installation/content snapshot.
type InstallationID string

// ParseInstallationID validates and returns an InstallationID.
func ParseInstallationID(value string) (InstallationID, error) {
	if err := validateOpaqueID(value, installationIDPrefix, "skill_installation_id"); err != nil {
		return "", err
	}
	return InstallationID(value), nil
}

func (id InstallationID) String() string {
	return string(id)
}

// DeploymentID is Mender's stable opaque identity for one exact Skill
// deployment/materialization.
type DeploymentID string

// ParseDeploymentID validates and returns a DeploymentID.
func ParseDeploymentID(value string) (DeploymentID, error) {
	if err := validateOpaqueID(value, deploymentIDPrefix, "skill_deployment_id"); err != nil {
		return "", err
	}
	return DeploymentID(value), nil
}

func (id DeploymentID) String() string {
	return string(id)
}

func validateOpaqueID(value, prefix, field string) error {
	if !strings.HasPrefix(value, prefix) {
		return fmt.Errorf("%s: invalid prefix", field)
	}
	encoded := strings.TrimPrefix(value, prefix)
	if len(encoded) != opaqueIDHexLength {
		return fmt.Errorf("%s: invalid length", field)
	}
	if encoded != strings.ToLower(encoded) {
		return fmt.Errorf("%s: must use lowercase hexadecimal encoding", field)
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return fmt.Errorf("%s: must use hexadecimal encoding", field)
	}
	return nil
}
