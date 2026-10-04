package command

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	pluginapi "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-plugin"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	rolloutExpectedVersionFlag  = "expected-version"
	rolloutRevisionFlag         = "revision"
	rolloutIdempotencyFlag      = "idempotency-key"
	progressiveRolloutsPluginID = "dev.stellwerk.progressive-rollouts"
	progressiveRolloutsHelp     = " [Enterprise plugin: dev.stellwerk.progressive-rollouts; installation, enablement and entitlement required]"
	rolloutCancelCommand        = "cancel"
	rolloutCancelRollback       = "cancel-and-rollback"
	rolloutRollbackCommand      = "rollback"
	rolloutRollbackEnvironment  = "rollback-environment"
	rolloutFailCommand          = "fail"
)

var rolloutAliases = []string{"ro"}

func rolloutID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, errors.Wrapf(err, "invalid rollout id %q", value)
	}
	return id, nil
}

func rolloutIdempotencyKey(cmd *cobra.Command) string {
	value, _ := cmd.Flags().GetString(rolloutIdempotencyFlag)
	if value == "" {
		return uuid.NewString()
	}
	return value
}

func rolloutResponseError(action string, status int, body []byte) error {
	return errors.Errorf("%s failed with HTTP %d: %s", action, status, string(body))
}

func writeRolloutValue(cmd *cobra.Command, value any) error {
	return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), value)
}

type progressiveRolloutsAccess int

const (
	progressiveRolloutsMutate progressiveRolloutsAccess = iota
	progressiveRolloutsRead
	progressiveRolloutsSafety
)

func requireProgressiveRolloutsPlugin(access progressiveRolloutsAccess, operationID string, run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		native, err := useNativeProgressiveRollouts(cmd)
		if err != nil {
			return err
		}
		if native {
			if operationID == "" {
				return run(cmd, args)
			}
			if err := requireNativeProgressiveRolloutsOperation(cmd.Context(), orgID, operationID, access); err != nil {
				return err
			}
			return run(cmd, args)
		}
		response, err := MustCpClient(cmd.Context()).GetPluginCapabilityWithResponse(cmd.Context(), orgID, progressiveRolloutsPluginID)
		if err != nil {
			return errors.Wrap(err, "plugin_unavailable: Progressive Rollouts capability discovery failed")
		}
		if response.StatusCode() != http.StatusOK || response.JSON200 == nil {
			return progressiveRolloutsCapabilityDiscoveryError(response.StatusCode(), response.Body)
		}
		if err := progressiveRolloutsPluginError(*response.JSON200, access); err != nil {
			return err
		}
		return run(cmd, args)
	}
}

func progressiveRolloutsCapabilityDiscoveryError(status int, body []byte) error {
	switch status {
	case http.StatusNotFound, http.StatusNotImplemented:
		return errors.Errorf("plugin_not_installed: Progressive Rollouts (%s) is not supported by this server; ask an administrator to install and enable the Enterprise plugin", progressiveRolloutsPluginID)
	case http.StatusServiceUnavailable:
		return errors.Errorf("plugin_unavailable: Progressive Rollouts (%s) capability discovery is temporarily unavailable", progressiveRolloutsPluginID)
	default:
		return rolloutResponseError("discover Progressive Rollouts plugin", status, body)
	}
}

func progressiveRolloutsPluginError(capability cp.PluginCapability, access progressiveRolloutsAccess) error {
	if capability.State == cp.PluginAvailabilityStateAvailable {
		return nil
	}
	if (access == progressiveRolloutsRead || access == progressiveRolloutsSafety) &&
		(capability.State == cp.PluginAvailabilityStateNotEnabled || capability.State == cp.PluginAvailabilityStateNotEntitled) {
		return nil
	}
	code := "plugin_" + string(capability.State)
	action := ref.DerefOr(capability.AdminAction, "Ask an organization administrator to check the plugin installation and entitlement.")
	return errors.Errorf("%s: %s (%s) is %s. %s", code, capability.DisplayName, capability.Id, capability.State, action)
}

var CreateProgressiveRollout = &cobra.Command{
	Use:     "rollout",
	Aliases: rolloutAliases,
	Args:    cobra.NoArgs,
	Short:   "Create a Progressive Rollout Draft",
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readSetFlagsIntoType[cp.ProgressiveRolloutSpec](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRollout](cmd.Context(), orgID, "createProgressiveRollout", nil, nil, *body, rolloutIdempotencyKey(cmd), http.StatusCreated)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).CreateProgressiveRolloutWithResponse(cmd.Context(), orgID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to create Progressive Rollout")
		}
		if response.StatusCode() != http.StatusCreated {
			return rolloutResponseError("create rollout", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON201)
	},
}

