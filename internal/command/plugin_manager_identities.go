package command

import (
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
)

const (
	pluginManagerWorkloadIssuerFlag         = "workload-issuer"
	pluginManagerWorkloadNamespaceFlag      = "workload-namespace"
	pluginManagerWorkloadServiceAccountFlag = "workload-service-account"
	pluginManagerWorkloadAudienceFlag       = "workload-audience"
	pluginManagerCheckerExpiryFlag          = "checker-expiry-days"
	pluginManagerPrincipalNameFlag          = "principal-name"
	pluginManagerCheckerNameFlag            = "checker-name"
)

// pluginIdentities is what create-identities prints: the ids Core records and
// the admission checker's token, which IAM shows only once.
type pluginIdentities struct {
	BindingId        uuid.UUID                       `json:"binding_id"`
	ServiceUserId    uuid.UUID                       `json:"service_user_id"`
	AdmissionChecker *iam.PlatformPrincipalWithToken `json:"admission_checker,omitempty"`
	Installation     *cp.PluginInstallation          `json:"installation,omitempty"`
}

// iamRefusal reports an IAM answer the command cannot continue with.
func iamRefusal(op string, status int, raw []byte) error {
	return errors.Errorf("unexpected status code %d when trying to %s: %s", status, op, string(raw))
}

// workloadBinding returns the binding of the installation: the one IAM
// already holds, else a new one with a new platform principal.
func workloadBinding(cmd *cobra.Command, item cp.PluginInstallation) (iam.PluginWorkloadBinding, error) {
	client := MustIamClient(cmd.Context())
	list, err := client.ListPluginWorkloadBindingsWithResponse(cmd.Context())
	if err != nil {
		return iam.PluginWorkloadBinding{}, errors.Wrap(err, "failed to list plugin workload bindings")
	} else if list.JSON200 == nil {
		return iam.PluginWorkloadBinding{}, iamRefusal("list plugin workload bindings", list.StatusCode(), list.Body)
	}
	for _, binding := range list.JSON200.Items {
		if binding.InstallationId != item.InstallationId {
			continue
		}
		if binding.ServiceUserScope != iam.ServiceUserScopePlatform {
			return iam.PluginWorkloadBinding{}, errors.Errorf("IAM binds installation %s to service user %s of an organization; revoke binding %s in IAM (DELETE /plugin-workload-bindings/%s) and run the command again", item.InstallationId, binding.ServiceUserId, binding.Id, binding.Id)
		}
		infoMessageF("IAM already binds installation %s to platform principal %s.", item.InstallationId, binding.ServiceUserId)
		return binding, nil
	}
	flag := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }
	displayName := flag(pluginManagerPrincipalNameFlag)
	if displayName == "" {
		displayName = item.PluginId
	}
	body := iam.PluginWorkloadBindingCreateBody{
		Issuer: flag(pluginManagerWorkloadIssuerFlag), Namespace: flag(pluginManagerWorkloadNamespaceFlag),
		ServiceAccount: flag(pluginManagerWorkloadServiceAccountFlag), Audience: flag(pluginManagerWorkloadAudienceFlag),
		PluginId: item.PluginId, InstallationId: item.InstallationId,
		PlatformPrincipal: &iam.PluginWorkloadPrincipalCreateBody{DisplayName: displayName},
	}
	created, err := client.CreatePluginWorkloadBindingWithResponse(cmd.Context(), body)
	if err != nil {
		return iam.PluginWorkloadBinding{}, errors.Wrap(err, "failed to create the plugin workload binding")
	} else if created.JSON201 == nil {
		return iam.PluginWorkloadBinding{}, iamRefusal("create the plugin workload binding", created.StatusCode(), created.Body)
	}
	successMessageF("Bound installation %s to the new platform principal %s.", item.InstallationId, created.JSON201.ServiceUserId)
	return *created.JSON201, nil
}

// admissionChecker creates the static-token platform principal that checks
// Deployments for the Data Plane and the plugin.
func admissionChecker(cmd *cobra.Command, item cp.PluginInstallation) (*iam.PlatformPrincipalWithToken, error) {
	name, _ := cmd.Flags().GetString(pluginManagerCheckerNameFlag)
	if name == "" {
		name = item.PluginId + " admission checker"
	}
	days, _ := cmd.Flags().GetInt(pluginManagerCheckerExpiryFlag)
	created, err := MustIamClient(cmd.Context()).CreatePlatformPrincipalWithResponse(cmd.Context(), iam.PlatformPrincipalCreateBody{DisplayName: name, ExpiryInDays: days})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the admission checker")
	} else if created.JSON201 == nil {
		return nil, iamRefusal("create the admission checker", created.StatusCode(), created.Body)
	}
	successMessageF("Admission checker %s created. Its token will not be shown again.", created.JSON201.Id)
	return created.JSON201, nil
}

