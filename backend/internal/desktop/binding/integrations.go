package binding

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/integration/domain"
)

type IntegrationOffering struct {
	OfferingID       string            `json:"offeringId"`
	DisplayName      string            `json:"displayName"`
	Description      string            `json:"description"`
	Kind             string            `json:"kind"`
	DriverPluginID   string            `json:"driverPluginId"`
	Transport        string            `json:"transport"`
	Endpoint         string            `json:"endpoint"`
	Headers          map[string]string `json:"headers"`
	DefaultNamespace string            `json:"defaultNamespace"`
	Authentication   string            `json:"authentication"`
	SourceURL        string            `json:"sourceUrl"`
}
type IntegrationOfferingList struct {
	Integrations []IntegrationOffering `json:"integrations"`
}

func projectIntegrationOffering(item domain.Offering) IntegrationOffering {
	return IntegrationOffering{
		OfferingID: item.OfferingID, DisplayName: item.DisplayName, Description: item.Description,
		Kind: item.Kind, DriverPluginID: item.DriverPluginID, Transport: item.Transport, Endpoint: item.Endpoint,
		Headers: item.Headers, DefaultNamespace: item.DefaultNamespace,
		Authentication: item.Authentication, SourceURL: item.SourceURL,
	}
}
func (s *Service) ListIntegrationOfferings() (IntegrationOfferingList, error) {
	if s == nil || s.runtime == nil {
		return IntegrationOfferingList{}, fmt.Errorf("desktop integrations: runtime unavailable")
	}
	items, err := s.runtime.ListIntegrationOfferings(s.ctx)
	if err != nil {
		return IntegrationOfferingList{}, err
	}
	result := IntegrationOfferingList{Integrations: make([]IntegrationOffering, 0, len(items))}
	for _, item := range items {
		result.Integrations = append(result.Integrations, projectIntegrationOffering(item))
	}
	return result, nil
}