var GetProgressiveRollout = &cobra.Command{
	Use:     "rollout <rollout-id>",
	Aliases: rolloutAliases,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a Progressive Rollout with its plan, execution and audit history",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutDetail](cmd.Context(), orgID, "getProgressiveRollout", map[string]string{rolloutIDPathParameter: id.String()}, nil, nil, "", http.StatusOK)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).GetProgressiveRolloutWithResponse(cmd.Context(), orgID, id)
		if err != nil {
			return errors.Wrap(err, "failed to get Progressive Rollout")
		}
		if response.StatusCode() != http.StatusOK {
			return rolloutResponseError("get rollout", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON200)
	},
}

var ListProgressiveRollouts = &cobra.Command{
	Use:     "rollouts",
	Aliases: []string{"ros"},
	Args:    cobra.NoArgs,
	Short:   "List Progressive Rollouts",
	RunE: func(cmd *cobra.Command, _ []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		states, _ := cmd.Flags().GetStringSlice("state")
		stateValues := make([]cp.ProgressiveRolloutState, len(states))
		for index := range states {
			stateValues[index] = cp.ProgressiveRolloutState(states[index])
		}
		projectID, _ := cmd.Flags().GetString("project")
		moduleID, _ := cmd.Flags().GetString("module")
		view, _ := cmd.Flags().GetString("view")
		historyVisibility, _ := cmd.Flags().GetString("history-visibility")
		var conflicts *bool
		if cmd.Flags().Changed("conflicts") {
			value, _ := cmd.Flags().GetBool("conflicts")
			conflicts = &value
		}
		native, err := useNativeProgressiveRollouts(cmd)
		if err != nil {
			return err
		}
		items, err := clients.CollectAll(
			func(page string) (*cp.ListProgressiveRolloutsResponse, error) {
				if native {
					query := nativeProgressiveRolloutListQuery(page, states, projectID, moduleID, view, historyVisibility, conflicts)
					result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutPage](cmd.Context(), orgID, "listProgressiveRollouts", nil, query, nil, "", http.StatusOK)
					if err != nil {
						return nil, err
					}
					return nativeProgressiveRolloutPageResponse(result), nil
				}
				return MustCpClient(cmd.Context()).ListProgressiveRolloutsWithResponse(cmd.Context(), orgID, &cp.ListProgressiveRolloutsParams{
					Page: ref.RefStringEmptyNil(page), State: func() *[]cp.ProgressiveRolloutState {
						if len(stateValues) == 0 {
							return nil
						}
						return &stateValues
					}(),
					Project: ref.RefStringEmptyNil(projectID), Module: ref.RefStringEmptyNil(moduleID), Conflicts: conflicts,
					View: func() *cp.ListProgressiveRolloutsParamsView {
						if view == "" {
							return nil
						}
						value := cp.ListProgressiveRolloutsParamsView(view)
						return &value
					}(),
					HistoryVisibility: func() *cp.ListProgressiveRolloutsParamsHistoryVisibility {
						if historyVisibility == "" {
							return nil
						}
						value := cp.ListProgressiveRolloutsParamsHistoryVisibility(historyVisibility)
						return &value
					}(),
				})
			},
			func(response *cp.ListProgressiveRolloutsResponse) ([]cp.ProgressiveRollout, *string) {
				if response.JSON200 == nil {
					return nil, nil
				}
				return response.JSON200.Items, response.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		return writeRolloutValue(cmd, items)
	},
}

var UpdateProgressiveRollout = &cobra.Command{
	Use:     "rollout <rollout-id>",
	Aliases: rolloutAliases,
	Args:    cobra.ExactArgs(1),
	Short:   "Update a Progressive Rollout Draft",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		body, err := readSetFlagsIntoType[cp.ProgressiveRolloutDraftUpdate](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRollout](cmd.Context(), orgID, "updateProgressiveRolloutDraft", map[string]string{rolloutIDPathParameter: id.String()}, nil, *body, rolloutIdempotencyKey(cmd), http.StatusOK)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).UpdateProgressiveRolloutDraftWithResponse(cmd.Context(), orgID, id, *body)
		if err != nil {
			return errors.Wrap(err, "failed to update Progressive Rollout")
		}
		if response.StatusCode() != http.StatusOK {
			return rolloutResponseError("update rollout", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON200)
	},
}

var ProgressiveRolloutCmd = &cobra.Command{
	Use:     "rollout",
	Aliases: rolloutAliases,
	Short:   "Design and control Progressive Rollouts" + progressiveRolloutsHelp,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		output, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), output, []string{printer.JsonPrinterType, printer.YamlPrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)
		return nil
	},
}

var PreviewProgressiveRollout = &cobra.Command{
	Use:   "preview",
	Args:  cobra.NoArgs,
	Short: "Resolve selectors and preview an exact candidate plan",
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readSetFlagsIntoType[cp.ProgressiveRolloutSpec](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutPlan](cmd.Context(), orgID, "previewProgressiveRollout", nil, nil, *body, "", http.StatusOK)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).PreviewProgressiveRolloutWithResponse(cmd.Context(), orgID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to preview Progressive Rollout")
		}
		if response.StatusCode() != http.StatusOK {
			return rolloutResponseError("preview rollout", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON200)
	},
}

