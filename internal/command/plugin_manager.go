package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const (
	pluginManagerExpectedRevisionFlag = "expected-revision"
	pluginManagerManifestSHA256Flag   = "manifest-sha256"
	pluginManagerBackendURLFlag       = "backend-url"
	pluginManagerUIOriginFlag         = "ui-origin"
	pluginManagerServiceUserFlag      = "service-user"
	pluginManagerAdmissionCheckerFlag = "admission-checker"
	pluginManagerEnableOrgFlag        = "enable-org"
	pluginManagerTimezoneFlag         = "timezone"
	pluginManagerApprovalPolicyFlag   = "approval-policy"
	pluginManagerResumeFlag           = "resume"
	pluginAuditEventsAfterFlag        = "after"
	pluginAuditEventsLimitFlag        = "limit"

	pluginWorkUnsettled      = "plugin_work_unsettled"
	pluginInstallationExists = "plugin_installation_exists"
	pluginRevisionConflict   = "plugin_manager_revision_conflict"

	// Refusals after Core asked IAM about the installation's service user.
	pluginWorkloadBindingMissing = "plugin_workload_binding_missing"
	pluginServiceUserOrgMismatch = "plugin_service_user_organization_mismatch"
)

// pluginActivationHints turn the diagnostics of a refused activation into
// the operator's next step. A refused activation changes nothing.
var pluginActivationHints = map[string]string{
	"plugin_status_unreachable":            "Core could not read the plugin status from /.well-known/stellwerk/plugin-status on the backend URL. Check that the plugin runtime is running and that the Control Plane reaches the backend URL and trusts its certificate",
	"plugin_status_invalid":                "The plugin answered with an invalid status document. Check that the plugin runtime supports this Core's plugin contract and read its logs",
	"plugin_installation_mismatch":         "The running plugin reports another installation UUID. Configure the plugin runtime with installation %s and let it roll out",
	"plugin_package_digest_mismatch":       "The running plugin verified another package. Deploy the package Core expects (the pending upgrade's, if any) to the plugin runtime and wait for its rollout",
	"plugin_migration_generation_mismatch": "The plugin database is not at the migration generation the package declares. Let the plugin runtime finish its migrations",
}

// pluginManagerError turns a refused Plugin Manager request into an error
// that names the stable code and the operator's next step.
func pluginManagerError(op string, status int, raw []byte, installationID string) error {
	var body cp.Error
	if json.Unmarshal(raw, &body) != nil || body.Error == "" {
		return errors.Errorf("unexpected status code %d when trying to %s: %s", status, op, string(raw))
	}
	refused := fmt.Sprintf("failed to %s (%s): %s", op, body.Error, strings.TrimSuffix(body.Message, "."))
	if hint, ok := pluginActivationHints[body.Error]; ok {
		if strings.Contains(hint, "%s") {
			hint = fmt.Sprintf(hint, installationID)
		}
		return errors.Errorf("%s. %s, then run the command again: a refused activation changed nothing", refused, hint)
	}
	switch body.Error {
	case pluginWorkUnsettled:
		return errors.Errorf("%s. Core still holds work of this plugin (operation grants, dispatches or Module reservations) and uninstall has no force option. Stop new work with 'octl plugin-manager disable %s --%s <text>', let the running work settle, then uninstall again", refused, installationID, reasonFlag)
	case pluginInstallationExists:
		return errors.Errorf("%s. Continue the existing installation with the same command and --%s", refused, pluginManagerResumeFlag)
	case pluginWorkloadBindingMissing:
		return errors.Errorf("%s. The binding names the installation UUID, so it can only exist after the installation: create it for installation %s in IAM "+
			"(POST /plugin-workload-bindings, platform permission plugin_workload_binding_write), then run the command again", refused, installationID)
	case pluginServiceUserOrgMismatch:
		return errors.Errorf("%s. Enable the organization of the installation's service user instead, or give the installation a service user of this organization: "+
			"create the service user there, bind it to installation %s in IAM, record it with 'octl plugin-manager set-identity' "+
			"(only in state installed: uninstall and reinstall an active installation first), activate it, then enable the organization", refused, installationID)
	case pluginRevisionConflict:
		return errors.Errorf("%s. Review the current state with 'octl get plugin-installations' and run the command again; without --%s it uses the current revision", refused, pluginManagerExpectedRevisionFlag)
	case "plugin_manager_forbidden":
		return errors.Errorf("%s. The Plugin Manager is for platform operators: reads need the platform permission plugin_manager_read, changes plugin_manager_write", refused)
	case "plugin_host_disabled":
		return errors.Errorf("%s. Enable the plugin host of this Control Plane first", refused)
	}
	return errors.New(refused)
}

