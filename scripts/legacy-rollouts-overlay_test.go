package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func overlayFixture(t *testing.T) []byte {
	t.Helper()
	directory := t.TempDir()
	source, err := os.OpenRoot("../clients/platform-orchestrator-cp")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	fixture, err := os.OpenRoot(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fixture.Close()) })
	for _, filename := range []string{"spec.yaml", "legacy-enterprise-spec.yaml"} {
		raw, err := source.ReadFile(filename)
		require.NoError(t, err)
		require.NoError(t, fixture.WriteFile(filename, raw, 0o600))
	}
	expected, err := source.ReadFile("legacy-rollouts-overlay.yaml")
	require.NoError(t, err)
	t.Chdir(directory)
	return expected
}

func TestLegacyRolloutsOverlayRealGenerationIsByteReproducible(t *testing.T) {
	expected := overlayFixture(t)
	require.NoError(t, run())
	actual, err := os.ReadFile("legacy-rollouts-overlay.yaml")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	require.NoError(t, run())
	actual, err = os.ReadFile("legacy-rollouts-overlay.yaml")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestLegacyRolloutsOverlayInputDriftCannotOverwriteOutput(t *testing.T) {
	for _, filename := range []string{"spec.yaml", "legacy-enterprise-spec.yaml"} {
		t.Run(filename, func(t *testing.T) {
			expected := overlayFixture(t)
			require.NoError(t, run())
			fixture, err := os.OpenRoot(".")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, fixture.Close()) })
			raw, err := fixture.ReadFile(filename)
			require.NoError(t, err)
			require.NoError(t, fixture.WriteFile(filename, append(raw, '\n'), 0o600))
			require.ErrorContains(t, run(), "immutable source pin")
			actual, err := os.ReadFile("legacy-rollouts-overlay.yaml")
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}