var PrepareProgressiveRollout = &cobra.Command{
	Use:   "prepare <rollout-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Freeze the reviewed candidate as an immutable plan revision",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		expected, _ := cmd.Flags().GetInt64(rolloutExpectedVersionFlag)
		fingerprint, _ := cmd.Flags().GetString("candidate-fingerprint")
		accepted, _ := cmd.Flags().GetStringSlice("accept-conflict")
		prepareBody := cp.ProgressiveRolloutPrepareBody{ExpectedResourceVersion: expected, CandidateFingerprint: fingerprint, AcceptedConflictFingerprints: accepted}
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutDetail](cmd.Context(), orgID, "prepareProgressiveRollout", map[string]string{rolloutIDPathParameter: id.String()}, nil, prepareBody, rolloutIdempotencyKey(cmd), http.StatusOK)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).PrepareProgressiveRolloutWithResponse(cmd.Context(), orgID, id,
			&cp.PrepareProgressiveRolloutParams{IdempotencyKey: rolloutIdempotencyKey(cmd)},
			prepareBody)
		if err != nil {
			return errors.Wrap(err, "failed to prepare Progressive Rollout")
		}
		if response.StatusCode() != http.StatusOK {
			return rolloutResponseError("prepare rollout", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON200)
	},
}

func newProgressiveRolloutControlCommand(command string) *cobra.Command {
	result := &cobra.Command{
		Use:   command + " <rollout-id>",
		Args:  cobra.ExactArgs(1),
		Short: fmt.Sprintf("Run the %s command on a Progressive Rollout", command),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := rolloutID(args[0])
			if err != nil {
				return err
			}
			if destructive := command == rolloutFailCommand || command == rolloutCancelCommand || command == rolloutCancelRollback || command == rolloutRollbackCommand || command == rolloutRollbackEnvironment || command == "archive"; destructive {
				noPrompt, _ := cmd.Flags().GetBool(deployCmdNoPromptFlag)
				if !noPrompt {
					confirmed, err := PromptYesNo(cmd.Context(), cmd.InOrStdin(), fmt.Sprintf("Execute destructive rollout command %s?", command))
					if err != nil {
						return err
					}
					if !confirmed {
						return errors.New("rollout command cancelled")
					}
				}
			}
			orgID, err := ShouldOrg(cmd.Context())
			if err != nil {
				return err
			}
			expected, _ := cmd.Flags().GetInt64(rolloutExpectedVersionFlag)
			confirmDefective, _ := cmd.Flags().GetBool("confirm-defective-versions")
			waveID, _ := cmd.Flags().GetString("wave")
			reason, _ := cmd.Flags().GetString("reason")
			environmentValue, _ := cmd.Flags().GetString("environment")
			environmentValues, _ := cmd.Flags().GetStringSlice("environments")
			var environmentID *uuid.UUID
			if environmentValue != "" {
				parsed, err := uuid.Parse(environmentValue)
				if err != nil {
					return errors.Wrap(err, "invalid Environment UUID")
				}
				environmentID = &parsed
			}
			environmentIDs := make([]uuid.UUID, 0, len(environmentValues))
			for _, value := range environmentValues {
				parsed, err := uuid.Parse(value)
				if err != nil {
					return errors.Wrapf(err, "invalid Environment UUID %q", value)
				}
				environmentIDs = append(environmentIDs, parsed)
			}
			body := cp.ProgressiveRolloutCommandBody{ExpectedResourceVersion: expected, WaveId: ref.RefStringEmptyNil(waveID), EnvironmentId: environmentID,
				EnvironmentIds: func() *[]uuid.UUID {
					if len(environmentIDs) == 0 {
						return nil
					}
					return &environmentIDs
				}(), Reason: ref.RefStringEmptyNil(reason),
				ConfirmDefectiveVersions: func() *bool {
					if !confirmDefective {
						return nil
					}
					return &confirmDefective
				}()}
			key := rolloutIdempotencyKey(cmd)
			if native, err := useNativeProgressiveRollouts(cmd); err != nil {
				return err
			} else if native {
				result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutDetail](cmd.Context(), orgID, "commandProgressiveRollout", map[string]string{rolloutIDPathParameter: id.String(), "rolloutCommand": command}, nil, body, key, http.StatusOK)
				if err != nil {
					return err
				}
				return writeRolloutValue(cmd, *result)
			}
			response, err := MustCpClient(cmd.Context()).CommandProgressiveRolloutWithResponse(cmd.Context(), orgID, id,
				cp.CommandProgressiveRolloutParamsRolloutCommand(command), &cp.CommandProgressiveRolloutParams{IdempotencyKey: key}, body)
			if err != nil {
				return errors.Wrap(err, "failed to execute Progressive Rollout command")
			}
			if response.StatusCode() != http.StatusOK {
				return rolloutResponseError(command+" rollout", response.StatusCode(), response.Body)
			}
			return writeRolloutValue(cmd, *response.JSON200)
		},
	}
	result.Flags().Int64(rolloutExpectedVersionFlag, 0, "Expected Rollout resource version")
	_ = result.MarkFlagRequired(rolloutExpectedVersionFlag)
	result.Flags().String(rolloutIdempotencyFlag, "", "Stable idempotency key (a random UUID is used when omitted)")
	result.Flags().String("wave", "", "Wave ID for start-wave")
	result.Flags().String("environment", "", "Environment UUID for an Environment-scoped command")
	result.Flags().StringSlice("environments", nil, "Environment UUIDs for override-pins or continue-unpinned (all eligible when omitted)")
	result.Flags().String("reason", "", "Human-readable audit reason")
	result.Flags().Bool(deployCmdNoPromptFlag, false, "Do not prompt for destructive command confirmation")
	result.Flags().Bool("confirm-defective-versions", false, "Explicitly confirm restoring Defective starting versions during Rollback")
	if command == rolloutFailCommand || command == "bypass-bake" || command == "override-pins" || command == "continue-unpinned" {
		_ = result.MarkFlagRequired("reason")
	}
	if command == "start-wave" {
		_ = result.MarkFlagRequired("wave")
	}
	if command == "retry-environment" || command == rolloutRollbackEnvironment {
		_ = result.MarkFlagRequired("environment")
	}
	return result
}

