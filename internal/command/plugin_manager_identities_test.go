package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
)

const (
	pmTestIssuer    = "https://kubernetes.example.test"
	pmTestNamespace = "plugins"
	pmTestAccount   = "progressive-rollouts"
	pmTestAudience  = "stellwerk-iam"
	pmIamBindings   = "/plugin-workload-bindings"
	pmIamPrincipals = "/platform-principals"
	pmIdentitiesCmd = "create-identities"
	pmOrgModeCmd    = "organization-mode"
	pmTestOrgBeta   = "beta"
	pmTestOrgGamma  = "gamma"
)

// iamServer is an IAM fake for the platform principal endpoints that keeps
// IAM's rules: one binding per installation, the configured issuer, and
// platform principals that only exist together with their binding or with a
// static token.
type iamServer struct {
	t  *testing.T
	mu sync.Mutex

	url      string
	bindings []iam.PluginWorkloadBinding
	// organizationBound makes a new binding name a service user of an
	// organization, as bindings did before platform principals existed.
	organizationBound bool
	forbidden         bool
	requests          []string
	bodies            []map[string]any
}

func newIamServer(t *testing.T) *iamServer {
	t.Helper()
	f := &iamServer{t: t}
	server := httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

func (f *iamServer) requestLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *iamServer) clearRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests, f.bodies = nil, nil
}

func iamWrite(t *testing.T, w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	assert.NoError(t, json.NewEncoder(w).Encode(value))
}

func (f *iamServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := io.ReadAll(r.Body)
	assert.NoError(f.t, err)
	var body map[string]any
	if len(raw) > 0 {
		assert.NoError(f.t, json.Unmarshal(raw, &body))
	}
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.bodies = append(f.bodies, body)
	if f.forbidden {
		iamWrite(f.t, w, http.StatusForbidden, iam.Error{Error: "forbidden", Message: "plugin_workload_binding_write is required"})
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET " + pmIamBindings:
		iamWrite(f.t, w, http.StatusOK, iam.PluginWorkloadBindingList{Items: append([]iam.PluginWorkloadBinding{}, f.bindings...)})
	case "POST " + pmIamBindings:
		var create iam.PluginWorkloadBindingCreateBody
		assert.NoError(f.t, json.Unmarshal(raw, &create))
		if create.Issuer != pmTestIssuer {
			iamWrite(f.t, w, http.StatusBadRequest, iam.Error{Error: "invalid_issuer", Message: "issuer is not the configured one"})
			return
		}
		if create.PlatformPrincipal == nil || create.ServiceUserId != nil {
			iamWrite(f.t, w, http.StatusBadRequest, iam.Error{Error: "invalid_request", Message: "exactly one of platform_principal and service_user_id"})
			return
		}
		scope := iam.ServiceUserScopePlatform
		if f.organizationBound {
			scope = iam.ServiceUserScopeOrganization
		}
		binding := iam.PluginWorkloadBinding{Id: uuid.New(), Issuer: create.Issuer, Namespace: create.Namespace, ServiceAccount: create.ServiceAccount, Audience: create.Audience,
			ServiceUserId: uuid.New(), ServiceUserScope: scope, PluginId: create.PluginId, InstallationId: create.InstallationId, CreatedAt: time.Now(), CreatedBy: uuid.New()}
		f.bindings = append(f.bindings, binding)
		iamWrite(f.t, w, http.StatusCreated, binding)
	case "POST " + pmIamPrincipals:
		var create iam.PlatformPrincipalCreateBody
		assert.NoError(f.t, json.Unmarshal(raw, &create))
		if create.ExpiryInDays < 1 || create.ExpiryInDays > 3660 {
			iamWrite(f.t, w, http.StatusBadRequest, iam.Error{Error: "invalid_request", Message: "expiry_in_days must be 1 to 3660"})
			return
		}
		expires := time.Now().AddDate(0, 0, create.ExpiryInDays)
		principal := iam.PlatformPrincipalWithToken{Token: "SU-" + uuid.NewString()}
		principal.Id, principal.DisplayName, principal.Credential, principal.CurrentTokenExpiresAt = uuid.New(), create.DisplayName, iam.PlatformPrincipalCredentialStaticToken, &expires
		iamWrite(f.t, w, http.StatusCreated, principal)
	default:
		f.t.Errorf("unexpected IAM request %s %s", r.Method, r.URL.Path)
	}
}