// pluginManagerIdempotencyKey derives the key from the request itself. The
// identical command (same target, body, reason and expected revision) gets
// the same key, so sending it again after a lost answer replays the stored
// answer instead of changing anything twice.
func pluginManagerIdempotencyKey(method, path string, body any) string {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err) // generated request bodies always marshal
	}
	sum := sha256.Sum256(append([]byte(method+" "+path+"\n"), raw...))
	return "octl-" + hex.EncodeToString(sum[:20])
}

// pluginManagerWriter sends Plugin Manager commands for one CLI command.
// Every command carries a reason, an idempotency key and the Plugin Manager
// revision it was decided on: --expected-revision for the first command,
// otherwise the revision read when the CLI command started, and after that
// the revision each answer reports.
type pluginManagerWriter struct {
	ctx      context.Context
	client   cp.ClientWithResponsesInterface
	reason   string
	revision int64
	// installations as read when the CLI command started.
	installations []cp.PluginInstallation
}

func newPluginManagerWriter(cmd *cobra.Command) (*pluginManagerWriter, error) {
	w := &pluginManagerWriter{ctx: cmd.Context(), client: MustCpClient(cmd.Context())}
	w.reason, _ = cmd.Flags().GetString(reasonFlag)
	r, err := w.client.ListPluginInstallationsWithResponse(w.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the Plugin Manager")
	} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
		return nil, pluginManagerError("read the Plugin Manager", r.StatusCode(), r.Body, "")
	}
	w.revision, w.installations = r.JSON200.Revision, r.JSON200.Items
	if cmd.Flags().Changed(pluginManagerExpectedRevisionFlag) {
		w.revision, _ = cmd.Flags().GetInt64(pluginManagerExpectedRevisionFlag)
	}
	return w, nil
}

func (w *pluginManagerWriter) command() cp.PluginManagerCommand {
	return cp.PluginManagerCommand{ExpectedRevision: w.revision, Reason: w.reason}
}

// installation returns the installation as read when the command started.
func (w *pluginManagerWriter) installation(id uuid.UUID) (cp.PluginInstallation, error) {
	for _, item := range w.installations {
		if item.InstallationId == id {
			return item, nil
		}
	}
	return cp.PluginInstallation{}, errors.Errorf("plugin installation '%s' not found", id)
}

// installationResult checks a command's answer, follows its revision and
// prints the resulting state.
func (w *pluginManagerWriter) installationResult(op string, id string, status int, raw []byte, result *cp.PluginInstallation) (cp.PluginInstallation, error) {
	if status/100 != 2 || result == nil {
		return cp.PluginInstallation{}, pluginManagerError(op, status, raw, id)
	}
	if result.Revision != nil {
		w.revision = *result.Revision
	}
	printPluginInstallationState(*result)
	return *result, nil
}

func printPluginInstallationState(item cp.PluginInstallation) {
	state := fmt.Sprintf("Installation %s of %s: %s, availability %s", item.InstallationId, item.PluginId, item.State, item.Availability)
	if item.TargetPackageSha256 != nil {
		state += ", upgrade to package " + *item.TargetPackageSha256 + " pending"
	}
	if item.Revision != nil {
		state += fmt.Sprintf(", revision %d", *item.Revision)
	}
	successMessageF("%s.", state)
	if item.Diagnostic != nil {
		changedMessageF("Diagnostic %s: %s", item.Diagnostic.Code, item.Diagnostic.Message)
	}
}

func (w *pluginManagerWriter) create(body cp.PluginInstallationCreate) (cp.PluginInstallation, error) {
	body.ExpectedRevision, body.Reason = w.revision, w.reason
	params := &cp.CreatePluginInstallationParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodPost, "/plugin-installations", body)}
	r, err := w.client.CreatePluginInstallationWithResponse(w.ctx, params, body)
	if err != nil {
		return cp.PluginInstallation{}, errors.Wrap(err, "failed to install the package")
	}
	return w.installationResult("install package "+body.PackageSha256, "", r.StatusCode(), r.Body, r.JSON201)
}