var CreatePluginIdentities = &cobra.Command{
	Use:   "create-identities <installation-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Create the installation's platform principals in IAM and record them before it is activated",
	Long: `Create the identities of an installation and record them.

An installation acts as a platform principal: a service user that belongs to no organization and holds no role. This
command asks IAM to bind the plugin's Kubernetes ServiceAccount to the installation with a new platform principal
(POST /plugin-workload-bindings), creates the admission checker, a platform principal with a static token
(POST /platform-principals, which the Data Plane presents too), and records both with the installation. It needs the
platform permission plugin_workload_binding_write and a user token. The installation must be in state installed; an
installation already bound in IAM keeps its principal.

The admission checker's token is shown once, in the output. If recording fails after it was created, the output still
carries it: pass the checker to the next run with --` + pluginManagerAdmissionCheckerFlag + ` instead of creating another.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		id, err := parseInstallationID(args[0])
		if err != nil {
			return err
		}
		var checker uuid.UUID
		if v, _ := cmd.Flags().GetString(pluginManagerAdmissionCheckerFlag); v != "" {
			if checker, err = uuid.Parse(v); err != nil {
				return errors.Wrapf(err, "invalid --%s", pluginManagerAdmissionCheckerFlag)
			}
		}
		w, err := newPluginManagerWriter(cmd)
		if err != nil {
			return err
		}
		item, err := w.installation(id)
		if err != nil {
			return err
		}
		binding, err := workloadBinding(cmd, item)
		if err != nil {
			return err
		}
		result := pluginIdentities{BindingId: binding.Id, ServiceUserId: binding.ServiceUserId}
		if checker == uuid.Nil {
			if result.AdmissionChecker, err = admissionChecker(cmd, item); err != nil {
				return err
			}
			checker = result.AdmissionChecker.Id
		}
		recorded, err := w.setIdentity(id, binding.ServiceUserId, checker)
		if err != nil {
			if result.AdmissionChecker != nil {
				_ = writePluginManagerResult(cmd, result)
			}
			return errors.Wrapf(err, "the identities exist in IAM; run the command again with --%s %s", pluginManagerAdmissionCheckerFlag, checker)
		}
		result.Installation = &recorded
		return writePluginManagerResult(cmd, result)
	},
}

func init() {
	flags := CreatePluginIdentities.Flags()
	flags.String(pluginManagerWorkloadIssuerFlag, "", "ServiceAccount token issuer of the cluster the plugin runs in, as IAM is configured")
	flags.String(pluginManagerWorkloadNamespaceFlag, "", "Kubernetes namespace of the plugin's ServiceAccount")
	flags.String(pluginManagerWorkloadServiceAccountFlag, "", "Name of the plugin's Kubernetes ServiceAccount")
	flags.String(pluginManagerWorkloadAudienceFlag, "", "The only audience of the plugin's projected ServiceAccount token")
	for _, name := range []string{pluginManagerWorkloadIssuerFlag, pluginManagerWorkloadNamespaceFlag, pluginManagerWorkloadServiceAccountFlag, pluginManagerWorkloadAudienceFlag} {
		_ = CreatePluginIdentities.MarkFlagRequired(name)
	}
	flags.String(pluginManagerPrincipalNameFlag, "", "Display name of the plugin's platform principal (default: the plugin ID)")
	flags.String(pluginManagerCheckerNameFlag, "", "Display name of the admission checker (default: the plugin ID and admission checker)")
	flags.Int(pluginManagerCheckerExpiryFlag, 365, "Days until the admission checker's token expires (1 to 3660)")
	flags.String(pluginManagerAdmissionCheckerFlag, "", "Use this existing platform principal as admission checker instead of creating one")
	flags.String(reasonFlag, "", "Why the change is made, recorded in the audit trail")
	_ = CreatePluginIdentities.MarkFlagRequired(reasonFlag)
	flags.Int64(pluginManagerExpectedRevisionFlag, 0, "The Plugin Manager revision the change was decided on (default: the current revision)")
	PluginManagerCmd.AddCommand(CreatePluginIdentities)
}
