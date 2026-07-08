package printer

import (
	"bytes"
	"testing"
	"time"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testGCPProviderType      = "google"
	testGCPProviderSource    = "registry.terraform.io/hashicorp/google"
	testGCPVersionConstraint = "~> 4.0"
	testProjectID            = "my-project"
	testNilInputName         = "nil input"
	testEmptySliceName       = "empty slice"
	testProviderID           = "test-id"
	testAWSProviderType      = "aws"
	testAWSProviderSource    = "registry.terraform.io/hashicorp/aws"
	testAWSVersionConstraint = "~> 5.0"
	testRegionField          = "region"
	testSliceOfStructsName   = "slice of structs"
	testFirstProviderID      = "id1"
	testSecondProviderID     = "id2"
	testEastRegion           = "us-east-1"
	testWestRegion           = "us-west-2"
	testProjectField         = "project"
)

type UnknownStruct struct {
	Field string `json:"field"`
}
type ModuleProvider struct {
	Id                string                 `json:"id"`
	Description       *string                `json:"description,omitempty"`
	ProviderType      string                 `json:"provider_type"`
	Source            string                 `json:"source"`
	VersionConstraint string                 `json:"version_constraint"`
	Configuration     map[string]interface{} `json:"configuration"`
	CreatedAt         time.Time              `json:"created_at"`
}