var ApproveProgressiveRollout = &cobra.Command{
	Use:   "approve <rollout-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Approve one Environment or bulk-approve a Project's eligible Environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		scopeValue, _ := cmd.Flags().GetString("scope-id")
		scopeID, err := uuid.Parse(scopeValue)
		if err != nil {
			return errors.Wrap(err, "invalid scope UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		revision, _ := cmd.Flags().GetInt(rolloutRevisionFlag)
		scopeType, _ := cmd.Flags().GetString("scope-type")
		reason, _ := cmd.Flags().GetString("reason")
		body := cp.ProgressiveRolloutApprovalBody{PlanRevision: revision, ScopeId: scopeID, ScopeType: cp.ProgressiveRolloutApprovalBodyScopeType(scopeType), Reason: reason}
		key := rolloutIdempotencyKey(cmd)
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutApproval](cmd.Context(), orgID, "approveProgressiveRolloutScope", map[string]string{rolloutIDPathParameter: id.String()}, nil, body, key, http.StatusCreated)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).ApproveProgressiveRolloutScopeWithResponse(cmd.Context(), orgID, id,
			&cp.ApproveProgressiveRolloutScopeParams{IdempotencyKey: key}, body)
		if err != nil {
			return errors.Wrap(err, "failed to approve Progressive Rollout")
		}
		if response.StatusCode() != http.StatusCreated {
			return rolloutResponseError("approve rollout", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON201)
	},
}

var ResolveProgressiveRolloutConflict = &cobra.Command{
	Use:   "resolve-conflict <rollout-id> <conflict-id> <retry|approve|abort>",
	Args:  cobra.ExactArgs(3),
	Short: "Resolve an exact observed Rollout conflict",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		conflictID, err := uuid.Parse(args[1])
		if err != nil {
			return errors.Wrap(err, "invalid conflict id")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		revision, _ := cmd.Flags().GetInt(rolloutRevisionFlag)
		expected, _ := cmd.Flags().GetInt64(rolloutExpectedVersionFlag)
		fingerprint, _ := cmd.Flags().GetString("observed-fingerprint")
		body := cp.ProgressiveRolloutConflictResolutionBody{PlanRevision: revision, ExpectedResourceVersion: expected, ObservedFingerprint: fingerprint}
		key := rolloutIdempotencyKey(cmd)
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutDetail](cmd.Context(), orgID, "resolveProgressiveRolloutConflict", map[string]string{rolloutIDPathParameter: id.String(), "conflictId": conflictID.String(), "resolution": args[2]}, nil, body, key, http.StatusOK)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).ResolveProgressiveRolloutConflictWithResponse(cmd.Context(), orgID, id, conflictID,
			cp.ResolveProgressiveRolloutConflictParamsResolution(args[2]), &cp.ResolveProgressiveRolloutConflictParams{IdempotencyKey: key}, body)
		if err != nil {
			return errors.Wrap(err, "failed to resolve Progressive Rollout conflict")
		}
		if response.StatusCode() != http.StatusOK {
			return rolloutResponseError("resolve rollout conflict", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON200)
	},
}