func (w *pluginManagerWriter) setIdentity(id, serviceUser, admissionChecker uuid.UUID) (cp.PluginInstallation, error) {
	body := cp.PluginInstallationIdentity{ExpectedRevision: w.revision, Reason: w.reason, ServiceUserId: serviceUser, AdmissionCheckerId: admissionChecker}
	params := &cp.SetPluginInstallationIdentityParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodPut, fmt.Sprintf("/plugin-installations/%s/identity", id), body)}
	r, err := w.client.SetPluginInstallationIdentityWithResponse(w.ctx, id, params, body)
	if err != nil {
		return cp.PluginInstallation{}, errors.Wrap(err, "failed to set the installation identity")
	}
	return w.installationResult("set the identity of installation "+id.String(), id.String(), r.StatusCode(), r.Body, r.JSON200)
}

func (w *pluginManagerWriter) transition(id uuid.UUID, action cp.TransitionPluginInstallationParamsInstallationAction) (cp.PluginInstallation, error) {
	body := w.command()
	params := &cp.TransitionPluginInstallationParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodPost, fmt.Sprintf("/plugin-installations/%s/actions/%s", id, action), body)}
	r, err := w.client.TransitionPluginInstallationWithResponse(w.ctx, id, action, params, body)
	if err != nil {
		return cp.PluginInstallation{}, errors.Wrapf(err, "failed to %s the installation", action)
	}
	return w.installationResult(fmt.Sprintf("%s installation %s", action, id), id.String(), r.StatusCode(), r.Body, r.JSON200)
}

func (w *pluginManagerWriter) upgrade(id uuid.UUID, packageSHA256 string) (cp.PluginInstallation, error) {
	body := cp.PluginInstallationUpgrade{ExpectedRevision: w.revision, Reason: w.reason, PackageSha256: packageSHA256}
	params := &cp.UpgradePluginInstallationParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodPost, fmt.Sprintf("/plugin-installations/%s/upgrade", id), body)}
	r, err := w.client.UpgradePluginInstallationWithResponse(w.ctx, id, params, body)
	if err != nil {
		return cp.PluginInstallation{}, errors.Wrap(err, "failed to upgrade the installation")
	}
	return w.installationResult("upgrade installation "+id.String(), id.String(), r.StatusCode(), r.Body, r.JSON200)
}

func (w *pluginManagerWriter) purge(id uuid.UUID) (cp.PluginInstallation, error) {
	body := w.command()
	params := &cp.PurgePluginInstallationParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodDelete, fmt.Sprintf("/plugin-installations/%s", id), body)}
	r, err := w.client.PurgePluginInstallationWithResponse(w.ctx, id, params, body)
	if err != nil {
		return cp.PluginInstallation{}, errors.Wrap(err, "failed to purge the installation")
	}
	return w.installationResult("purge installation "+id.String(), id.String(), r.StatusCode(), r.Body, r.JSON200)
}

// putEnablement enables or disables the installation for an organization.
// Unset settings keep the organization's current ones: the request replaces
// the enablement as a whole.
func (w *pluginManagerWriter) putEnablement(cmd *cobra.Command, current cp.PluginInstallation, orgId string, state cp.PluginEnablementPutState) (cp.PluginInstallation, error) {
	body := cp.PluginEnablementPut{ExpectedRevision: w.revision, Reason: w.reason, State: state}
	for _, enablement := range current.Organizations {
		if enablement.OrgId == orgId {
			body.Timezone, body.ApprovalPolicies = &enablement.Timezone, &enablement.ApprovalPolicies
		}
	}
	if cmd.Flags().Changed(pluginManagerTimezoneFlag) {
		timezone, _ := cmd.Flags().GetString(pluginManagerTimezoneFlag)
		body.Timezone = &timezone
	}
	if cmd.Flags().Changed(pluginManagerApprovalPolicyFlag) {
		policies, _ := cmd.Flags().GetStringToString(pluginManagerApprovalPolicyFlag)
		body.ApprovalPolicies = &policies
	}
	id := current.InstallationId
	params := &cp.PutPluginEnablementParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodPut, fmt.Sprintf("/plugin-installations/%s/organizations/%s", id, orgId), body)}
	r, err := w.client.PutPluginEnablementWithResponse(w.ctx, id, orgId, params, body)
	if err != nil {
		return cp.PluginInstallation{}, errors.Wrapf(err, "failed to change organization %s", orgId)
	}
	return w.installationResult(fmt.Sprintf("set organization %s of installation %s to %s", orgId, id, state), id.String(), r.StatusCode(), r.Body, r.JSON200)
}

