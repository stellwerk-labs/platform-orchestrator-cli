package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoreCandidateSpecRequiresExplicitMatchingSource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mismatch bool
	}{
		{name: "missing source"}, {name: "different source", mismatch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := t.TempDir()
			for _, path := range []string{
				"scripts/check-api-specs.sh",
				"clients/platform-orchestrator-cp/SOURCE.md", "clients/platform-orchestrator-cp/spec.yaml",
				"clients/platform-orchestrator-dp/SOURCE.md", "clients/platform-orchestrator-dp/spec.yaml",
				"clients/pipelines/SOURCE.md", "clients/pipelines/spec.yaml",
				"clients/progressive-rollouts/SOURCE.md", "clients/progressive-rollouts/spec.yaml",
			} {
				data, err := os.ReadFile(filepath.Join("..", path)) // #nosec G304 -- Path comes only from the literal repository fixture list above.
				require.NoError(t, err)
				if path == "clients/platform-orchestrator-cp/SOURCE.md" {
					data = regexp.MustCompile("(?m)^- Qualification: `[^`]+`$").ReplaceAll(data, []byte("- Qualification: `local-candidate`"))
					require.Contains(t, string(data), "- Qualification: `local-candidate`")
				}
				target := filepath.Join(fixture, path)
				require.NoError(t, os.MkdirAll(filepath.Dir(target), 0700))
				require.NoError(t, os.WriteFile(target, data, 0600)) // #nosec G703 -- Target joins t.TempDir with a path from the literal fixture list.
			}
			t.Setenv("CP_SPEC_PATH", "")
			t.Setenv("DP_SPEC_PATH", filepath.Join(fixture, "clients", "platform-orchestrator-dp", "spec.yaml"))
			t.Setenv("PIPELINES_SPEC_PATH", filepath.Join(fixture, "clients", "pipelines", "spec.yaml"))
			if tc.mismatch {
				path := filepath.Join(t.TempDir(), "different.yaml")
				require.NoError(t, os.WriteFile(path, []byte("openapi: wrong\n"), 0600))
				t.Setenv("CP_SPEC_PATH", path)
			}
			command := exec.CommandContext(t.Context(), "bash", "scripts/check-api-specs.sh")
			command.Dir = fixture
			output, err := command.CombinedOutput()
			require.Error(t, err)
			if tc.mismatch {
				require.Contains(t, string(output), "clients/platform-orchestrator-cp/spec.yaml differs")
			} else {
				require.Contains(t, string(output), "Unpublished Core candidate requires explicit CP_SPEC_PATH")
			}
		})
	}
}