var RevokeProgressiveRolloutApproval = &cobra.Command{
	Use:   "revoke-approval <rollout-id> <approval-id>",
	Args:  cobra.ExactArgs(2),
	Short: "Revoke an approval before an affected deployment starts",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		approvalID, err := uuid.Parse(args[1])
		if err != nil {
			return errors.Wrap(err, "invalid approval id")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		revision, _ := cmd.Flags().GetInt(rolloutRevisionFlag)
		key := rolloutIdempotencyKey(cmd)
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			_, err := invokeNativeProgressiveRollout[struct{}](cmd.Context(), orgID, "revokeProgressiveRolloutApproval", map[string]string{rolloutIDPathParameter: id.String(), "approvalId": approvalID.String()}, map[string][]string{planRevisionQueryParameter: {fmt.Sprint(revision)}}, nil, key, http.StatusNoContent)
			return err
		}
		response, err := MustCpClient(cmd.Context()).RevokeProgressiveRolloutApprovalWithResponse(cmd.Context(), orgID, id, approvalID,
			&cp.RevokeProgressiveRolloutApprovalParams{PlanRevision: revision, IdempotencyKey: key})
		if err != nil {
			return errors.Wrap(err, "failed to revoke Progressive Rollout approval")
		}
		if response.StatusCode() != http.StatusNoContent {
			return rolloutResponseError("revoke rollout approval", response.StatusCode(), response.Body)
		}
		return nil
	},
}

func newProgressiveRolloutRevisionListCommand(kind string) *cobra.Command {
	return &cobra.Command{
		Use:   kind + " <rollout-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List Rollout " + kind,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := rolloutID(args[0])
			if err != nil {
				return err
			}
			orgID, err := ShouldOrg(cmd.Context())
			if err != nil {
				return err
			}
			revision, _ := cmd.Flags().GetInt(rolloutRevisionFlag)
			if native, err := useNativeProgressiveRollouts(cmd); err != nil {
				return err
			} else if native {
				query := map[string][]string{planRevisionQueryParameter: {fmt.Sprint(revision)}}
				if kind == "approvals" {
					result, err := invokeNativeProgressiveRollout[[]cp.ProgressiveRolloutApproval](cmd.Context(), orgID, "listProgressiveRolloutApprovals", map[string]string{rolloutIDPathParameter: id.String()}, query, nil, "", http.StatusOK)
					if err != nil {
						return err
					}
					return writeRolloutValue(cmd, *result)
				}
				result, err := invokeNativeProgressiveRollout[[]cp.ProgressiveRolloutConflict](cmd.Context(), orgID, "listProgressiveRolloutConflicts", map[string]string{rolloutIDPathParameter: id.String()}, query, nil, "", http.StatusOK)
				if err != nil {
					return err
				}
				return writeRolloutValue(cmd, *result)
			}
			if kind == "approvals" {
				response, err := MustCpClient(cmd.Context()).ListProgressiveRolloutApprovalsWithResponse(cmd.Context(), orgID, id, &cp.ListProgressiveRolloutApprovalsParams{PlanRevision: revision})
				if err != nil {
					return errors.Wrap(err, "failed to list Rollout approvals")
				}
				if response.StatusCode() != http.StatusOK {
					return rolloutResponseError("list rollout approvals", response.StatusCode(), response.Body)
				}
				return writeRolloutValue(cmd, *response.JSON200)
			}
			response, err := MustCpClient(cmd.Context()).ListProgressiveRolloutConflictsWithResponse(cmd.Context(), orgID, id, &cp.ListProgressiveRolloutConflictsParams{PlanRevision: revision})
			if err != nil {
				return errors.Wrap(err, "failed to list Rollout conflicts")
			}
			if response.StatusCode() != http.StatusOK {
				return rolloutResponseError("list rollout conflicts", response.StatusCode(), response.Body)
			}
			return writeRolloutValue(cmd, *response.JSON200)
		},
	}
}

var ProgressiveRolloutPlan = &cobra.Command{
	Use:   "plan <rollout-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Get an immutable Rollout plan revision",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		revision, _ := cmd.Flags().GetInt(rolloutRevisionFlag)
		if native, err := useNativeProgressiveRollouts(cmd); err != nil {
			return err
		} else if native {
			result, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutPlan](cmd.Context(), orgID, "getProgressiveRolloutPlan", map[string]string{rolloutIDPathParameter: id.String(), "planRevision": fmt.Sprint(revision)}, nil, nil, "", http.StatusOK)
			if err != nil {
				return err
			}
			return writeRolloutValue(cmd, *result)
		}
		response, err := MustCpClient(cmd.Context()).GetProgressiveRolloutPlanWithResponse(cmd.Context(), orgID, id, revision)
		if err != nil {
			return errors.Wrap(err, "failed to get Rollout plan")
		}
		if response.StatusCode() != http.StatusOK {
			return rolloutResponseError("get rollout plan", response.StatusCode(), response.Body)
		}
		return writeRolloutValue(cmd, *response.JSON200)
	},
}

