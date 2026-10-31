package mcpserver

import (
	"errors"
	"regexp"
	"strings"
	"time"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

const maxItems = 100

// IDs are single path segments. Percent escapes, slashes, traversal pairs and whitespace
// are excluded so generated URL construction cannot reinterpret tool scope.
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validateIDs(ids ...string) error {
	for _, id := range ids {
		if !identifier.MatchString(id) || strings.Contains(id, "..") {
			return errors.New("IDs must contain 1 to 128 letters, digits, dots, underscores or hyphens (no traversal pairs), starting with a letter or digit")
		}
	}
	return nil
}

type ListInput struct {
	OrgID string `json:"org_id" jsonschema:"Organization ID; always explicit"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum items from 1 to 100; default 50"`
	Page  string `json:"page,omitempty" jsonschema:"Opaque next_page_token from a previous result"`
}
type ProjectInput struct {
	OrgID     string `json:"org_id"`
	ProjectID string `json:"project_id"`
}
type EnvironmentsInput struct {
	OrgID     string `json:"org_id"`
	ProjectID string `json:"project_id"`
	Limit     int    `json:"limit,omitempty"`
	Page      string `json:"page,omitempty"`
}
type EnvironmentInput struct {
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	EnvironmentID string `json:"environment_id"`
}
type ModulesInput struct {
	OrgID string `json:"org_id"`
	Limit int    `json:"limit,omitempty"`
}
type VersionsInput struct {
	OrgID    string `json:"org_id"`
	ModuleID string `json:"module_id"`
	Limit    int    `json:"limit,omitempty"`
	Page     string `json:"page,omitempty"`
}
type VersionInput struct {
	OrgID     string `json:"org_id"`
	ModuleID  string `json:"module_id"`
	VersionID string `json:"version_id"`
}
type ResourceTypeInput struct {
	OrgID  string `json:"org_id"`
	TypeID string `json:"type_id"`
}

type Page[T any] struct {
	Items         []T     `json:"items"`
	NextPageToken *string `json:"next_page_token,omitempty"`
	Truncated     bool    `json:"truncated"`
}
type ProjectMetadata struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}
type EnvironmentMetadata struct {
	ID              string `json:"id"`
	ProjectID       string `json:"project_id"`
	DisplayName     string `json:"display_name"`
	EnvironmentType string `json:"environment_type"`
	Status          string `json:"status"`
}
type ModuleMetadata struct {
	ID              string `json:"id"`
	ResourceType    string `json:"resource_type"`
	Status          string `json:"status"`
	ResourceVersion int64  `json:"resource_version"`
}
type VersionMetadata struct {
	VersionID           string    `json:"version_id"`
	SemanticVersion     *string   `json:"semantic_version,omitempty"`
	LifecycleStatus     string    `json:"lifecycle_status"`
	VerificationStatus  string    `json:"verification_status"`
	ResourceTypeVersion *int64    `json:"resource_type_version,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}
type ResourceTypeMetadata struct {
	ID                  string `json:"id"`
	CatalogueStatus     string `json:"catalogue_status"`
	ResourceVersion     int64  `json:"resource_version"`
	BuiltIn             bool   `json:"built_in"`
	DeveloperAccessible bool   `json:"developer_accessible"`
	OutputFieldCount    int    `json:"output_field_count"`
}

func projectMetadata(p cp.Project) ProjectMetadata {
	return ProjectMetadata{p.Id, p.DisplayName, string(p.Status), p.CreatedAt}
}
func environmentMetadata(e cp.Environment) EnvironmentMetadata {
	return EnvironmentMetadata{e.Id, e.ProjectId, e.DisplayName, e.EnvTypeId, string(e.Status)}
}
func versionMetadata(v cp.CoreModuleVersionDetail) VersionMetadata {
	return VersionMetadata{v.Version.OpaqueVersionId, v.Version.SemanticVersion, string(v.Version.LifecycleStatus), string(v.Version.VerificationStatus), v.Version.ResourceTypeVersion, v.CreatedAt}
}
func pagination(limit int, page string) (int, *string, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > maxItems || len(page) > 1024 {
		return 0, nil, errors.New("limit must be 1 to 100 and page must be at most 1024 bytes")
	}
	if page == "" {
		return limit, nil, nil
	}
	return limit, &page, nil
}
func boundedPage[T any](items []T, limit int, next *string) Page[T] {
	truncated := len(items) > limit
	if truncated {
		items = items[:limit]
	}
	if items == nil {
		items = []T{}
	}
	if next != nil && len(*next) > 1024 {
		next = nil
		truncated = true
	}
	return Page[T]{items, next, truncated}
}
