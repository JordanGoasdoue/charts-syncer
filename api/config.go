package api

import (
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

// Validate validates the config file is correct
func (c *Config) Validate() error {
	if repo := c.GetSource().GetRepo(); repo != nil {
		switch k := repo.GetKind(); k {
		case Kind_CHARTMUSEUM, Kind_HELM, Kind_HARBOR, Kind_OCI:
			if _, err := url.ParseRequestURI(repo.GetUrl()); err != nil {
				return errors.Errorf(`"source.repo.url" should be a valid URL: %v`, err)
			}
		}
	}
	if repo := c.GetTarget().GetRepo(); repo != nil {
		switch k := repo.GetKind(); k {
		case Kind_CHARTMUSEUM, Kind_HELM, Kind_HARBOR, Kind_OCI:
			if _, err := url.ParseRequestURI(repo.GetUrl()); err != nil {
				return errors.Errorf(`"target.repo.url" should be a valid URL: %v`, err)
			}
		}
	}

	// Authentication
	// Container images
	if auth := c.GetSource().GetContainers().GetAuth(); auth != nil {
		if auth.Username == "" || auth.Password == "" || auth.Registry == "" {
			return errors.Errorf(`"source.containers.auth" "registry", "username"" and "password" are required"`)
		}
	}
	if auth := c.GetTarget().GetContainers().GetAuth(); auth != nil {
		// NOTE: we do not indicate that the registry is empty because this one is set from target.containerRegistry
		// so the user does not need to set it up
		if auth.Username == "" || auth.Password == "" {
			return errors.Errorf(`"target.containers.auth" "username"" and "password" are required"`)
		}
	}
	if repo := c.GetTarget().GetRepo(); repo != nil {
		if repo.GetKind() != Kind_OCI && repo.GetKind() != Kind_LOCAL {
			return errors.Errorf(`"target.repo.kind" should be "OCI" or "LOCAL"`)
		}
	}

	// Validate cherry picked charts
	if err := c.validateCherryPickedCharts(); err != nil {
		return err
	}

	return nil
}

// validateCherryPickedCharts validates the cherry picked charts configuration
func (c *Config) validateCherryPickedCharts() error {
	cherryPickedCharts := c.GetCherryPickedCharts()
	if len(cherryPickedCharts) == 0 {
		return nil // No cherry picked charts to validate
	}

	chartNames := make(map[string]bool)

	for i, chart := range cherryPickedCharts {
		// Validate chart name
		if chart.GetName() == "" {
			return errors.Errorf(`"cherry_picked_charts[%d].name" cannot be empty`, i)
		}

		// Check for duplicate chart names
		if chartNames[chart.GetName()] {
			return errors.Errorf(`"cherry_picked_charts" contains duplicate chart name: "%s"`, chart.GetName())
		}
		chartNames[chart.GetName()] = true

		// Validate versions
		versions := chart.GetVersions()
		if len(versions) == 0 {
			return errors.Errorf(`"cherry_picked_charts[%d].versions" cannot be empty for chart "%s"`, i, chart.GetName())
		}

		versionSet := make(map[string]bool)
		for j, version := range versions {
			// Check for empty versions
			if strings.TrimSpace(version) == "" {
				return errors.Errorf(`"cherry_picked_charts[%d].versions[%d]" cannot be empty for chart "%s"`, i, j, chart.GetName())
			}

			// Check for duplicate versions within the same chart
			if versionSet[version] {
				return errors.Errorf(`"cherry_picked_charts[%d]" contains duplicate version "%s" for chart "%s"`, i, version, chart.GetName())
			}
			versionSet[version] = true
		}
	}

	return nil
}
