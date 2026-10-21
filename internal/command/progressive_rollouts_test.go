package command

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

func TestProgressiveRolloutControlSurface(t *testing.T) {
	for _, name := range []string{
		"return-to-draft", "start", "start-wave", "bypass-bake", "override-pins", "continue-unpinned", "pause", "resume", "retry-environment",
		"fail", "cancel", "cancel-and-rollback", "rollback", "rollback-environment", "archive",
	} {
		command, _, err := ProgressiveRolloutCmd.Find([]string{name})
		require.NoError(t, err)
		assert.Equal(t, name, command.Name())
		assert.NotNil(t, command.Flag(rolloutExpectedVersionFlag))
		assert.NotNil(t, command.Flag(rolloutIdempotencyFlag))
	}
}

func TestProgressiveRolloutControlCommandsRequireSemanticArguments(t *testing.T) {
	for _, name := range []string{"fail", "bypass-bake", "override-pins", "continue-unpinned"} {
		command := newProgressiveRolloutControlCommand(name)
		require.NoError(t, command.Flags().Set(rolloutExpectedVersionFlag, "1"))
		err := command.ValidateRequiredFlags()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `required flag(s) "reason" not set`)
	}

	for name, flag := range map[string]string{
		"start-wave":           "wave",
		"retry-environment":    "environment",
		"rollback-environment": "environment",
	} {
		command := newProgressiveRolloutControlCommand(name)
		require.NoError(t, command.Flags().Set(rolloutExpectedVersionFlag, "1"))
		err := command.ValidateRequiredFlags()
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf(`required flag(s) "%s" not set`, flag))
	}
}

func TestProgressiveRolloutPluginAvailabilityDiagnostics(t *testing.T) {
	for _, state := range []cp.PluginAvailabilityState{cp.PluginAvailabilityStateNotInstalled, cp.PluginAvailabilityStateNotEnabled, cp.PluginAvailabilityStateNotEntitled, cp.PluginAvailabilityStateIncompatible, cp.PluginAvailabilityStateUnavailable} {
		err := progressiveRolloutsPluginError(cp.PluginContribution{
			Id: progressiveRolloutsPluginID, DisplayName: "Progressive Rollouts", State: state,
		}, progressiveRolloutsMutate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin_"+string(state))
		assert.Contains(t, err.Error(), progressiveRolloutsPluginID)
	}
	assert.NoError(t, progressiveRolloutsPluginError(cp.PluginContribution{State: cp.PluginAvailabilityStateAvailable}, progressiveRolloutsMutate))
}

func TestProgressiveRolloutCapabilityDiscoverySupportsLegacyServers(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusNotImplemented} {
		err := progressiveRolloutsCapabilityDiscoveryError(status, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin_not_installed")
		assert.Contains(t, err.Error(), progressiveRolloutsPluginID)
		assert.Contains(t, err.Error(), "licence", "not_installed message must mention licence requirement")
	}
	err := progressiveRolloutsCapabilityDiscoveryError(http.StatusServiceUnavailable, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin_unavailable")
	assert.Contains(t, err.Error(), progressiveRolloutsPluginID, "unavailable message must include plugin ID")
}

func TestProgressiveRolloutPluginErrorFallbackMentionsLicence(t *testing.T) {
	for _, state := range []cp.PluginAvailabilityState{
		cp.PluginAvailabilityStateNotEnabled,
		cp.PluginAvailabilityStateNotEntitled,
		cp.PluginAvailabilityStateIncompatible,
	} {
		err := progressiveRolloutsPluginError(cp.PluginContribution{
			Id: progressiveRolloutsPluginID, DisplayName: "Progressive Rollouts", State: state,
		}, progressiveRolloutsMutate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "licence", "fallback admin action must mention licence")
	}
}

func TestProgressiveRolloutHelpMarksEnterpriseRequirement(t *testing.T) {
	for _, command := range []*cobra.Command{ProgressiveRolloutCmd, CreateProgressiveRollout, GetProgressiveRollout, ListProgressiveRollouts, UpdateProgressiveRollout} {
		assert.Contains(t, command.Short, "Enterprise")
		assert.Contains(t, command.Short, progressiveRolloutsPluginID)
	}
}

func TestProgressiveRolloutReadAndSafetyRemainAvailableAfterEntitlementExpiry(t *testing.T) {
	for _, state := range []cp.PluginAvailabilityState{cp.PluginAvailabilityStateNotEnabled, cp.PluginAvailabilityStateNotEntitled} {
		contribution := cp.PluginContribution{State: state}
		assert.NoError(t, progressiveRolloutsPluginError(contribution, progressiveRolloutsRead))
		assert.NoError(t, progressiveRolloutsPluginError(contribution, progressiveRolloutsSafety))
		require.Error(t, progressiveRolloutsPluginError(contribution, progressiveRolloutsMutate))
	}
	for _, state := range []cp.PluginAvailabilityState{cp.PluginAvailabilityStateNotInstalled, cp.PluginAvailabilityStateIncompatible, cp.PluginAvailabilityStateUnavailable} {
		contribution := cp.PluginContribution{State: state}
		require.Error(t, progressiveRolloutsPluginError(contribution, progressiveRolloutsRead))
		require.Error(t, progressiveRolloutsPluginError(contribution, progressiveRolloutsSafety))
	}
}

func TestProgressiveRolloutCommandsPreserveOnlyExplicitSafetyActions(t *testing.T) {
	safety := map[string]bool{
		"pause": true, rolloutCancelCommand: true, rolloutCancelRollback: true,
		rolloutRollbackCommand: true, rolloutRollbackEnvironment: true,
	}
	for _, command := range ProgressiveRolloutCmd.Commands() {
		if command.Flag(rolloutExpectedVersionFlag) == nil {
			continue
		}
		expected := progressiveRolloutsMutate
		if safety[command.Name()] {
			expected = progressiveRolloutsSafety
		}
		assert.Equal(t, expected, progressiveRolloutCommandAccess(command.Name()), command.Name())
	}
}

func TestProgressiveRolloutEvidenceSurface(t *testing.T) {
	for _, name := range []string{"logs", "outputs"} {
		command, _, err := ProgressiveRolloutCmd.Find([]string{name})
		require.NoError(t, err)
		assert.Equal(t, name, command.Name())
	}
}

func TestProgressiveRolloutApprovalRequiresAnAuditedReason(t *testing.T) {
	assert.NotNil(t, ApproveProgressiveRollout.Flag("reason"))
	assert.NotNil(t, ApproveProgressiveRollout.Flag("scope-type"))
	assert.NotNil(t, ApproveProgressiveRollout.Flag("scope-id"))
}

func TestProgressiveRolloutCommandDoesNotOwnCoreModuleLifecycleAliases(t *testing.T) {
	for _, name := range []string{"set-version-status", "proposed-versions"} {
		assert.NotContains(t, commandNames(ProgressiveRolloutCmd.Commands()), name)
	}
}

func commandNames(commands []*cobra.Command) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		result = append(result, command.Name())
	}
	return result
}

func TestRolloutIdempotencyKey(t *testing.T) {
	command := newProgressiveRolloutControlCommand("pause")
	generated := rolloutIdempotencyKey(command)
	assert.NotEqual(t, uuid.Nil, uuid.MustParse(generated))

	require.NoError(t, command.Flags().Set(rolloutIdempotencyFlag, "stable-command-key"))
	assert.Equal(t, "stable-command-key", rolloutIdempotencyKey(command))
}

func TestRolloutIDRejectsInvalidValue(t *testing.T) {
	_, err := rolloutID("absolutely-not-a-uuid")
	require.Error(t, err)
}