// removeEnablement removes the installation's enablement for an
// organization. Removing an absent enablement changes nothing.
func (w *pluginManagerWriter) removeEnablement(id uuid.UUID, orgId string) (cp.PluginInstallation, error) {
	body := w.command()
	params := &cp.RemovePluginEnablementParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodDelete, fmt.Sprintf("/plugin-installations/%s/organizations/%s", id, orgId), body)}
	r, err := w.client.RemovePluginEnablementWithResponse(w.ctx, id, orgId, params, body)
	if err != nil {
		return cp.PluginInstallation{}, errors.Wrapf(err, "failed to remove organization %s", orgId)
	}
	return w.installationResult(fmt.Sprintf("remove organization %s from installation %s", orgId, id), id.String(), r.StatusCode(), r.Body, r.JSON200)
}

// enablementHolds says whether the organization is enabled with the
// settings the flags ask for.
func enablementHolds(cmd *cobra.Command, item cp.PluginInstallation, orgId string) bool {
	for _, enablement := range item.Organizations {
		if enablement.OrgId != orgId || enablement.State != cp.PluginEnablementStateEnabled {
			continue
		}
		timezone, _ := cmd.Flags().GetString(pluginManagerTimezoneFlag)
		policies, _ := cmd.Flags().GetStringToString(pluginManagerApprovalPolicyFlag)
		return (!cmd.Flags().Changed(pluginManagerTimezoneFlag) || timezone == enablement.Timezone) &&
			(!cmd.Flags().Changed(pluginManagerApprovalPolicyFlag) || maps.Equal(policies, enablement.ApprovalPolicies))
	}
	return false
}

func parseInstallationID(arg string) (uuid.UUID, error) {
	id, err := uuid.Parse(arg)
	return id, errors.Wrapf(err, "invalid installation id '%s'", arg)
}

func writePluginManagerResult(cmd *cobra.Command, item any) error {
	return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), item)
}