func TestJsonPrinter_Write(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name: "simple struct",
			input: ModuleProvider{
				Id:                testProviderID,
				Description:       ref.Ref("Test AWS provider"),
				ProviderType:      testAWSProviderType,
				Source:            testAWSProviderSource,
				VersionConstraint: testAWSVersionConstraint,
				Configuration:     map[string]interface{}{testRegionField: testWestRegion},
				CreatedAt:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			expected: `{
  "id": "test-id",
  "description": "Test AWS provider",
  "provider_type": "aws",
  "source": "registry.terraform.io/hashicorp/aws",
  "version_constraint": "~> 5.0",
  "configuration": {
    "region": "us-west-2"
  },
  "created_at": "2023-01-01T12:00:00Z"
}
`,
		},
		{
			name: testSliceOfStructsName,
			input: []ModuleProvider{
				{
					Id:                testFirstProviderID,
					Description:       ref.Ref("AWS provider for production"),
					ProviderType:      testAWSProviderType,
					Source:            testAWSProviderSource,
					VersionConstraint: testAWSVersionConstraint,
					Configuration:     map[string]interface{}{testRegionField: testEastRegion},
					CreatedAt:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				},
				{
					Id:                testSecondProviderID,
					Description:       ref.Ref("GCP provider for staging"),
					ProviderType:      testGCPProviderType,
					Source:            testGCPProviderSource,
					VersionConstraint: testGCPVersionConstraint,
					Configuration:     map[string]interface{}{testProjectField: testProjectID},
					CreatedAt:         time.Date(2023, 2, 1, 10, 30, 0, 0, time.UTC),
				},
			},
			expected: `[
  {
    "id": "id1",
    "description": "AWS provider for production",
    "provider_type": "aws",
    "source": "registry.terraform.io/hashicorp/aws",
    "version_constraint": "~> 5.0",
    "configuration": {
      "region": "us-east-1"
    },
    "created_at": "2023-01-01T12:00:00Z"
  },
  {
    "id": "id2",
    "description": "GCP provider for staging",
    "provider_type": "google",
    "source": "registry.terraform.io/hashicorp/google",
    "version_constraint": "~> 4.0",
    "configuration": {
      "project": "my-project"
    },
    "created_at": "2023-02-01T10:30:00Z"
  }
]
`,
		},
		{
			name:     testNilInputName,
			input:    nil,
			expected: "null\n",
		},
		{
			name:     testEmptySliceName,
			input:    []ModuleProvider{},
			expected: "[]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := &JsonPrinter{}
			var buf bytes.Buffer

			err := printer.Write(&buf, tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestYamlPrinter_Write(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name: "simple struct",
			input: ModuleProvider{
				Id:                testProviderID,
				Description:       ref.Ref("Test AWS provider"),
				ProviderType:      testAWSProviderType,
				Source:            testAWSProviderSource,
				VersionConstraint: testAWSVersionConstraint,
				Configuration:     map[string]interface{}{testRegionField: testWestRegion},
				CreatedAt:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			expected: `configuration:
  region: us-west-2
created_at: "2023-01-01T12:00:00Z"
description: Test AWS provider
id: test-id
provider_type: aws
source: registry.terraform.io/hashicorp/aws
version_constraint: ~> 5.0
`,
		},
		{
			name: testSliceOfStructsName,
			input: []ModuleProvider{
				{
					Id:                testFirstProviderID,
					Description:       ref.Ref("AWS provider for production"),
					ProviderType:      testAWSProviderType,
					Source:            testAWSProviderSource,
					VersionConstraint: testAWSVersionConstraint,
					Configuration:     map[string]interface{}{testRegionField: testEastRegion},
					CreatedAt:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				},
				{
					Id:                testSecondProviderID,
					Description:       ref.Ref("GCP provider for staging"),
					ProviderType:      testGCPProviderType,
					Source:            testGCPProviderSource,
					VersionConstraint: testGCPVersionConstraint,
					Configuration:     map[string]interface{}{testProjectField: testProjectID},
					CreatedAt:         time.Date(2023, 2, 1, 10, 30, 0, 0, time.UTC),
				},
			},
			expected: `- configuration:
    region: us-east-1
  created_at: "2023-01-01T12:00:00Z"
  description: AWS provider for production
  id: id1
  provider_type: aws
  source: registry.terraform.io/hashicorp/aws
  version_constraint: ~> 5.0
- configuration:
    project: my-project
  created_at: "2023-02-01T10:30:00Z"
  description: GCP provider for staging
  id: id2
  provider_type: google
  source: registry.terraform.io/hashicorp/google
  version_constraint: ~> 4.0
`,
		},
		{
			name:     testNilInputName,
			input:    nil,
			expected: "null\n",
		},
		{
			name:     testEmptySliceName,
			input:    []ModuleProvider{},
			expected: "[]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := &YamlPrinter{}
			var buf bytes.Buffer

			err := printer.Write(&buf, tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestTablePrinter_Write(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		expectError bool
		errorMsg    string
		contains    []string
	}{
		{
			name: "single struct",
			input: ModuleProvider{
				Id:                testProviderID,
				Description:       ref.Ref("Test AWS provider"),
				ProviderType:      testAWSProviderType,
				Source:            testAWSProviderSource,
				VersionConstraint: testAWSVersionConstraint,
				Configuration:     map[string]interface{}{testRegionField: testWestRegion},
				CreatedAt:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			contains: []string{"Id", tableFieldDescription, tableFieldProviderType, tableFieldSource, tableFieldVersionConstraint, tableFieldConfiguration, tableFieldCreatedAt, testProviderID, "Test AWS provider", testAWSProviderType, testAWSProviderSource, testAWSVersionConstraint},
		},
		{
			name: testSliceOfStructsName,
			input: []ModuleProvider{
				{
					Id:                testFirstProviderID,
					Description:       ref.Ref("AWS provider for production"),
					ProviderType:      testAWSProviderType,
					Source:            testAWSProviderSource,
					VersionConstraint: testAWSVersionConstraint,
					Configuration:     map[string]interface{}{testRegionField: testEastRegion},
					CreatedAt:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				},
				{
					Id:                testSecondProviderID,
					Description:       ref.Ref("GCP provider for staging"),
					ProviderType:      testGCPProviderType,
					Source:            testGCPProviderSource,
					VersionConstraint: testGCPVersionConstraint,
					Configuration:     map[string]interface{}{testProjectField: testProjectID},
					CreatedAt:         time.Date(2023, 2, 1, 10, 30, 0, 0, time.UTC),
				},
			},
			contains: []string{"Id", tableFieldDescription, tableFieldProviderType, tableFieldSource, tableFieldVersionConstraint, tableFieldConfiguration, tableFieldCreatedAt, testFirstProviderID, testSecondProviderID, "AWS provider for production", "GCP provider for staging", testAWSProviderType, testGCPProviderType, testAWSProviderSource, testGCPProviderSource, testAWSVersionConstraint, testGCPVersionConstraint},
		},
		{
			name: "struct with nil pointer",
			input: ModuleProvider{
				Id:                testProviderID,
				Description:       nil,
				ProviderType:      testAWSProviderType,
				Source:            testAWSProviderSource,
				VersionConstraint: testAWSVersionConstraint,
				Configuration:     nil,
				CreatedAt:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			contains: []string{"Id", tableFieldDescription, tableFieldProviderType, tableFieldSource, tableFieldVersionConstraint, tableFieldConfiguration, tableFieldCreatedAt, testProviderID, "-", testAWSProviderType, testAWSProviderSource, testAWSVersionConstraint},
		},
		{
			name:     testNilInputName,
			input:    nil,
			contains: []string{""},
		},
		{
			name:     testEmptySliceName,
			input:    []ModuleProvider{},
			contains: []string{""},
		},
		{
			name:        "non-slice, non-struct input",
			input:       "invalid string",
			expectError: true,
			errorMsg:    "provided object is not a slice or a struct",
		},
		{
			name:        "unknown struct type",
			input:       []UnknownStruct{{Field: "test"}},
			expectError: true,
			errorMsg:    "no table columns defined for type UnknownStruct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := &TablePrinter{}
			var buf bytes.Buffer

			err := printer.Write(&buf, tt.input)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				output := buf.String()
				for _, expected := range tt.contains {
					if expected != "" {
						assert.Contains(t, output, expected)
					}
				}
			}
		})
	}
}