var ProgressiveRolloutEvents = &cobra.Command{
	Use: "events <rollout-id>", Args: cobra.ExactArgs(1), Short: "List append-only Rollout audit events",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := rolloutID(args[0])
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		after, _ := cmd.Flags().GetInt64("after")
		native, err := useNativeProgressiveRollouts(cmd)
		if err != nil {
			return err
		}
		events := make([]cp.ProgressiveRolloutEvent, 0, 500)
		for {
			if native {
				query := make(map[string][]string)
				if after != 0 {
					query["after"] = []string{fmt.Sprint(after)}
				}
				page, err := invokeNativeProgressiveRollout[[]cp.ProgressiveRolloutEvent](cmd.Context(), orgID, "listProgressiveRolloutEvents", map[string]string{rolloutIDPathParameter: id.String()}, query, nil, "", http.StatusOK)
				if err != nil {
					return err
				}
				events = append(events, (*page)...)
				if len(*page) < 500 {
					break
				}
				after = (*page)[len(*page)-1].Sequence
				continue
			}
			response, err := MustCpClient(cmd.Context()).ListProgressiveRolloutEventsWithResponse(cmd.Context(), orgID, id, &cp.ListProgressiveRolloutEventsParams{After: func() *int64 {
				if after == 0 {
					return nil
				}
				return &after
			}()})
			if err != nil {
				return errors.Wrap(err, "failed to list Rollout events")
			}
			if response.StatusCode() != http.StatusOK {
				return rolloutResponseError("list rollout events", response.StatusCode(), response.Body)
			}
			page := *response.JSON200
			events = append(events, page...)
			if len(page) < 500 {
				break
			}
			after = page[len(page)-1].Sequence
		}
		return writeRolloutValue(cmd, events)
	},
}

func newProgressiveRolloutEvidenceCommand(kind string) *cobra.Command {
	return &cobra.Command{
		Use:   kind + " <rollout-id> <deployment-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Retrieve authorised, decrypted Rollout deployment " + kind,
		RunE: func(cmd *cobra.Command, args []string) error {
			rollout, err := rolloutID(args[0])
			if err != nil {
				return err
			}
			deployment, err := uuid.Parse(args[1])
			if err != nil {
				return errors.Wrapf(err, "invalid deployment id %q", args[1])
			}
			orgID, err := ShouldOrg(cmd.Context())
			if err != nil {
				return err
			}
			if native, err := useNativeProgressiveRollouts(cmd); err != nil {
				return err
			} else if native {
				projectValue, _ := cmd.Flags().GetString("project-uuid")
				environmentValue, _ := cmd.Flags().GetString("environment-uuid")
				if (projectValue == "") != (environmentValue == "") {
					return errors.New("native material reference requires both --project-uuid and --environment-uuid")
				}
				if projectValue == "" {
					if err := requireNativeProgressiveRolloutsOperation(cmd.Context(), orgID, "getProgressiveRollout", progressiveRolloutsRead); err != nil {
						return errors.Wrap(err, "native material reference was not supplied and Rollout detail is unavailable")
					}
					detail, err := invokeNativeProgressiveRollout[cp.ProgressiveRolloutDetail](cmd.Context(), orgID, "getProgressiveRollout", map[string]string{rolloutIDPathParameter: rollout.String()}, nil, nil, "", http.StatusOK)
					if err != nil {
						return err
					}
					body, status, err := nativeProgressiveRolloutMaterial(cmd.Context(), orgID, *detail, deployment, kind)
					if err != nil {
						return err
					}
					if status != http.StatusOK {
						return rolloutResponseError("get protected rollout deployment "+kind, status, body)
					}
					if _, err := cmd.OutOrStdout().Write(body); err != nil {
						return err
					}
					if len(body) == 0 || body[len(body)-1] != '\n' {
						_, err = fmt.Fprintln(cmd.OutOrStdout())
					}
					return err
				}
				projectID, err := uuid.Parse(projectValue)
				if err != nil {
					return errors.Wrap(err, "native material reference requires a valid --project-uuid")
				}
				environmentID, err := uuid.Parse(environmentValue)
				if err != nil {
					return errors.Wrap(err, "native material reference requires a valid --environment-uuid")
				}
				response, err := MustPluginClient(cmd.Context()).GetPluginDeploymentMaterialWithResponse(cmd.Context(), orgID, progressiveRolloutsPluginID, projectID, environmentID, deployment, pluginapi.GetPluginDeploymentMaterialParamsMaterialKind(kind))
				if err != nil {
					return errors.Wrap(err, "failed to retrieve protected Rollout deployment "+kind)
				}
				if response.StatusCode() != http.StatusOK {
					return rolloutResponseError("get protected rollout deployment "+kind, response.StatusCode(), response.Body)
				}
				body := response.Body
				if _, err := cmd.OutOrStdout().Write(body); err != nil {
					return err
				}
				if len(body) == 0 || body[len(body)-1] != '\n' {
					_, err = fmt.Fprintln(cmd.OutOrStdout())
				}
				return err
			}
			var status int
			var body []byte
			if kind == "logs" {
				response, err := MustCpClient(cmd.Context()).GetProgressiveRolloutDeploymentLogsWithResponse(cmd.Context(), orgID, rollout, deployment)
				if err != nil {
					return errors.Wrap(err, "failed to retrieve Rollout deployment logs")
				}
				status, body = response.StatusCode(), response.Body
			} else {
				response, err := MustCpClient(cmd.Context()).GetProgressiveRolloutDeploymentOutputsWithResponse(cmd.Context(), orgID, rollout, deployment)
				if err != nil {
					return errors.Wrap(err, "failed to retrieve Rollout deployment outputs")
				}
				status, body = response.StatusCode(), response.Body
			}
			if status != http.StatusOK {
				return rolloutResponseError("get rollout deployment "+kind, status, body)
			}
			if _, err := cmd.OutOrStdout().Write(body); err != nil {
				return err
			}
			if len(body) == 0 || body[len(body)-1] != '\n' {
				_, err = fmt.Fprintln(cmd.OutOrStdout())
			}
			return err
		},
	}
}