// PluginManagerCmd groups the platform operator's Plugin Manager commands.
// Reading the Plugin Manager stays under 'octl get'.
var PluginManagerCmd = &cobra.Command{
	Use:     "plugin-manager",
	Aliases: []string{"pm"},
	Short:   "Register, install and operate plugins of the platform (platform operators only)",
	Long: `Register, install and operate plugins of the platform.

The Plugin Manager is platform-scoped and only for platform operators: reads need the platform permission
plugin_manager_read, changes plugin_manager_write. Organization roles never qualify.

Every change requires --reason, which the audit trail records ('octl get plugin-audit-events'). It is sent with the
Plugin Manager revision it was decided on (--expected-revision, by default the current one) and an idempotency key
derived from the request: if anything changed in between, the change is refused, and sending the identical command
again replays the recorded answer. Each command prints the resulting state and revision.`,
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

var RegisterPluginPackage = &cobra.Command{
	Use:   "register-package <registration-manifest-file>",
	Args:  cobra.ExactArgs(1),
	Short: "Register an immutable plugin package from its registration manifest (JSON or YAML)",
	Long: `Register an immutable plugin package from its registration manifest (JSON or YAML).

--manifest-sha256 is the SHA-256 of the package manifest the plugin runtime verifies; installations and upgrades name
the package by it. Registering the same digest with the same manifest again changes nothing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return errors.Wrap(err, "failed to read the registration manifest")
		}
		manifest, err := yaml.YAMLToJSON(raw)
		if err != nil {
			return errors.Wrap(err, "registration manifest is neither JSON nor YAML")
		}
		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		digest, _ := cmd.Flags().GetString(pluginManagerManifestSHA256Flag)
		body := cp.PluginPackagePush{ExpectedRevision: w.revision, Reason: w.reason, ManifestSha256: digest, Manifest: manifest}
		r, err := w.client.PushPluginPackageWithResponse(w.ctx, &cp.PushPluginPackageParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodPost, "/plugin-packages", body)}, body)
		if err != nil {
			return errors.Wrap(err, "failed to register the package")
		} else if r.StatusCode() != http.StatusCreated || r.JSON201 == nil {
			return pluginManagerError("register package "+digest, r.StatusCode(), r.Body, "")
		}
		state := fmt.Sprintf("Package %s %s (%s) registered", r.JSON201.PluginId, r.JSON201.Version, r.JSON201.ManifestSha256)
		if r.JSON201.Revision != nil {
			state += fmt.Sprintf(", revision %d", *r.JSON201.Revision)
		}
		successMessageF("%s.", state)
		return writePluginManagerResult(cmd, *r.JSON201)
	},
}

var InstallPlugin = &cobra.Command{
	Use:   "install <package-sha256>",
	Args:  cobra.ExactArgs(1),
	Short: "Install a registered package: create the installation, record its identity, activate it and enable an organization",
	Long: `Install a registered package and take it as far as the flags say:

  1. create the installation (Core assigns the installation UUID the plugin runtime, IAM and licences name),
  2. record the plugin service user and admission checker (--service-user, --admission-checker): the service user
     belongs to the organization to enable, and IAM binds it to the plugin and the new installation UUID,
  3. activate it once the running plugin proves the installation UUID, package and migration generation,
  4. enable it for the organization of its service user (--enable-org, with --timezone and --approval-policy).

When a step fails, for example because the IAM workload binding or the plugin runtime does not exist yet, the steps
before it stay done. Run the same command again with --resume to continue from the state the Plugin Manager recorded:
it finds the plugin's installation and skips every step whose target state already holds, in the order above. Nothing
is kept locally.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		digest := args[0]
		backendURL, _ := cmd.Flags().GetString(pluginManagerBackendURLFlag)
		uiOrigin, _ := cmd.Flags().GetString(pluginManagerUIOriginFlag)
		orgId, _ := cmd.Flags().GetString(pluginManagerEnableOrgFlag)
		var serviceUser, admissionChecker uuid.UUID
		if v, _ := cmd.Flags().GetString(pluginManagerServiceUserFlag); v != "" {
			var err error
			if serviceUser, err = uuid.Parse(v); err != nil {
				return errors.Wrapf(err, "invalid --%s", pluginManagerServiceUserFlag)
			}
			v, _ = cmd.Flags().GetString(pluginManagerAdmissionCheckerFlag)
			if admissionChecker, err = uuid.Parse(v); err != nil {
				return errors.Wrapf(err, "invalid --%s", pluginManagerAdmissionCheckerFlag)
			}
		}
		if orgId == "" && (cmd.Flags().Changed(pluginManagerTimezoneFlag) || cmd.Flags().Changed(pluginManagerApprovalPolicyFlag)) {
			return errors.Errorf("--%s and --%s need --%s", pluginManagerTimezoneFlag, pluginManagerApprovalPolicyFlag, pluginManagerEnableOrgFlag)
		}

		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		var recorded *cp.PluginInstallation
		if resume, _ := cmd.Flags().GetBool(pluginManagerResumeFlag); resume {
			if recorded, err = w.recordedInstallation(digest); err != nil {
				return err
			}
		}
		var item cp.PluginInstallation
		if recorded == nil {
			body := cp.PluginInstallationCreate{PackageSha256: digest, BackendUrl: backendURL}
			if uiOrigin != "" {
				body.UiOrigin = &uiOrigin
			}
			if item, err = w.create(body); err != nil {
				return err
			}
		} else {
			item = *recorded
			if err := resumable(item, digest, backendURL, uiOrigin); err != nil {
				return err
			}
		}

		// From here on the installation is recorded: every step whose target
		// state already holds is skipped, so --resume continues after a failure.
		steps := []struct {
			pending func() bool
			run     func() (cp.PluginInstallation, error)
		}{
			{
				pending: func() bool { return serviceUser != uuid.Nil && !identityRecorded(item, serviceUser, admissionChecker) },
				run: func() (cp.PluginInstallation, error) {
					return w.setIdentity(item.InstallationId, serviceUser, admissionChecker)
				},
			},
			{
				pending: func() bool { return item.State == cp.PluginInstallationStateInstalled },
				run: func() (cp.PluginInstallation, error) {
					return w.transition(item.InstallationId, cp.PluginInstallationActionActivate)
				},
			},
			{
				pending: func() bool { return orgId != "" && !enablementHolds(cmd, item, orgId) },
				run: func() (cp.PluginInstallation, error) {
					return w.putEnablement(cmd, item, orgId, cp.PluginEnablementPutStateEnabled)
				},
			},
		}
		for _, step := range steps {
			if !step.pending() {
				continue
			}
			next, err := step.run()
			if err != nil {
				return errors.Wrapf(err, "installation %s is recorded; continue with --%s", item.InstallationId, pluginManagerResumeFlag)
			}
			item = next
		}
		return writePluginManagerResult(cmd, item)
	},
}

