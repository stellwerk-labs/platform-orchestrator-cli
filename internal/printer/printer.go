package printer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/gosuri/uitable"
	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"
)

type Printer interface {
	Write(w io.Writer, item interface{}) error
}

const (
	JsonPrinterType  = "json"
	YamlPrinterType  = "yaml"
	TablePrinterType = "table"
	OutputFormatFlag = "out"

	tableTypeActiveResourceNode     = "ActiveResourceNode"
	tableTypeAvailableResourceType  = "AvailableResourceType"
	tableTypeCurrentUser            = "CurrentUser"
	tableTypeDeployment             = "Deployment"
	tableTypeDeploymentManifest     = "DeploymentManifest"
	tableTypeDeploymentSummary      = "DeploymentSummary"
	tableTypeEnvironment            = "Environment"
	tableTypeEnvironmentTypeSummary = "EnvironmentTypeSummary"
	tableTypeModule                 = "Module"
	tableTypeModuleProvider         = "ModuleProvider"
	tableTypeModuleProviderSummary  = "ModuleProviderSummary"
	tableTypeModuleSummary          = "ModuleSummary"
	tableTypeProject                = "Project"
	tableTypeResourceType           = "ResourceType"
	tableTypeRuleSummary            = "RuleSummary"
	tableTypeRunner                 = "Runner"
	tableTypeRunnerRuleSummary      = "RunnerRuleSummary"
	tableTypeRunnerSummary          = "RunnerSummary"

	tableFieldBuiltIn                 = "BuiltIn"
	tableFieldCompletedAt             = "CompletedAt"
	tableFieldConfiguration           = "Configuration"
	tableFieldCoprovisioned           = "Coprovisioned"
	tableFieldCreatedAt               = "CreatedAt"
	tableFieldDependencies            = "Dependencies"
	tableFieldDescription             = "Description"
	tableFieldDisplayName             = "DisplayName"
	tableFieldEnvId                   = "EnvId"
	tableFieldEnvTypeId               = "EnvTypeId"
	tableFieldId                      = "Id"
	tableFieldIsDeveloperAccessible   = "IsDeveloperAccessible"
	tableFieldManifest                = "Manifest"
	tableFieldMode                    = "Mode"
	tableFieldModuleId                = "ModuleId"
	tableFieldModuleInputs            = "ModuleInputs"
	tableFieldModuleParams            = "ModuleParams"
	tableFieldModuleSource            = "ModuleSource"
	tableFieldModuleVersion           = "ModuleVersion"
	tableFieldName                    = "Name"
	tableFieldOrganizationMemberships = "OrganizationMemberships"
	tableFieldOrgId                   = "OrgId"
	tableFieldPlanOnly                = "PlanOnly"
	tableFieldProjectId               = "ProjectId"
	tableFieldProviderMapping         = "ProviderMapping"
	tableFieldProviderType            = "ProviderType"
	tableFieldResourceClass           = "ResourceClass"
	tableFieldResourceType            = "ResourceType"
	tableFieldRunnerId                = "RunnerId"
	tableFieldRunnerType              = "RunnerType"
	tableFieldSource                  = "Source"
	tableFieldStatus                  = "Status"
	tableFieldUpdatedAt               = "UpdatedAt"
	tableFieldUuid                    = "Uuid"
	tableFieldVersionConstraint       = "VersionConstraint"
	tableFieldVersionId               = "VersionId"
	tableFieldWorkloads               = "Workloads"
)

type JsonPrinter struct{}

func (p *JsonPrinter) Write(w io.Writer, item interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(item)
}

type YamlPrinter struct{}

func (p *YamlPrinter) Write(w io.Writer, item interface{}) error {
	obj, err := yaml.Marshal(item)
	if err != nil {
		return err
	}
	if _, err = w.Write(obj); err != nil {
		return err
	}
	return nil
}

