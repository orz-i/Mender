package domain

import (
	"fmt"
	"net/url"
	"strings"
)

// Offering is a reviewed, inert integration template. It is neither an
// executable Plugin Installation nor an activated Provider/Resource.
type Offering struct {
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

func (o Offering) Validate() error {
	if !strings.HasPrefix(o.OfferingID, "io.mender.integration.") ||
		strings.TrimSpace(o.DisplayName) == "" || strings.TrimSpace(o.Description) == "" ||
		o.Kind != "remote-mcp" || o.DriverPluginID != "io.mender.provider.mcp" ||
		o.Transport != "streamable-http" || o.Authentication != "optional-bearer" {
		return fmt.Errorf("integration offering: invalid kind, identity or driver")
	}
	endpoint, err := url.Parse(o.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return fmt.Errorf("integration offering: endpoint requires public HTTPS and no embedded credentials")
	}
	if o.DefaultNamespace == "" || o.SourceURL == "" || !strings.HasPrefix(o.SourceURL, "https://") {
		return fmt.Errorf("integration offering: missing namespace or source")
	}
	for key, value := range o.Headers {
		if strings.EqualFold(key, "Authorization") || key == "" || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("integration offering: unsafe static header")
		}
	}
	return nil
}

func (o Offering) Clone() Offering {
	out := o
	out.Headers = make(map[string]string, len(o.Headers))
	for key, value := range o.Headers {
		out.Headers[key] = value
	}
	return out
}