func identityRecorded(item cp.PluginInstallation, serviceUser, admissionChecker uuid.UUID) bool {
	return item.ServiceUserId != nil && *item.ServiceUserId == serviceUser &&
		item.AdmissionCheckerId != nil && *item.AdmissionCheckerId == admissionChecker
}

// recordedInstallation finds the installation of the package's plugin that
// an earlier install recorded, or nil when there is none to resume.
func (w *pluginManagerWriter) recordedInstallation(digest string) (*cp.PluginInstallation, error) {
	r, err := w.client.ListPluginPackagesWithResponse(w.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list plugin packages")
	} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
		return nil, pluginManagerError("list plugin packages", r.StatusCode(), r.Body, "")
	}
	pluginId := ""
	for _, item := range r.JSON200.Items {
		if item.ManifestSha256 == digest {
			pluginId = item.PluginId
		}
	}
	if pluginId == "" {
		return nil, errors.Errorf("package %s is not registered; register it with 'octl plugin-manager register-package'", digest)
	}
	for _, item := range w.installations {
		if item.PluginId == pluginId && item.State != cp.PluginInstallationStatePurged {
			infoMessageF("Resuming installation %s of %s: %s at revision %d.", item.InstallationId, pluginId, item.State, w.revision)
			return &item, nil
		}
	}
	infoMessageF("No installation of %s recorded; installing.", pluginId)
	return nil, nil
}

// resumable refuses to resume an installation that is not the one the
// command describes or that an operator deliberately took out of the
// install flow.
func resumable(item cp.PluginInstallation, digest, backendURL, uiOrigin string) error {
	id := item.InstallationId
	switch {
	case item.PackageSha256 != digest:
		return errors.Errorf("installation %s serves package %s, not %s; request the new package with 'octl plugin-manager upgrade %s %s'", id, item.PackageSha256, digest, id, digest)
	case item.BackendUrl != backendURL:
		return errors.Errorf("installation %s uses backend URL %s, not %s", id, item.BackendUrl, backendURL)
	case uiOrigin != "" && (item.UiOrigin == nil || *item.UiOrigin != uiOrigin):
		return errors.Errorf("installation %s does not use UI origin %s", id, uiOrigin)
	case item.State == cp.PluginInstallationStateDisabled:
		return errors.Errorf("installation %s was disabled by an operator; enable it with 'octl plugin-manager enable %s --%s <text>'", id, id, reasonFlag)
	case item.State == cp.PluginInstallationStateUninstalled:
		return errors.Errorf("installation %s is uninstalled; reinstall it with 'octl plugin-manager reinstall %s --%s <text>' or purge it", id, id, reasonFlag)
	}
	return nil
}

var SetPluginIdentity = &cobra.Command{
	Use:   "set-identity <installation-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Record the plugin service user and admission checker of an installation before it is activated",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		id, err := parseInstallationID(args[0])
		if err != nil {
			return err
		}
		var identities [2]uuid.UUID
		for i, flag := range []string{pluginManagerServiceUserFlag, pluginManagerAdmissionCheckerFlag} {
			v, _ := cmd.Flags().GetString(flag)
			if identities[i], err = uuid.Parse(v); err != nil {
				return errors.Wrapf(err, "invalid --%s", flag)
			}
		}
		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		item, err := w.setIdentity(id, identities[0], identities[1])
		if err != nil {
			return err
		}
		return writePluginManagerResult(cmd, item)
	},
}

// transitionCommand builds the command for one installation action.
func transitionCommand(action cp.TransitionPluginInstallationParamsInstallationAction, short string) *cobra.Command {
	return &cobra.Command{
		Use:   string(action) + " <installation-id>",
		Args:  cobra.ExactArgs(1),
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			id, err := parseInstallationID(args[0])
			if err != nil {
				return err
			}
			w, err := newPluginManagerWriter(cmd)
			if err != nil {
				return err
			}
			item, err := w.transition(id, action)
			if err != nil {
				return err
			}
			return writePluginManagerResult(cmd, item)
		},
	}
}

