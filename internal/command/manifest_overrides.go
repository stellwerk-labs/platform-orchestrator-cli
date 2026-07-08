package command

import (
	"strings"

	"github.com/pkg/errors"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

// applyManifestOverrides applies a list of key=value overrides to a manifest.
// Each override must follow the dot-notation path format:
//
//	workloads.<workload>.outputs.<key>=<value>
//	workloads.<workload>.resources.<alias>.params.<key>=<value>
//	workloads.<workload>.resources.<alias>.type=<value>
//	workloads.<workload>.resources.<alias>.class=<value>
//	workloads.<workload>.resources.<alias>.id=<value>
//	shared.<alias>.params.<key>=<value>
//	shared.<alias>.type=<value>
//	shared.<alias>.class=<value>
//	shared.<alias>.id=<value>
func applyManifestOverrides(manifest *dp.DeploymentManifest, overrides []string) error {
	for _, o := range overrides {
		if err := applyManifestOverride(manifest, o); err != nil {
			return err
		}
	}
	return nil
}

func applyManifestOverride(manifest *dp.DeploymentManifest, override string) error {
	key, value, found := strings.Cut(override, "=")
	if !found {
		return errors.Errorf("invalid --set value %q: must be in the format <path>=<value>", override)
	}
	if key == "" {
		return errors.Errorf("invalid --set value %q: path must not be empty", override)
	}

	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return errors.Errorf("invalid --set path %q: must start with 'workloads.<name>.' or 'shared.<alias>.'", key)
	}

	switch parts[0] {
	case "workloads":
		return applyWorkloadOverride(manifest, parts[1:], key, value)
	case "shared":
		return applySharedOverride(manifest, parts[1:], key, value)
	default:
		return errors.Errorf("invalid --set path %q: unknown top-level segment %q, expected 'workloads' or 'shared'", key, parts[0])
	}
}

func applyWorkloadOverride(manifest *dp.DeploymentManifest, parts []string, fullKey, value string) error {
	if len(parts) < 2 {
		return errors.Errorf("invalid --set path %q: expected 'workloads.<name>.<field>...'", fullKey)
	}

	workloadName := parts[0]
	section := parts[1]

	if manifest.Workloads == nil {
		manifest.Workloads = make(map[string]dp.DeploymentManifestWorkload)
	}
	workload := manifest.Workloads[workloadName]

	switch section {
	case "outputs":
		if len(parts) != 3 {
			return errors.Errorf("invalid --set path %q: expected 'workloads.<name>.outputs.<key>'", fullKey)
		}
		outputKey := parts[2]
		if workload.Outputs == nil {
			workload.Outputs = make(map[string]string)
		}
		workload.Outputs[outputKey] = value

	case "resources":
		if len(parts) < 4 {
			return errors.Errorf("invalid --set path %q: expected 'workloads.<name>.resources.<alias>.<field>[.<key>]'", fullKey)
		}
		alias := parts[2]
		field := parts[3]

		if workload.Resources == nil {
			workload.Resources = make(map[string]dp.DeploymentManifestResource)
		}
		resource := workload.Resources[alias]

		if err := applyResourceField(&resource, parts[3:], field, fullKey, value); err != nil {
			return err
		}
		workload.Resources[alias] = resource

	default:
		return errors.Errorf("invalid --set path %q: unknown workload field %q, expected 'outputs' or 'resources'", fullKey, section)
	}

	manifest.Workloads[workloadName] = workload
	return nil
}

func applySharedOverride(manifest *dp.DeploymentManifest, parts []string, fullKey, value string) error {
	if len(parts) < 2 {
		return errors.Errorf("invalid --set path %q: expected 'shared.<alias>.<field>[.<key>]'", fullKey)
	}

	alias := parts[0]
	field := parts[1]

	if manifest.Shared == nil {
		manifest.Shared = make(map[string]dp.DeploymentManifestResource)
	}
	resource := manifest.Shared[alias]

	if err := applyResourceField(&resource, parts[1:], field, fullKey, value); err != nil {
		return err
	}
	manifest.Shared[alias] = resource
	return nil
}

func applyResourceField(resource *dp.DeploymentManifestResource, fieldParts []string, field, fullKey, value string) error {
	switch field {
	case "type":
		if len(fieldParts) != 1 {
			return errors.Errorf("invalid --set path %q: 'type' takes no sub-keys", fullKey)
		}
		resource.Type = value

	case "class":
		if len(fieldParts) != 1 {
			return errors.Errorf("invalid --set path %q: 'class' takes no sub-keys", fullKey)
		}
		resource.Class = ref.Ref(value)

	case "id":
		if len(fieldParts) != 1 {
			return errors.Errorf("invalid --set path %q: 'id' takes no sub-keys", fullKey)
		}
		resource.Id = ref.Ref(value)

	case "params":
		if len(fieldParts) != 2 {
			return errors.Errorf("invalid --set path %q: expected 'params.<key>'", fullKey)
		}
		paramKey := fieldParts[1]
		if resource.Params == nil {
			resource.Params = make(map[string]interface{})
		}
		resource.Params[paramKey] = value

	default:
		return errors.Errorf("invalid --set path %q: unknown resource field %q, expected 'type', 'class', 'id', or 'params'", fullKey, field)
	}
	return nil
}