func pmIdentityArgs(id string, extra ...string) []string {
	return append([]string{pmTestCmd, pmIdentitiesCmd, id,
		"--" + pluginManagerWorkloadIssuerFlag, pmTestIssuer, "--" + pluginManagerWorkloadNamespaceFlag, pmTestNamespace,
		"--" + pluginManagerWorkloadServiceAccountFlag, pmTestAccount, "--" + pluginManagerWorkloadAudienceFlag, pmTestAudience,
		reasonArg, "identify the plugin", outFlag, jsonOutput}, extra...)
}

func TestCreateIdentitiesBindsAPlatformPrincipalAndRecordsTheAdmissionChecker(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	id := pmInstalled(t, fixture, ctx)
	fixture.clearRequests()

	stdout, stderr, err := pmRun(ctx, pmIdentityArgs(id)...)
	require.NoError(t, err)
	require.Equal(t, []string{"GET " + pmIamBindings, "POST " + pmIamBindings, "POST " + pmIamPrincipals}, fixture.iam.requestLines())
	bind := fixture.iam.bodies[1]
	require.Equal(t, map[string]any{
		"issuer": pmTestIssuer, "namespace": pmTestNamespace, "service_account": pmTestAccount, "audience": pmTestAudience,
		"plugin_id": pmTestPlugin, "installation_id": id, "platform_principal": map[string]any{"display_name": pmTestPlugin},
	}, bind, "a new platform principal named after the plugin, never an existing service user")
	require.Equal(t, map[string]any{"display_name": pmTestPlugin + " admission checker", "expiry_in_days": float64(365)}, fixture.iam.bodies[2])
	require.Equal(t, []string{pmTestGetInstallations, "PUT /plugin-installations/" + id + "/identity"}, fixture.requestLines())

	binding := fixture.iam.bindings[0]
	var result struct {
		BindingId        uuid.UUID `json:"binding_id"`
		ServiceUserId    uuid.UUID `json:"service_user_id"`
		AdmissionChecker struct {
			Id    uuid.UUID `json:"id"`
			Token string    `json:"token"`
		} `json:"admission_checker"`
		Installation cp.PluginInstallation `json:"installation"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, binding.Id, result.BindingId)
	require.Equal(t, binding.ServiceUserId, result.ServiceUserId)
	require.True(t, strings.HasPrefix(result.AdmissionChecker.Token, "SU-"), "the one-time token is printed")
	require.Equal(t, binding.ServiceUserId, *result.Installation.ServiceUserId)
	require.Equal(t, result.AdmissionChecker.Id, *result.Installation.AdmissionCheckerId)
	require.Equal(t, binding.ServiceUserId, *fixture.installations[0].ServiceUserId)
	require.Contains(t, stderr, "Admission checker "+result.AdmissionChecker.Id.String()+" created. Its token will not be shown again.")
	require.NotContains(t, stderr, result.AdmissionChecker.Token, "the token goes to the structured output only")

	// install --resume continues with activation and enablement; the recorded identities are skipped.
	fixture.clearRequests()
	_, _, err = pmRun(ctx, pmTestCmd, pmTestInstall, pmTestPackage, pmTestBackendFlag, pmTestBackend, pmTestResumeFlag, "--enable-org", pmTestOrg, reasonArg, "continue")
	require.NoError(t, err)
	require.Equal(t, []string{pmTestGetInstallations, pmTestGetPackages, "POST /plugin-installations/" + id + "/actions/activate", "PUT /plugin-installations/" + id + "/organizations/" + pmTestOrg}, fixture.requestLines())
}

func TestCreateIdentitiesKeepsWhatIAMAlreadyHolds(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	id := pmInstalled(t, fixture, ctx)

	// Recording fails after both principals exist: the token must not be lost and the retry creates nothing twice.
	fixture.identityRefusal = pluginAdmissionCheckerNotPlatformPrincipal
	fixture.iam.clearRequests()
	stdout, _, err := pmRun(ctx, pmIdentityArgs(id)...)
	require.ErrorContains(t, err, "(plugin_admission_checker_not_platform_principal)")
	require.Contains(t, stdout, `"token": "SU-`, "the token was printed before the failure")
	var printed struct {
		AdmissionChecker struct{ Id string } `json:"admission_checker"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &printed))
	checker := printed.AdmissionChecker.Id
	require.ErrorContains(t, err, "run the command again with --"+pluginManagerAdmissionCheckerFlag+" "+checker)
	require.Nil(t, fixture.installations[0].ServiceUserId)

	fixture.identityRefusal = ""
	fixture.iam.clearRequests()
	stdout, _, err = pmRun(ctx, pmIdentityArgs(id, "--"+pluginManagerAdmissionCheckerFlag, checker)...)
	require.NoError(t, err)
	require.Equal(t, []string{"GET " + pmIamBindings}, fixture.iam.requestLines(), "the binding exists and the checker is given: IAM creates nothing")
	require.NotContains(t, stdout, `"token"`)
	require.Len(t, fixture.iam.bindings, 1)
	require.Equal(t, fixture.iam.bindings[0].ServiceUserId, *fixture.installations[0].ServiceUserId)
	require.Equal(t, checker, fixture.installations[0].AdmissionCheckerId.String())
}

func TestCreateIdentitiesExplainsWhatIAMRefuses(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	id := pmInstalled(t, fixture, ctx)

	fixture.iam.forbidden = true
	_, _, err := pmRun(ctx, pmIdentityArgs(id)...)
	require.ErrorContains(t, err, "unexpected status code 403 when trying to list plugin workload bindings")
	fixture.iam.forbidden = false

	_, _, err = pmRun(ctx, pmIdentityArgs(id, "--"+pluginManagerWorkloadIssuerFlag, "https://other.example.test")...)
	require.ErrorContains(t, err, "unexpected status code 400 when trying to create the plugin workload binding")
	require.Contains(t, err.Error(), "issuer is not the configured one")
	require.Empty(t, fixture.iam.bindings)

	_, _, err = pmRun(ctx, pmIdentityArgs(id, "--"+pluginManagerCheckerExpiryFlag, "0")...)
	require.ErrorContains(t, err, "unexpected status code 400 when trying to create the admission checker")
	require.Len(t, fixture.iam.bindings, 1, "the binding stays and the next run reuses it")
	require.Nil(t, fixture.installations[0].ServiceUserId)

	other := uuid.NewString()
	_, _, err = pmRun(ctx, pmIdentityArgs(other)...)
	require.EqualError(t, err, "plugin installation '"+other+"' not found")

	_, _, err = pmRun(ctx, pmIdentityArgs(id, "--"+pluginManagerAdmissionCheckerFlag, "not-a-uuid")...)
	require.ErrorContains(t, err, "invalid --"+pluginManagerAdmissionCheckerFlag)
}

func TestCreateIdentitiesRefusesAnInstallationBoundToAnOrganizationServiceUser(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	id := pmInstalled(t, fixture, ctx)
	fixture.iam.organizationBound = true
	_, _, err := pmRun(ctx, pmIdentityArgs(id)...)
	require.NoError(t, err, "a new binding is always created by the platform")

	_, _, err = pmRun(ctx, pmIdentityArgs(id)...)
	require.ErrorContains(t, err, "IAM binds installation "+id+" to service user "+fixture.iam.bindings[0].ServiceUserId.String()+" of an organization; revoke binding "+fixture.iam.bindings[0].Id.String()+" in IAM")
}

func TestPluginManagerNamesTheNextStepForEveryCoreRefusal(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	fixture.activeOrgs = []string{pmTestOrg, pmTestOrgBeta, pmTestOrgGamma}

	fixture.identityRefusal = pluginServiceUserNotPlatformPrincipal
	_, _, err := pmRun(ctx, pmInstallArgs()...)
	id := fixture.installations[0].InstallationId.String()
	require.ErrorContains(t, err, "(plugin_service_user_not_platform_principal): IAM does not know the identity as a platform principal. "+
		"The installation's service user must be a platform principal, not the service user of an organization. Create the binding with a new platform principal: 'octl plugin-manager create-identities "+id+"'")

	fixture.identityRefusal = pluginAdmissionCheckerNotPlatformPrincipal
	_, _, err = pmRun(ctx, pmInstallArgs(pmTestResumeFlag)...)
	require.ErrorContains(t, err, "The admission checker must be a platform principal with a static token that no plugin workload is bound to. Create it with 'octl plugin-manager create-identities "+id+"'")

	fixture.identityRefusal = ""
	fixture.activationRefusal = pluginInstallationIncomplete
	_, _, err = pmRun(ctx, pmInstallArgs(pmTestResumeFlag)...)
	require.ErrorContains(t, err, "(plugin_installation_incomplete)")
	require.ErrorContains(t, err, "the default approval policies ('octl plugin-manager organization-mode')")
	fixture.activationRefusal = ""
	_, _, err = pmRun(ctx, pmInstallArgs(pmTestResumeFlag)...)
	require.NoError(t, err)

	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "all", reasonArg, "serve everybody")
	require.NoError(t, err)
	_, _, err = pmRun(ctx, pmTestCmd, "remove-org", id, pmTestOrg, reasonArg, "offboard")
	require.ErrorContains(t, err, "(plugin_serves_all_organizations)")
	require.ErrorContains(t, err, "'octl plugin-manager disable-org "+id+" <org-id> --reason <text>'")
	require.ErrorContains(t, err, "'octl plugin-manager organization-mode "+id+" selected --reason <text>'")

	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "selected", reasonArg, "back")
	require.NoError(t, err)
	fixture.orgLimit = 3
	fixture.activeOrgs = append(fixture.activeOrgs, "delta")
	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "all", reasonArg, "one too many")
	require.ErrorContains(t, err, "(plugin_organization_limit)")
	require.ErrorContains(t, err, "Import a grant with a higher limit ('octl plugin-manager import-entitlements'), or disable an organization first: disabled organizations do not count")
}

func TestOrganizationModeServesEveryOrganizationAndKeepsOptOuts(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	_, _, err := pmRun(ctx, pmInstallArgs()...)
	require.NoError(t, err)
	id := fixture.installations[0].InstallationId.String()
	fixture.activeOrgs = []string{pmTestOrg, pmTestOrgBeta, pmTestOrgGamma}
	revision := fixture.revision
	fixture.clearRequests()

	stdout, stderr, err := pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "all", "--"+pluginManagerDefaultTimezoneFlag, "Europe/Berlin",
		"--"+pluginManagerDefaultPolicyFlag, "environment=two-person", reasonArg, "serve every organization", outFlag, jsonOutput)
	require.NoError(t, err)
	require.Equal(t, []string{pmTestGetInstallations, "PUT /plugin-installations/" + id + "/organization-mode"}, fixture.requestLines())
	write := fixture.writes()[0]
	require.Equal(t, map[string]any{
		"expected_revision": float64(revision), "reason": "serve every organization", "mode": "all",
		"default_timezone": "Europe/Berlin", "default_approval_policies": map[string]any{"environment": "two-person"},
	}, write.body)
	require.True(t, pmTestKey.MatchString(write.key))
	require.Contains(t, stderr, "Installation "+id+" of "+pmTestPlugin+": active, availability available, organizations all, revision "+fmt.Sprint(revision+1)+".\n")
	require.Contains(t, stderr, "Organization "+pmTestOrgBeta+": enabled, timezone Europe/Berlin, capabilities pending.\n")
	require.Contains(t, stderr, "Organization "+pmTestOrg+": enabled, timezone Europe/Berlin, capabilities registered.\n", "the existing enablement is untouched")
	var served cp.PluginInstallation
	require.NoError(t, json.Unmarshal([]byte(stdout), &served))
	require.Equal(t, cp.PluginOrganizationModeAll, served.OrganizationMode)
	require.Len(t, served.Organizations, 3)

	// Unset defaults keep the current ones, so a plain mode change never resets them.
	fixture.clearRequests()
	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "selected", reasonArg, "only the enabled ones")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"environment": "two-person"}, fixture.writes()[0].body["default_approval_policies"])
	require.Equal(t, "Europe/Berlin", fixture.writes()[0].body["default_timezone"])
	require.Equal(t, cp.PluginOrganizationModeSelected, fixture.installations[0].OrganizationMode)
	require.Len(t, fixture.installations[0].Organizations, 3, "every enablement stays")

	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "all", "--"+pluginManagerDefaultPolicyFlag, "project=one-person", reasonArg, "new defaults")
	require.NoError(t, err)
	require.Equal(t, "one-person", fixture.installations[0].OrganizationDefaults.ApprovalPolicies["project"])
	require.Equal(t, "Europe/Berlin", fixture.installations[0].OrganizationDefaults.Timezone, "the timezone stays")
	require.Equal(t, map[string]string{"environment": "two-person"}, fixture.installations[0].Organizations[1].ApprovalPolicies, "beta keeps the policies it was created with")
}

func TestOrganizationModeRefusesWhatCoreRefuses(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	id := pmInstalled(t, fixture, ctx)

	_, _, err := pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "all", reasonArg, "too early")
	require.ErrorContains(t, err, "failed to set organizations of installation "+id+" to all (plugin_installation_state): The installation is installed")

	revision := fixture.revision
	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "some", reasonArg, "x")
	require.ErrorContains(t, err, "invalid mode 'some': use all or selected")
	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, "all")
	require.ErrorContains(t, err, `required flag(s) "reason" not set`)
	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, id, reasonArg, "x")
	require.Error(t, err)
	_, _, err = pmRun(ctx, pmTestCmd, pmOrgModeCmd, uuid.NewString(), "all", reasonArg, "x")
	require.ErrorContains(t, err, "not found")
	require.Equal(t, revision, fixture.revision)
}

func TestInstallationStateShowsOrganizationsPendingOrganizationsAndLicence(t *testing.T) {
	limit := int64(2)
	pending := []string{pmTestOrgGamma}
	expires := time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)
	detail := "IAM did not answer"
	grant := &cp.PluginEntitlementGrant{Scope: cp.PluginGrantScopeInstallation, State: cp.PluginEntitlementGrantStateGrace, ExpiresAt: expires, OrganizationLimit: &limit}
	revision := int64(9)
	item := cp.PluginInstallation{InstallationId: uuid.New(), PluginId: pmTestPlugin, State: cp.PluginInstallationStateActive, Availability: cp.PluginAvailabilityStateAvailable,
		OrganizationMode: cp.PluginOrganizationModeAll, Revision: &revision, PendingOrganizations: &pending,
		Entitlement: &cp.PluginInstallationEntitlement{EnabledOrganizations: 2, InstallationGrant: grant},
		Organizations: []cp.PluginEnablement{
			{OrgId: pmTestOrg, State: cp.PluginEnablementStateEnabled, Timezone: "UTC", CapabilityRegistration: cp.PluginCapabilityRegistration{State: cp.PluginCapabilityRegistrationStateRegistered}, EntitlementGrant: grant},
			{OrgId: pmTestOrgBeta, State: cp.PluginEnablementStateDisabled, Timezone: "Europe/Berlin", CapabilityRegistration: cp.PluginCapabilityRegistration{State: cp.PluginCapabilityRegistrationStateFailed, Detail: &detail}},
		}}
	var messages bytes.Buffer
	previous := color.Output
	color.Output = &messages
	t.Cleanup(func() { color.Output = previous })
	printPluginInstallationState(item)
	require.Equal(t, "Installation "+item.InstallationId.String()+" of "+pmTestPlugin+": active, availability available, organizations all, revision 9.\n"+
		"Organization acme: enabled, timezone UTC, capabilities registered, installation licence grant grace.\n"+
		"Organization beta: disabled, timezone Europe/Berlin, capabilities failed (IAM did not answer).\n"+
		"Not enabled yet: gamma. Run 'octl plugin-manager organization-mode "+item.InstallationId.String()+" all' again once the plugin host and the licence limit allow it.\n"+
		"Licence: 2 organizations enabled, installation-wide grant grace until 2027-01-02, limit 2 organizations.\n", messages.String())
}