var (
	ActivatePlugin = transitionCommand(cp.PluginInstallationActionActivate,
		"Serve an installation once the running plugin proves its installation UUID, package and migration generation; also completes a pending upgrade")
	DisablePlugin = transitionCommand(cp.PluginInstallationActionDisable,
		"Keep an active installation safety-only for every organization: running work can settle, no new work starts")
	EnablePlugin = transitionCommand(cp.PluginInstallationActionEnable,
		"Return a disabled installation to active")
	UninstallPlugin = transitionCommand(cp.PluginInstallationActionUninstall,
		"Remove an installation's registrations and organizations; keeps its UUID, licence binding and plugin data")
	ReinstallPlugin = transitionCommand(cp.PluginInstallationActionReinstall,
		"Return an uninstalled installation, with its UUID, to installed")
)

var UpgradePlugin = &cobra.Command{
	Use:   "upgrade <installation-id> <package-sha256>",
	Args:  cobra.ExactArgs(2),
	Short: "Request the package an installation serves next; 'activate' switches once the plugin runtime runs it",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		id, err := parseInstallationID(args[0])
		if err != nil {
			return err
		}
		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		item, err := w.upgrade(id, args[1])
		if err != nil {
			return err
		}
		if item.TargetPackageSha256 != nil {
			infoMessageF("Core keeps serving package %s. Roll the plugin runtime to package %s, then switch with 'octl plugin-manager activate %s --%s <text>'.", item.PackageSha256, *item.TargetPackageSha256, id, reasonFlag)
		}
		return writePluginManagerResult(cmd, item)
	},
}

var PurgePlugin = &cobra.Command{
	Use:   "purge <installation-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Retire an uninstalled installation's UUID for good; its licence grants never match again",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		id, err := parseInstallationID(args[0])
		if err != nil {
			return err
		}
		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		item, err := w.purge(id)
		if err != nil {
			return err
		}
		infoMessageF("Core does not own plugin data: delete it through the plugin runtime.")
		return writePluginManagerResult(cmd, item)
	},
}

// enablementCommand builds the command that sets an organization's
// enablement to state.
func enablementCommand(use string, state cp.PluginEnablementPutState, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <installation-id> <org-id>",
		Args:  cobra.ExactArgs(2),
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			id, err := parseInstallationID(args[0])
			if err != nil {
				return err
			}
			w, err := newPluginManagerWriter(cmd)
			if err != nil {
				return err
			}
			current, err := w.installation(id)
			if err != nil {
				return err
			}
			item, err := w.putEnablement(cmd, current, args[1], state)
			if err != nil {
				return err
			}
			return writePluginManagerResult(cmd, item)
		},
	}
}

var (
	EnablePluginOrganization = enablementCommand("enable-org", cp.PluginEnablementPutStateEnabled,
		"Enable an active installation for the organization of its service user")
	DisablePluginOrganization = enablementCommand("disable-org", cp.PluginEnablementPutStateDisabled,
		"Keep an installation safety-only for an organization")
)