func init() {
	CreateProgressiveRollout.Short += progressiveRolloutsHelp
	GetProgressiveRollout.Short += progressiveRolloutsHelp
	ListProgressiveRollouts.Short += progressiveRolloutsHelp
	UpdateProgressiveRollout.Short += progressiveRolloutsHelp
	CreateCmd.AddCommand(CreateProgressiveRollout)
	GetCmd.AddCommand(GetProgressiveRollout, ListProgressiveRollouts)
	UpdateCmd.AddCommand(UpdateProgressiveRollout)
	ListProgressiveRollouts.Flags().StringSlice("state", nil, "Filter by Rollout state")
	ListProgressiveRollouts.Flags().String("project", "", "Filter by frozen target Project ID")
	ListProgressiveRollouts.Flags().String("module", "", "Filter by target module ID")
	ListProgressiveRollouts.Flags().Bool("conflicts", false, "Filter by presence of conflicts")
	ListProgressiveRollouts.Flags().String("view", "", "Server-owned lifecycle view: running, plan, or history")
	ListProgressiveRollouts.Flags().String("history-visibility", "", "History retention subset: recent, archived, or all")
	CreateProgressiveRollout.Flags().String(rolloutIdempotencyFlag, "", "Stable idempotency key for native transport (a random UUID is used when omitted)")
	UpdateProgressiveRollout.Flags().String(rolloutIdempotencyFlag, "", "Stable idempotency key for native transport (a random UUID is used when omitted)")

	printer.SetupSingleOutputFormatFlag(ProgressiveRolloutCmd.PersistentFlags())
	PreviewProgressiveRollout.Flags().String(createUpdateCmdSetJsonFlag, "", "Read the Rollout spec from JSON or @file")
	PreviewProgressiveRollout.Flags().String(createUpdateCmdSetYamlFlag, "", "Read the Rollout spec from YAML or @file")
	PrepareProgressiveRollout.Flags().Int64(rolloutExpectedVersionFlag, 0, "Expected Rollout resource version")
	PrepareProgressiveRollout.Flags().String("candidate-fingerprint", "", "Fingerprint returned by preview")
	PrepareProgressiveRollout.Flags().StringSlice("accept-conflict", nil, "Acknowledge an exact planning-conflict fingerprint")
	PrepareProgressiveRollout.Flags().String(rolloutIdempotencyFlag, "", "Stable idempotency key")
	_ = PrepareProgressiveRollout.MarkFlagRequired(rolloutExpectedVersionFlag)
	_ = PrepareProgressiveRollout.MarkFlagRequired("candidate-fingerprint")

	ApproveProgressiveRollout.Flags().Int(rolloutRevisionFlag, 0, "Plan revision")
	ApproveProgressiveRollout.Flags().String("scope-type", "", "Approval scope: environment, or project for a bulk action")
	ApproveProgressiveRollout.Flags().String("scope-id", "", "Environment UUID, or Project UUID for a bulk action")
	ApproveProgressiveRollout.Flags().String("reason", "", "Mandatory human-readable approval reason")
	ApproveProgressiveRollout.Flags().String(rolloutIdempotencyFlag, "", "Stable idempotency key")
	_ = ApproveProgressiveRollout.MarkFlagRequired(rolloutRevisionFlag)
	_ = ApproveProgressiveRollout.MarkFlagRequired("scope-type")
	_ = ApproveProgressiveRollout.MarkFlagRequired("scope-id")
	_ = ApproveProgressiveRollout.MarkFlagRequired("reason")

	ResolveProgressiveRolloutConflict.Flags().Int(rolloutRevisionFlag, 0, "Plan revision")
	ResolveProgressiveRolloutConflict.Flags().Int64(rolloutExpectedVersionFlag, 0, "Expected Rollout resource version")
	ResolveProgressiveRolloutConflict.Flags().String("observed-fingerprint", "", "Exact observed conflict fingerprint")
	ResolveProgressiveRolloutConflict.Flags().String(rolloutIdempotencyFlag, "", "Stable idempotency key")
	_ = ResolveProgressiveRolloutConflict.MarkFlagRequired(rolloutRevisionFlag)
	_ = ResolveProgressiveRolloutConflict.MarkFlagRequired(rolloutExpectedVersionFlag)
	_ = ResolveProgressiveRolloutConflict.MarkFlagRequired("observed-fingerprint")
	ProgressiveRolloutEvents.Flags().Int64("after", 0, "Return events after this sequence")
	RevokeProgressiveRolloutApproval.Flags().Int(rolloutRevisionFlag, 0, "Plan revision")
	RevokeProgressiveRolloutApproval.Flags().String(rolloutIdempotencyFlag, "", "Stable idempotency key")
	_ = RevokeProgressiveRolloutApproval.MarkFlagRequired(rolloutRevisionFlag)
	ProgressiveRolloutPlan.Flags().Int(rolloutRevisionFlag, 0, "Plan revision")
	_ = ProgressiveRolloutPlan.MarkFlagRequired(rolloutRevisionFlag)
	approvals := newProgressiveRolloutRevisionListCommand("approvals")
	conflicts := newProgressiveRolloutRevisionListCommand("conflicts")
	approvals.Flags().Int(rolloutRevisionFlag, 0, "Plan revision")
	conflicts.Flags().Int(rolloutRevisionFlag, 0, "Plan revision")
	_ = approvals.MarkFlagRequired(rolloutRevisionFlag)
	_ = conflicts.MarkFlagRequired(rolloutRevisionFlag)

	logs := newProgressiveRolloutEvidenceCommand("logs")
	outputs := newProgressiveRolloutEvidenceCommand("outputs")
	for _, command := range []*cobra.Command{logs, outputs} {
		command.Flags().String("project-uuid", "", "Immutable Project UUID from the deployment receipt (required for native transport)")
		command.Flags().String("environment-uuid", "", "Immutable Environment UUID from the deployment receipt (required for native transport)")
	}
	ProgressiveRolloutCmd.AddCommand(PreviewProgressiveRollout, PrepareProgressiveRollout, ApproveProgressiveRollout,
		RevokeProgressiveRolloutApproval, ResolveProgressiveRolloutConflict, approvals, conflicts,
		ProgressiveRolloutEvents, ProgressiveRolloutPlan,
		logs, outputs)
	for _, command := range []string{"return-to-draft", "start", "start-wave", "bypass-bake", "override-pins", "continue-unpinned", "pause", "resume", "retry-environment", rolloutFailCommand, rolloutCancelCommand, rolloutCancelRollback, rolloutRollbackCommand, rolloutRollbackEnvironment, "archive"} {
		control := newProgressiveRolloutControlCommand(command)
		control.RunE = requireProgressiveRolloutsPlugin(progressiveRolloutCommandAccess(command), "commandProgressiveRollout", control.RunE)
		ProgressiveRolloutCmd.AddCommand(control)
	}
	for command, operation := range map[*cobra.Command]string{
		CreateProgressiveRollout: "createProgressiveRollout", UpdateProgressiveRollout: "updateProgressiveRolloutDraft",
		PreviewProgressiveRollout: "previewProgressiveRollout", PrepareProgressiveRollout: "prepareProgressiveRollout",
		ApproveProgressiveRollout: "approveProgressiveRolloutScope", RevokeProgressiveRolloutApproval: "revokeProgressiveRolloutApproval",
		ResolveProgressiveRolloutConflict: "resolveProgressiveRolloutConflict",
	} {
		command.RunE = requireProgressiveRolloutsPlugin(progressiveRolloutsMutate, operation, command.RunE)
	}
	for command, operation := range map[*cobra.Command]string{
		GetProgressiveRollout: "getProgressiveRollout", ListProgressiveRollouts: "listProgressiveRollouts",
		approvals: "listProgressiveRolloutApprovals", conflicts: "listProgressiveRolloutConflicts",
		ProgressiveRolloutEvents: "listProgressiveRolloutEvents", ProgressiveRolloutPlan: "getProgressiveRolloutPlan",
		logs: "", outputs: "",
	} {
		command.RunE = requireProgressiveRolloutsPlugin(progressiveRolloutsRead, operation, command.RunE)
	}
	RootCmd.AddCommand(ProgressiveRolloutCmd)
}

func progressiveRolloutCommandAccess(command string) progressiveRolloutsAccess {
	switch command {
	case "pause", rolloutCancelCommand, rolloutCancelRollback, rolloutRollbackCommand, rolloutRollbackEnvironment:
		return progressiveRolloutsSafety
	default:
		return progressiveRolloutsMutate
	}
}
