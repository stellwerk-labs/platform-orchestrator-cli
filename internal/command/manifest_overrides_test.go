package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
)

const manifestOverridesTestServiceField = "service"

func TestApplyManifestOverrides(t *testing.T) {
	baseManifest := func() dp.DeploymentManifest {
		return dp.DeploymentManifest{
			Workloads: map[string]dp.DeploymentManifestWorkload{
				"my-workload": {
					Resources: map[string]dp.DeploymentManifestResource{
						manifestOverridesTestServiceField: {
							Type:   "k8s-service",
							Params: map[string]interface{}{"port": "8080"},
						},
					},
				},
			},
			Shared: map[string]dp.DeploymentManifestResource{
				"my-db": {
					Type: testArtResourceTypeId,
				},
			},
		}
	}

	t.Run("set workload output", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"workloads.my-workload.outputs.host=localhost"}))
		assert.Equal(t, "localhost", m.Workloads["my-workload"].Outputs["host"])
	})

	t.Run("set workload resource param", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"workloads.my-workload.resources.service.params.image=my-registry/app:v2"}))
		assert.Equal(t, "my-registry/app:v2", m.Workloads["my-workload"].Resources[manifestOverridesTestServiceField].Params["image"])
		// Existing params must not be wiped
		assert.Equal(t, "8080", m.Workloads["my-workload"].Resources[manifestOverridesTestServiceField].Params["port"])
	})

	t.Run("set workload resource type", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"workloads.my-workload.resources.service.type=k8s-deployment"}))
		assert.Equal(t, "k8s-deployment", m.Workloads["my-workload"].Resources[manifestOverridesTestServiceField].Type)
	})

	t.Run("set workload resource class", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"workloads.my-workload.resources.service.class=large"}))
		assert.Equal(t, "large", *m.Workloads["my-workload"].Resources[manifestOverridesTestServiceField].Class)
	})

	t.Run("set workload resource id", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"workloads.my-workload.resources.service.id=svc-42"}))
		assert.Equal(t, "svc-42", *m.Workloads["my-workload"].Resources[manifestOverridesTestServiceField].Id)
	})

	t.Run("set shared resource param", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"shared.my-db.params.host=db.example.com"}))
		assert.Equal(t, "db.example.com", m.Shared["my-db"].Params["host"])
	})

	t.Run("set shared resource type", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"shared.my-db.type=mysql"}))
		assert.Equal(t, "mysql", m.Shared["my-db"].Type)
	})

	t.Run("set shared resource class", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"shared.my-db.class=large"}))
		assert.Equal(t, "large", *m.Shared["my-db"].Class)
	})

	t.Run("set shared resource id", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{"shared.my-db.id=db-99"}))
		assert.Equal(t, "db-99", *m.Shared["my-db"].Id)
	})

	t.Run("auto-creates missing workload and resource maps", func(t *testing.T) {
		m := dp.DeploymentManifest{}
		require.NoError(t, applyManifestOverrides(&m, []string{"workloads.new-workload.resources.svc.params.image=nginx:latest"}))
		assert.Equal(t, "nginx:latest", m.Workloads["new-workload"].Resources["svc"].Params["image"])
	})

	t.Run("auto-creates missing shared map", func(t *testing.T) {
		m := dp.DeploymentManifest{}
		require.NoError(t, applyManifestOverrides(&m, []string{"shared.new-db.type=postgres"}))
		assert.Equal(t, testArtResourceTypeId, m.Shared["new-db"].Type)
	})

	t.Run("multiple overrides applied in order", func(t *testing.T) {
		m := baseManifest()
		require.NoError(t, applyManifestOverrides(&m, []string{
			"workloads.my-workload.resources.service.params.image=v1",
			"workloads.my-workload.resources.service.params.image=v2",
		}))
		// last write wins
		assert.Equal(t, "v2", m.Workloads["my-workload"].Resources[manifestOverridesTestServiceField].Params["image"])
	})

	t.Run("error: missing = separator", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"workloads.my-workload.outputs.host"})
		assert.EqualError(t, err, `invalid --set value "workloads.my-workload.outputs.host": must be in the format <path>=<value>`)
	})

	t.Run("error: empty path", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"=value"})
		assert.EqualError(t, err, `invalid --set value "=value": path must not be empty`)
	})

	t.Run("error: unknown top-level segment", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"modules.foo.bar=val"})
		assert.ErrorContains(t, err, `unknown top-level segment "modules"`)
	})

	t.Run("error: unknown workload field", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"workloads.my-workload.variables.foo=bar"})
		assert.ErrorContains(t, err, `unknown workload field "variables"`)
	})

	t.Run("error: unknown resource field", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"workloads.my-workload.resources.service.annotations.foo=bar"})
		assert.ErrorContains(t, err, `unknown resource field "annotations"`)
	})

	t.Run("error: type with sub-key", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"workloads.my-workload.resources.service.type.extra=bad"})
		assert.ErrorContains(t, err, `'type' takes no sub-keys`)
	})

	t.Run("error: params without key", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"workloads.my-workload.resources.service.params=val"})
		assert.ErrorContains(t, err, `expected 'params.<key>'`)
	})

	t.Run("error: workload path too short", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"workloads.my-workload=val"})
		assert.ErrorContains(t, err, `expected 'workloads.<name>.<field>...'`)
	})

	t.Run("error: shared path too short", func(t *testing.T) {
		m := baseManifest()
		err := applyManifestOverrides(&m, []string{"shared.my-db=val"})
		assert.ErrorContains(t, err, `expected 'shared.<alias>.<field>`)
	})
}