var RemovePluginOrganization = &cobra.Command{
	Use:   "remove-org <installation-id> <org-id>",
	Args:  cobra.ExactArgs(2),
	Short: "Remove an installation's enablement for an organization, with its timezone and approval policies",
	Long: `Remove an installation's enablement for an organization, with its timezone and approval policies.

The organization then no longer discovers the plugin: reads and safety actions such as cancel and rollback stop too.
disable-org instead keeps the plugin safety-only for the organization and keeps its settings. Core does not wait for
the organization's running plugin work, so disable the organization first and let that work settle. When no resource
event subscriber remains, the organization's resource event journal pauses; cursors issued before a later resume
expire with plugin_resource_cursor_expired.

Removing an enablement that does not exist changes nothing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		id, err := parseInstallationID(args[0])
		if err != nil {
			return err
		}
		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		decidedOn := w.revision
		item, err := w.removeEnablement(id, args[1])
		if err != nil {
			return err
		}
		if item.Revision != nil && *item.Revision == decidedOn {
			changedMessageF("Installation %s had no enablement for organization %s: nothing changed.", id, args[1])
		}
		return writePluginManagerResult(cmd, item)
	},
}

var ImportPluginEntitlements = &cobra.Command{
	Use:   "import-entitlements <bundle-file>",
	Args:  cobra.ExactArgs(1),
	Short: "Import a signed entitlement bundle: licence grants and revocations",
	Long: `Import a signed entitlement bundle: licence grants and revocations.

Each document must verify against the configured issuer trust, and a grant must name exactly the installation UUID,
plugin and organization of an enabled installation. A rejected document is reported and never blocks the others.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		bundle, err := os.ReadFile(args[0])
		if err != nil {
			return errors.Wrap(err, "failed to read the entitlement bundle")
		}
		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		body := cp.PluginEntitlementBundleImportRequest{ExpectedRevision: w.revision, Reason: w.reason, Bundle: bundle}
		r, err := w.client.ImportPluginEntitlementBundleWithResponse(w.ctx, &cp.ImportPluginEntitlementBundleParams{IdempotencyKey: pluginManagerIdempotencyKey(http.MethodPost, "/plugin-entitlement-bundles", body)}, body)
		if err != nil {
			return errors.Wrap(err, "failed to import the entitlement bundle")
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return pluginManagerError("import the entitlement bundle", r.StatusCode(), r.Body, "")
		}
		result := *r.JSON200
		successMessageF("Imported %d grants and %d revocations, revision %d.", result.ImportedGrants, result.ImportedRevocations, result.Revision)
		for _, diagnostic := range result.Diagnostics {
			changedMessageF("Skipped a document (%s): %s", diagnostic.Code, diagnostic.Detail)
		}
		return writePluginManagerResult(cmd, result)
	},
}

func init() {
	printer.SetupSingleOutputFormatFlag(PluginManagerCmd.PersistentFlags())
	writes := []*cobra.Command{RegisterPluginPackage, InstallPlugin, SetPluginIdentity, ActivatePlugin, DisablePlugin, EnablePlugin, UninstallPlugin, ReinstallPlugin, UpgradePlugin, PurgePlugin, EnablePluginOrganization, DisablePluginOrganization, RemovePluginOrganization, ImportPluginEntitlements}
	for _, write := range writes {
		write.Flags().String(reasonFlag, "", "Why the change is made, recorded in the audit trail")
		_ = write.MarkFlagRequired(reasonFlag)
		write.Flags().Int64(pluginManagerExpectedRevisionFlag, 0, "The Plugin Manager revision the change was decided on (default: the current revision)")
	}

	RegisterPluginPackage.Flags().String(pluginManagerManifestSHA256Flag, "", "SHA-256 of the package manifest the plugin runtime verifies")
	_ = RegisterPluginPackage.MarkFlagRequired(pluginManagerManifestSHA256Flag)

	InstallPlugin.Flags().String(pluginManagerBackendURLFlag, "", "Origin of the plugin backend")
	_ = InstallPlugin.MarkFlagRequired(pluginManagerBackendURLFlag)
	InstallPlugin.Flags().String(pluginManagerUIOriginFlag, "", "Origin of the plugin UI, exactly when the package contributes a UI")
	InstallPlugin.Flags().Bool(pluginManagerResumeFlag, false, "Continue the plugin's recorded installation after a partial failure")
	InstallPlugin.Flags().String(pluginManagerEnableOrgFlag, "", "Enable the installation for this organization, the organization of its service user")
	for _, cmd := range []*cobra.Command{InstallPlugin, SetPluginIdentity} {
		cmd.Flags().String(pluginManagerServiceUserFlag, "", "IAM service user the plugin acts as")
		cmd.Flags().String(pluginManagerAdmissionCheckerFlag, "", "IAM service user of the plugin's admission checker")
		cmd.MarkFlagsRequiredTogether(pluginManagerServiceUserFlag, pluginManagerAdmissionCheckerFlag)
	}
	_ = SetPluginIdentity.MarkFlagRequired(pluginManagerServiceUserFlag)
	for _, cmd := range []*cobra.Command{InstallPlugin, EnablePluginOrganization, DisablePluginOrganization} {
		cmd.Flags().String(pluginManagerTimezoneFlag, "", "IANA timezone of the organization for calendar policies (default: the current one, else UTC)")
		cmd.Flags().StringToString(pluginManagerApprovalPolicyFlag, nil, "Deployment approval scope=policy, for example environment=two-person (default: the current ones)")
	}

	PluginManagerCmd.AddCommand(writes...)
	RootCmd.AddCommand(PluginManagerCmd)
}