var tableColumns = map[string][]string{
	tableTypeActiveResourceNode:     {tableFieldId, tableFieldResourceType, tableFieldModuleId, tableFieldModuleVersion},
	tableTypeDeployment:             {tableFieldId, tableFieldProjectId, tableFieldEnvId, tableFieldStatus, tableFieldMode, tableFieldPlanOnly, tableFieldCreatedAt, tableFieldCompletedAt, tableFieldManifest, tableFieldRunnerId},
	tableTypeDeploymentManifest:     {tableFieldWorkloads},
	tableTypeDeploymentSummary:      {tableFieldId, tableFieldProjectId, tableFieldEnvId, tableFieldStatus, tableFieldMode, tableFieldPlanOnly, tableFieldCreatedAt, tableFieldCompletedAt},
	tableTypeEnvironmentTypeSummary: {tableFieldId, tableFieldDisplayName, tableFieldCreatedAt},
	tableTypeEnvironment:            {tableFieldId, tableFieldDisplayName, tableFieldEnvTypeId, tableFieldCreatedAt},
	tableTypeModule:                 {tableFieldId, tableFieldDescription, tableFieldOrgId, tableFieldResourceType, tableFieldModuleSource, tableFieldModuleParams, tableFieldModuleInputs, tableFieldProviderMapping, tableFieldDependencies, tableFieldCoprovisioned, tableFieldUpdatedAt, tableFieldVersionId, tableFieldCreatedAt},
	tableTypeModuleSummary:          {tableFieldId, tableFieldDescription, tableFieldResourceType, tableFieldModuleSource, tableFieldVersionId, tableFieldCreatedAt},
	tableTypeModuleProvider:         {tableFieldId, tableFieldDescription, tableFieldProviderType, tableFieldSource, tableFieldVersionConstraint, tableFieldConfiguration, tableFieldCreatedAt},
	tableTypeModuleProviderSummary:  {tableFieldId, tableFieldDescription, tableFieldProviderType, tableFieldSource, tableFieldCreatedAt},
	tableTypeProject:                {tableFieldId, tableFieldDisplayName, tableFieldUuid, tableFieldCreatedAt},
	tableTypeAvailableResourceType:  {tableFieldId, tableFieldName},
	tableTypeResourceType:           {tableFieldId, tableFieldBuiltIn, tableFieldDescription, tableFieldIsDeveloperAccessible, tableFieldCreatedAt},
	tableTypeRuleSummary:            {tableFieldId, tableFieldResourceType, tableFieldResourceClass, tableFieldModuleId, tableFieldCreatedAt},
	tableTypeRunner:                 {tableFieldId, tableFieldOrgId},
	tableTypeRunnerRuleSummary:      {tableFieldId, tableFieldRunnerId, tableFieldCreatedAt},
	tableTypeRunnerSummary:          {tableFieldId, tableFieldDescription, tableFieldRunnerType, tableFieldCreatedAt},
	tableTypeCurrentUser:            {tableFieldId, tableFieldDisplayName, tableFieldCreatedAt, tableFieldOrganizationMemberships},
}

type TablePrinter struct{}

func (p *TablePrinter) Write(w io.Writer, item interface{}) error {
	if item == nil {
		return nil
	}

	val := reflect.ValueOf(item)

	// Add support for a single struct
	if val.Kind() == reflect.Struct {
		slice := reflect.MakeSlice(reflect.SliceOf(val.Type()), 1, 1)
		slice.Index(0).Set(val)
		val = slice
	}

	if val.Kind() != reflect.Slice {
		return errors.New("provided object is not a slice or a struct")
	}

	if val.Len() == 0 {
		return nil // No items to display
	}

	firstItem := val.Index(0)
	if firstItem.Kind() == reflect.Pointer {
		firstItem = firstItem.Elem()
	}

	columns, ok := tableColumns[firstItem.Type().Name()]
	if !ok {
		return fmt.Errorf("no table columns defined for type %s", firstItem.Type().Name())
	}

	table := uitable.New()

	rawColumns := make([]interface{}, len(columns))
	for i, v := range columns {
		rawColumns[i] = v
	}
	table.AddRow(rawColumns...)

	for i := 0; i < val.Len(); i++ {
		row := make([]interface{}, len(columns))
		for j, column := range columns {
			value := val.Index(i).FieldByName(column)

			if !value.IsValid() || (value.Kind() == reflect.Pointer && value.IsNil()) || (value.IsZero() && value.Kind() != reflect.Bool) {
				row[j] = "-"
			} else if value.Kind() == reflect.Pointer {
				row[j] = value.Elem().Interface()
			} else {
				row[j] = value.Interface()
			}
		}
		table.AddRow(row...)
	}

	_, err := fmt.Fprintln(w, table)
	if err != nil {
		return err
	}
	return nil
}

func SetupListOutputFormatFlag(fs *pflag.FlagSet) {
	fs.StringP(OutputFormatFlag, "o", TablePrinterType, "Output format (json|yaml|table)")
}

func SetupSingleOutputFormatFlag(fs *pflag.FlagSet) {
	fs.StringP(OutputFormatFlag, "o", YamlPrinterType, "Output format (json|yaml)")
}
