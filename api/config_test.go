package api_test

import (
	"testing"

	"github.com/bitnami/charts-syncer/api"
)

func TestValidate(t *testing.T) {
	config := &api.Config{
		Source: &api.Source{
			Repo: &api.Repo{
				Url:  "ht//:fake.source.com",
				Kind: api.Kind_CHARTMUSEUM,
				Auth: &api.Auth{
					Username: "user",
					Password: "password",
				},
			},
		},
		Target: &api.Target{
			Repo: &api.Repo{
				Url:  "http://fake.target.com",
				Kind: api.Kind_CHARTMUSEUM,
				Auth: &api.Auth{
					Username: "user",
					Password: "password",
				},
			},
		},
	}

	if err := config.Validate(); err == nil {
		t.Errorf("expected error but got nothing")
	} else {
		expectedError := `"source.repo.url" should be a valid URL: parse "ht//:fake.source.com": invalid URI for request`
		if err.Error() != expectedError {
			t.Errorf("incorrect error, got: \n %s \n, want: \n %s \n", err.Error(), expectedError)
		}
	}
}

func TestValidateCherryPickedCharts(t *testing.T) {
	validConfig := &api.Config{
		Source: &api.Source{
			Repo: &api.Repo{
				Url:  "http://fake.source.com",
				Kind: api.Kind_OCI,
			},
		},
		Target: &api.Target{
			Repo: &api.Repo{
				Url:  "http://fake.target.com",
				Kind: api.Kind_OCI,
			},
		},
	}

	tests := []struct {
		name          string
		cherryPicked  []*api.CherryPickedChart
		expectedError string
	}{
		{
			name: "valid cherry picked charts",
			cherryPicked: []*api.CherryPickedChart{
				{
					Name:     "mongodb",
					Versions: []string{"1.1.0", "2.5.0"},
				},
				{
					Name:     "postgresql",
					Versions: []string{"3.2.1"},
				},
			},
			expectedError: "",
		},
		{
			name:          "no cherry picked charts",
			cherryPicked:  []*api.CherryPickedChart{},
			expectedError: "",
		},
		{
			name: "empty chart name",
			cherryPicked: []*api.CherryPickedChart{
				{
					Name:     "",
					Versions: []string{"1.1.0"},
				},
			},
			expectedError: `"cherry_picked_charts[0].name" cannot be empty`,
		},
		{
			name: "duplicate chart names",
			cherryPicked: []*api.CherryPickedChart{
				{
					Name:     "mongodb",
					Versions: []string{"1.1.0"},
				},
				{
					Name:     "mongodb",
					Versions: []string{"2.5.0"},
				},
			},
			expectedError: `"cherry_picked_charts" contains duplicate chart name: "mongodb"`,
		},
		{
			name: "empty versions array",
			cherryPicked: []*api.CherryPickedChart{
				{
					Name:     "mongodb",
					Versions: []string{},
				},
			},
			expectedError: `"cherry_picked_charts[0].versions" cannot be empty for chart "mongodb"`,
		},
		{
			name: "empty version string",
			cherryPicked: []*api.CherryPickedChart{
				{
					Name:     "mongodb",
					Versions: []string{"1.1.0", ""},
				},
			},
			expectedError: `"cherry_picked_charts[0].versions[1]" cannot be empty for chart "mongodb"`,
		},
		{
			name: "whitespace-only version string",
			cherryPicked: []*api.CherryPickedChart{
				{
					Name:     "mongodb",
					Versions: []string{"1.1.0", "   "},
				},
			},
			expectedError: `"cherry_picked_charts[0].versions[1]" cannot be empty for chart "mongodb"`,
		},
		{
			name: "duplicate versions within same chart",
			cherryPicked: []*api.CherryPickedChart{
				{
					Name:     "mongodb",
					Versions: []string{"1.1.0", "2.5.0", "1.1.0"},
				},
			},
			expectedError: `"cherry_picked_charts[0]" contains duplicate version "1.1.0" for chart "mongodb"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := *validConfig // Copy the valid config
			config.CherryPickedCharts = tt.cherryPicked

			err := config.Validate()

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error '%s' but got nothing", tt.expectedError)
				} else if err.Error() != tt.expectedError {
					t.Errorf("incorrect error, got: \n %s \n, want: \n %s \n", err.Error(), tt.expectedError)
				}
			}
		})
	}
}
