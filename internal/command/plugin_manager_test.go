package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
)

const (
	pmTestPlugin           = "progressive-rollouts"
	pmTestPackage          = "1111111111111111111111111111111111111111111111111111111111111111"
	pmTestNextPackage      = "2222222222222222222222222222222222222222222222222222222222222222"
	pmTestBackend          = "https://rollouts.plugins.example.test"
	pmTestOrg              = "acme"
	pmTestServiceUser      = "33333333-3333-4333-8333-333333333333"
	pmTestAdmissionChecker = "44444444-4444-4444-8444-444444444444"
	pmTestCmd              = "plugin-manager"
	pmTestReason           = "roll out the rollouts plugin"
	pmTestTimezone         = "UTC"
	pmTestActivate         = "activate"
	pmTestUnreachable      = "plugin_status_unreachable"
	pmTestBackendFlag      = "--" + pluginManagerBackendURLFlag
	pmTestResumeFlag       = "--" + pluginManagerResumeFlag
	pmTestPolicyScope      = "environment"
	pmTestInstall          = "install"
	pmTestGetInstallations = "GET /plugin-installations"
	pmTestGetPackages      = "GET /plugin-packages"
)

var pmTestKey = regexp.MustCompile(`^octl-[0-9a-f]{40}$`)

// pmRequest is one request the Plugin Manager fake received.
type pmRequest struct {
	method, path, key string
	body              map[string]any
}

func (r pmRequest) String() string { return r.method + " " + r.path }

type pmStoredAnswer struct {
	fingerprint string
	status      int
	body        []byte
}

// pluginManagerServer is a Plugin Manager fake that speaks the vendored
// contract to the real generated client and keeps the server's rules: one
// global revision, expected_revision checks, replay by idempotency key, a
// step whose target state holds changes nothing, at most one installation
// per plugin, and an append-only audit trail.
type pluginManagerServer struct {
	t  *testing.T
	mu sync.Mutex

	revision      int64
	packages      []cp.PluginPackage
	installations []*cp.PluginInstallation
	audit         []cp.PluginAuditEvent
	answers       map[string]pmStoredAnswer

	// activationRefusal, when set, is the diagnostic code activate answers.
	activationRefusal string
	// workUnsettled refuses uninstall with plugin_work_unsettled.
	workUnsettled bool
	// bindingMissing refuses identities with plugin_workload_binding_missing:
	// IAM does not bind the service user to the installation yet.
	bindingMissing bool
	// serviceUserOrg, when set, is the organization every service user
	// belongs to: a new enablement of another organization is refused with
	// plugin_service_user_organization_mismatch.
	serviceUserOrg string
	// hostDisabled answers every request with 404 plugin_host_disabled.
	hostDisabled bool

	requests []pmRequest
	replays  int
}

func newPluginManagerServer(t *testing.T) (*pluginManagerServer, context.Context) {
	t.Helper()
	fixture := &pluginManagerServer{t: t, answers: map[string]pmStoredAnswer{}}
	server := httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(server.Close)
	cpc, err := cp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), ConfigContextKey, config.Config{})
	ctx = context.WithValue(ctx, CpClientContextKey, cpc)
	t.Setenv("PO_API_URL", server.URL)
	cobra.EnableTraverseRunHooks = true
	color.NoColor = true
	return fixture, ctx
}

func (f *pluginManagerServer) writes() []pmRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var writes []pmRequest
	for _, request := range f.requests {
		if request.method != http.MethodGet {
			writes = append(writes, request)
		}
	}
	return writes
}

func (f *pluginManagerServer) clearRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}

func (f *pluginManagerServer) requestLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	lines := make([]string, 0, len(f.requests))
	for _, request := range f.requests {
		lines = append(lines, request.String())
	}
	return lines
}

func (f *pluginManagerServer) find(id string) *cp.PluginInstallation {
	for _, item := range f.installations {
		if item.InstallationId.String() == id {
			return item
		}
	}
	return nil
}

// decode reads a request body; the server goroutine must not stop the test.
func (f *pluginManagerServer) decode(raw []byte, v any) {
	if err := json.Unmarshal(raw, v); err != nil {
		f.t.Errorf("undecodable request %s: %v", raw, err)
	}
}

func pmWriteJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if raw, ok := value.([]byte); ok {
		_, _ = w.Write(raw) //nolint:gosec // the fake answers with JSON it encoded itself
		return
	}
	assert.NoError(t, json.NewEncoder(w).Encode(value))
}

func pmError(code, message string) []byte {
	raw, _ := json.Marshal(cp.Error{Error: code, Message: message})
	return raw
}

func (f *pluginManagerServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := io.ReadAll(r.Body)
	assert.NoError(f.t, err)
	request := pmRequest{method: r.Method, path: r.URL.Path, key: r.Header.Get("Idempotency-Key")}
	if len(raw) > 0 {
		assert.NoError(f.t, json.Unmarshal(raw, &request.body))
	}
	f.requests = append(f.requests, request)
	if f.hostDisabled {
		pmWriteJSON(f.t, w, http.StatusNotFound, pmError("plugin_host_disabled", "The plugin host is disabled on this Control Plane."))
		return
	}
	if r.Method == http.MethodGet {
		f.serveRead(w, r)
		return
	}

	// The command pipeline of every write.
	fingerprint := r.Method + " " + r.URL.Path + "\n" + string(raw)
	if !pmTestKey.MatchString(request.key) {
		f.t.Errorf("%s without a derived idempotency key: %q", request, request.key)
	}
	if stored, ok := f.answers[request.key]; ok {
		if stored.fingerprint != fingerprint {
			pmWriteJSON(f.t, w, http.StatusConflict, pmError("plugin_manager_idempotency_conflict", "The idempotency key was already used for a different Plugin Manager command."))
			return
		}
		f.replays++
		pmWriteJSON(f.t, w, stored.status, stored.body)
		return
	}
	reason, _ := request.body["reason"].(string)
	expected, ok := request.body["expected_revision"].(float64)
	if reason == "" || !ok {
		pmWriteJSON(f.t, w, http.StatusBadRequest, pmError("plugin_manager_request_invalid", "The expected revision and a reason are required."))
		return
	}
	if int64(expected) != f.revision {
		pmWriteJSON(f.t, w, http.StatusConflict, pmError(pluginRevisionConflict, fmt.Sprintf("The Plugin Manager is at revision %d, not at the expected revision %d. Read the current state and retry.", f.revision, int64(expected))))
		return
	}
	status, answer, changed := f.mutate(r, raw)
	if status/100 != 2 {
		pmWriteJSON(f.t, w, status, answer)
		return
	}
	if changed {
		f.revision++
		f.audit = append(f.audit, cp.PluginAuditEvent{Id: int64(len(f.audit) + 1), Revision: f.revision, Action: request.String(), Reason: reason, OccurredAt: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)})
	}
	switch v := answer.(type) {
	case *cp.PluginInstallation:
		v.Revision = &f.revision
		answer = *v
	case *cp.PluginPackage:
		v.Revision = &f.revision
		answer = *v
	case *cp.PluginEntitlementBundleImport:
		v.Revision = f.revision
		answer = *v
	}
	body, err := json.Marshal(answer)
	assert.NoError(f.t, err)
	f.answers[request.key] = pmStoredAnswer{fingerprint: fingerprint, status: status, body: body}
	pmWriteJSON(f.t, w, status, body)
}

func (f *pluginManagerServer) serveRead(w http.ResponseWriter, r *http.Request) {
	switch path := r.URL.Path; {
	case path == "/plugin-packages":
		pmWriteJSON(f.t, w, http.StatusOK, cp.PluginPackagePage{Revision: f.revision, Items: f.packages})
	case path == "/plugin-installations":
		items := []cp.PluginInstallation{}
		for _, item := range f.installations {
			items = append(items, *item)
		}
		pmWriteJSON(f.t, w, http.StatusOK, cp.PluginInstallationPage{Revision: f.revision, Items: items})
	case strings.HasPrefix(path, "/plugin-installations/"):
		item := f.find(strings.TrimPrefix(path, "/plugin-installations/"))
		if item == nil {
			pmWriteJSON(f.t, w, http.StatusNotFound, pmError("plugin_installation_not_found", "The plugin installation does not exist."))
			return
		}
		answer := *item
		answer.Revision = &f.revision
		pmWriteJSON(f.t, w, http.StatusOK, answer)
	case path == "/plugin-audit-events":
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items := []cp.PluginAuditEvent{}
		for _, event := range f.audit {
			if event.Id > after && len(items) < limit {
				items = append(items, event)
			}
		}
		pmWriteJSON(f.t, w, http.StatusOK, cp.PluginAuditEventPage{Items: items})
	default:
		f.t.Errorf("unexpected read %s", path)
	}
}

// mutate applies one write and reports whether it changed anything.
func (f *pluginManagerServer) mutate(r *http.Request, raw []byte) (int, any, bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	conflict := func(code, message string) (int, any, bool) {
		return http.StatusConflict, pmError(code, message), false
	}
	if parts[0] == "plugin-packages" {
		var body cp.PluginPackagePush
		f.decode(raw, &body)
		for i := range f.packages {
			if f.packages[i].ManifestSha256 == body.ManifestSha256 {
				return http.StatusCreated, &f.packages[i], false
			}
		}
		var manifest struct{ PluginId, Version string }
		f.decode(body.Manifest, &manifest)
		f.packages = append(f.packages, cp.PluginPackage{ManifestSha256: body.ManifestSha256, PluginId: manifest.PluginId, Version: manifest.Version, Manifest: body.Manifest})
		return http.StatusCreated, &f.packages[len(f.packages)-1], true
	}
	if parts[0] == "plugin-entitlement-bundles" {
		var body cp.PluginEntitlementBundleImportRequest
		f.decode(raw, &body)
		result := &cp.PluginEntitlementBundleImport{ImportedGrants: strings.Count(string(body.Bundle), "grant"), ImportedRevocations: strings.Count(string(body.Bundle), "revocation")}
		if strings.Contains(string(body.Bundle), "forged") {
			result.Diagnostics = append(result.Diagnostics, struct {
				Code   string `json:"code"`
				Detail string `json:"detail"`
			}{Code: "plugin_entitlement_signature_invalid", Detail: "document 2 does not verify"})
		}
		return http.StatusOK, result, result.ImportedGrants+result.ImportedRevocations > 0
	}
	if len(parts) == 1 {
		var body cp.PluginInstallationCreate
		f.decode(raw, &body)
		var pkg *cp.PluginPackage
		for i := range f.packages {
			if f.packages[i].ManifestSha256 == body.PackageSha256 {
				pkg = &f.packages[i]
			}
		}
		if pkg == nil {
			return http.StatusNotFound, pmError("plugin_package_not_found", "The plugin package does not exist."), false
		}
		for _, item := range f.installations {
			if item.PluginId == pkg.PluginId && item.State != cp.PluginInstallationStatePurged {
				return conflict(pluginInstallationExists, "The plugin already has an installation. Resume, upgrade or reinstall that installation, or purge it first.")
			}
		}
		item := &cp.PluginInstallation{InstallationId: uuid.New(), PluginId: pkg.PluginId, PackageSha256: pkg.ManifestSha256, State: cp.PluginInstallationStateInstalled,
			Availability: cp.PluginAvailabilityStateUnavailable, BackendUrl: body.BackendUrl, UiOrigin: body.UiOrigin, Organizations: []cp.PluginEnablement{}}
		f.installations = append(f.installations, item)
		return http.StatusCreated, item, true
	}
	item := f.find(parts[1])
	if item == nil {
		return http.StatusNotFound, pmError("plugin_installation_not_found", "The plugin installation does not exist."), false
	}
	before, _ := json.Marshal(item)
	state := func(code string) (int, any, bool) {
		return conflict("plugin_installation_state", fmt.Sprintf("The installation is %s.", item.State))
	}
	switch {
	case r.Method == http.MethodDelete && len(parts) == 4 && parts[2] == "organizations":
		item.Organizations = slices.DeleteFunc(item.Organizations, func(enablement cp.PluginEnablement) bool { return enablement.OrgId == parts[3] })
	case r.Method == http.MethodDelete:
		if item.State == cp.PluginInstallationStatePurged {
			return http.StatusOK, item, false
		} else if item.State != cp.PluginInstallationStateUninstalled {
			return state("")
		}
		item.State = cp.PluginInstallationStatePurged
	case len(parts) == 3 && parts[2] == "identity":
		var body cp.PluginInstallationIdentity
		f.decode(raw, &body)
		if f.bindingMissing {
			return conflict(pluginWorkloadBindingMissing, fmt.Sprintf("IAM has no workload binding of service user %s to installation %s of plugin %s; bind the plugin workload first.", body.ServiceUserId, item.InstallationId, item.PluginId))
		}
		if item.State != cp.PluginInstallationStateInstalled {
			return state("")
		}
		item.ServiceUserId, item.AdmissionCheckerId = &body.ServiceUserId, &body.AdmissionCheckerId
	case len(parts) == 3 && parts[2] == "upgrade":
		var body cp.PluginInstallationUpgrade
		f.decode(raw, &body)
		item.TargetPackageSha256 = &body.PackageSha256
	case len(parts) == 4 && parts[2] == "organizations":
		var body cp.PluginEnablementPut
		f.decode(raw, &body)
		enabled := slices.ContainsFunc(item.Organizations, func(enablement cp.PluginEnablement) bool { return enablement.OrgId == parts[3] })
		if f.serviceUserOrg != "" && item.ServiceUserId != nil && !enabled && parts[3] != f.serviceUserOrg {
			return conflict(pluginServiceUserOrgMismatch, fmt.Sprintf("Service user %s does not belong to organization %s. In v1alpha an installation serves only the organization of its service user.", item.ServiceUserId, parts[3]))
		}
		enablement := cp.PluginEnablement{OrgId: parts[3], State: cp.PluginEnablementState(body.State), Timezone: pmTestTimezone, ApprovalPolicies: map[string]string{}}
		if body.Timezone != nil {
			enablement.Timezone = *body.Timezone
		}
		if body.ApprovalPolicies != nil {
			enablement.ApprovalPolicies = *body.ApprovalPolicies
		}
		item.Organizations = []cp.PluginEnablement{enablement}
	case len(parts) == 4 && parts[2] == "actions":
		switch action := parts[3]; {
		case action == "activate" && (item.State == cp.PluginInstallationStateInstalled || item.TargetPackageSha256 != nil):
			if f.activationRefusal != "" {
				return conflict(f.activationRefusal, "The running plugin does not prove the installation.")
			}
			if item.TargetPackageSha256 != nil {
				item.PackageSha256, item.TargetPackageSha256 = *item.TargetPackageSha256, nil
			}
			if item.State == cp.PluginInstallationStateInstalled {
				item.State, item.Availability = cp.PluginInstallationStateActive, cp.PluginAvailabilityStateAvailable
			}
		case action == "disable" && item.State == cp.PluginInstallationStateActive:
			item.State, item.Availability = cp.PluginInstallationStateDisabled, cp.PluginAvailabilityStateNotEnabled
		case action == "enable" && item.State == cp.PluginInstallationStateDisabled:
			item.State, item.Availability = cp.PluginInstallationStateActive, cp.PluginAvailabilityStateAvailable
		case action == "uninstall" && item.State != cp.PluginInstallationStateUninstalled:
			if f.workUnsettled {
				return conflict(pluginWorkUnsettled, "Core still holds operation grants, dispatches or Module reservations of the plugin.")
			}
			item.State, item.Availability, item.Organizations = cp.PluginInstallationStateUninstalled, cp.PluginAvailabilityStateNotInstalled, []cp.PluginEnablement{}
		case action == "reinstall" && item.State == cp.PluginInstallationStateUninstalled:
			item.State, item.Availability = cp.PluginInstallationStateInstalled, cp.PluginAvailabilityStateUnavailable
		}
	default:
		f.t.Errorf("unexpected write %s %s", r.Method, r.URL.Path)
	}
	after, _ := json.Marshal(item)
	return http.StatusOK, item, string(before) != string(after)
}

func pmRun(ctx context.Context, args ...string) (string, string, error) {
	return executeAndResetCommand(ctx, RootCmd, args)
}

func pmRegister(t *testing.T, ctx context.Context, digest, version string) {
	t.Helper()
	manifest := writeTestFile(t, "registration.yaml", "pluginId: "+pmTestPlugin+"\nversion: "+version+"\n")
	_, stderr, err := pmRun(ctx, pmTestCmd, "register-package", manifest, "--manifest-sha256", digest, reasonArg, "register "+version)
	require.NoError(t, err)
	require.Contains(t, stderr, "Package "+pmTestPlugin+" "+version+" ("+digest+") registered, revision ")
}

// pmInstalled registers the package and leaves its installation in state
// installed: the plugin runtime is not running yet.
func pmInstalled(t *testing.T, fixture *pluginManagerServer, ctx context.Context) string {
	t.Helper()
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	fixture.activationRefusal = pmTestUnreachable
	_, _, err := pmRun(ctx, pmTestCmd, pmTestInstall, pmTestPackage, pmTestBackendFlag, pmTestBackend, reasonArg, "install")
	require.ErrorContains(t, err, "(plugin_status_unreachable)")
	fixture.activationRefusal = ""
	require.Equal(t, cp.PluginInstallationStateInstalled, fixture.installations[0].State)
	return fixture.installations[0].InstallationId.String()
}

func pmInstallArgs(extra ...string) []string {
	return append([]string{pmTestCmd, pmTestInstall, pmTestPackage, pmTestBackendFlag, pmTestBackend,
		"--service-user", pmTestServiceUser, "--admission-checker", pmTestAdmissionChecker,
		"--enable-org", pmTestOrg, "--timezone", "Europe/Berlin", reasonArg, pmTestReason}, extra...)
}

func TestPluginManagerInstallFlow(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	require.Equal(t, int64(1), fixture.revision)
	fixture.clearRequests()

	stdout, stderr, err := pmRun(ctx, append(pmInstallArgs(), outFlag, jsonOutput)...)
	require.NoError(t, err)
	require.Equal(t, []string{
		pmTestGetInstallations,
		"POST /plugin-installations",
		"PUT /plugin-installations/" + fixture.installations[0].InstallationId.String() + "/identity",
		"POST /plugin-installations/" + fixture.installations[0].InstallationId.String() + "/actions/activate",
		"PUT /plugin-installations/" + fixture.installations[0].InstallationId.String() + "/organizations/" + pmTestOrg,
	}, fixture.requestLines())
	keys := map[string]bool{}
	for i, write := range fixture.writes() {
		require.Equal(t, 1+i, int(write.body["expected_revision"].(float64)), "%s chains the revision of the previous answer", write)
		require.Equal(t, pmTestReason, write.body["reason"])
		keys[write.key] = true
	}
	require.Len(t, keys, 4, "every command has its own key")
	require.Equal(t, "Europe/Berlin", fixture.writes()[3].body["timezone"])

	id := fixture.installations[0].InstallationId.String()
	for _, line := range []string{
		"Installation " + id + " of " + pmTestPlugin + ": installed, availability unavailable, revision 2.",
		"Installation " + id + " of " + pmTestPlugin + ": installed, availability unavailable, revision 3.",
		"Installation " + id + " of " + pmTestPlugin + ": active, availability available, revision 4.",
		"Installation " + id + " of " + pmTestPlugin + ": active, availability available, revision 5.",
	} {
		require.Contains(t, stderr, line+"\n")
	}
	var installed cp.PluginInstallation
	require.NoError(t, json.Unmarshal([]byte(stdout), &installed))
	require.Equal(t, cp.PluginInstallationStateActive, installed.State)
	require.Equal(t, int64(5), *installed.Revision)
	require.Equal(t, pmTestServiceUser, installed.ServiceUserId.String())
	require.Equal(t, []cp.PluginEnablement{{OrgId: pmTestOrg, State: cp.PluginEnablementStateEnabled, Timezone: "Europe/Berlin", ApprovalPolicies: map[string]string{}}}, installed.Organizations)
}

func TestPluginManagerInstallResumesFromTheRecordedState(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	fixture.activationRefusal = pmTestUnreachable

	_, _, err := pmRun(ctx, pmInstallArgs()...)
	id := fixture.installations[0].InstallationId.String()
	require.EqualError(t, err, "installation "+id+" is recorded; continue with --resume: failed to activate installation "+id+
		" (plugin_status_unreachable): The running plugin does not prove the installation. Core could not read the plugin status from /.well-known/stellwerk/plugin-status on the backend URL. "+
		"Check that the plugin runtime is running and that the Control Plane reaches the backend URL and trusts its certificate, then run the command again: a refused activation changed nothing")
	require.Equal(t, cp.PluginInstallationStateInstalled, fixture.installations[0].State)
	require.NotNil(t, fixture.installations[0].ServiceUserId, "the identity step before the failure stays done")
	revision := fixture.revision

	fixture.activationRefusal = ""
	_, _, err = pmRun(ctx, pmInstallArgs()...)
	require.ErrorContains(t, err, "(plugin_installation_exists): The plugin already has an installation. Resume, upgrade or reinstall that installation, or purge it first. Continue the existing installation with the same command and --resume")
	require.Equal(t, revision, fixture.revision)

	fixture.clearRequests()
	_, stderr, err := pmRun(ctx, pmInstallArgs(pmTestResumeFlag)...)
	require.NoError(t, err)
	require.Contains(t, stderr, "Resuming installation "+id+" of "+pmTestPlugin+": installed at revision "+strconv.FormatInt(revision, 10)+".")
	require.Equal(t, []string{
		pmTestGetInstallations,
		pmTestGetPackages,
		"POST /plugin-installations/" + id + "/actions/activate",
		"PUT /plugin-installations/" + id + "/organizations/" + pmTestOrg,
	}, fixture.requestLines(), "the recorded identity is not sent again")
	require.Len(t, fixture.installations, 1)
	require.Equal(t, cp.PluginInstallationStateActive, fixture.installations[0].State)

	fixture.clearRequests()
	_, stderr, err = pmRun(ctx, pmInstallArgs(pmTestResumeFlag)...)
	require.NoError(t, err)
	require.Empty(t, fixture.writes(), "a completed install changes nothing")
	require.Contains(t, stderr, "Resuming installation "+id+" of "+pmTestPlugin+": active at revision ")

	t.Run("nothing recorded installs", func(t *testing.T) {
		fixture, ctx := newPluginManagerServer(t)
		pmRegister(t, ctx, pmTestPackage, "1.0.0")
		_, stderr, err := pmRun(ctx, pmInstallArgs(pmTestResumeFlag)...)
		require.NoError(t, err)
		require.Contains(t, stderr, "No installation of "+pmTestPlugin+" recorded; installing.")
		require.Equal(t, cp.PluginInstallationStateActive, fixture.installations[0].State)
	})
	t.Run("refuses another installation", func(t *testing.T) {
		for name, tc := range map[string]struct {
			prepare func(f *pluginManagerServer)
			args    []string
			err     string
		}{
			"other package":     {args: []string{pmTestCmd, pmTestInstall, pmTestNextPackage, pmTestBackendFlag, pmTestBackend, pmTestResumeFlag, reasonArg, "x"}, err: "serves package " + pmTestPackage + ", not " + pmTestNextPackage + "; request the new package with 'octl plugin-manager upgrade"},
			"other backend":     {args: []string{pmTestCmd, pmTestInstall, pmTestPackage, pmTestBackendFlag, "https://elsewhere.example.test", pmTestResumeFlag, reasonArg, "x"}, err: "uses backend URL " + pmTestBackend + ", not https://elsewhere.example.test"},
			"disabled":          {prepare: func(f *pluginManagerServer) { f.installations[0].State = cp.PluginInstallationStateDisabled }, args: pmInstallArgs(pmTestResumeFlag), err: "was disabled by an operator; enable it with 'octl plugin-manager enable "},
			"uninstalled":       {prepare: func(f *pluginManagerServer) { f.installations[0].State = cp.PluginInstallationStateUninstalled }, args: pmInstallArgs(pmTestResumeFlag), err: "is uninstalled; reinstall it with 'octl plugin-manager reinstall "},
			"not registered":    {args: []string{pmTestCmd, pmTestInstall, strings.Repeat("9", 64), pmTestBackendFlag, pmTestBackend, pmTestResumeFlag, reasonArg, "x"}, err: "is not registered; register it with 'octl plugin-manager register-package'"},
			"settings need org": {args: []string{pmTestCmd, pmTestInstall, pmTestPackage, pmTestBackendFlag, pmTestBackend, "--timezone", pmTestTimezone, reasonArg, "x"}, err: "--timezone and --approval-policy need --enable-org"},
		} {
			fixture, ctx := newPluginManagerServer(t)
			pmRegister(t, ctx, pmTestPackage, "1.0.0")
			pmRegister(t, ctx, pmTestNextPackage, "1.1.0")
			_, _, err := pmRun(ctx, pmInstallArgs()...)
			require.NoError(t, err)
			if tc.prepare != nil {
				tc.prepare(fixture)
			}
			writes := len(fixture.writes())
			_, _, err = pmRun(ctx, tc.args...)
			require.ErrorContains(t, err, tc.err, name)
			require.Len(t, fixture.writes(), writes, name)
		}
	})
}

func TestPluginManagerInstallResumesInTheOrderOfTheServiceIdentity(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	fixture.bindingMissing = true
	fixture.clearRequests()

	_, _, err := pmRun(ctx, pmInstallArgs()...)
	id := fixture.installations[0].InstallationId.String()
	require.EqualError(t, err, "installation "+id+" is recorded; continue with --resume: failed to set the identity of installation "+id+" (plugin_workload_binding_missing): "+
		"IAM has no workload binding of service user "+pmTestServiceUser+" to installation "+id+" of plugin "+pmTestPlugin+"; bind the plugin workload first. "+
		"The binding names the installation UUID, so it can only exist after the installation: create it for installation "+id+" in IAM "+
		"(POST /plugin-workload-bindings, platform permission plugin_workload_binding_write), then run the command again")
	require.Equal(t, []string{pmTestGetInstallations, "POST /plugin-installations", "PUT /plugin-installations/" + id + "/identity"}, fixture.requestLines(), "nothing after the refused identity")
	require.Equal(t, cp.PluginInstallationStateInstalled, fixture.installations[0].State)
	require.Nil(t, fixture.installations[0].ServiceUserId)

	// IAM binds the service user, which belongs to another organization.
	fixture.bindingMissing, fixture.serviceUserOrg = false, "service-user-org"
	fixture.clearRequests()
	_, _, err = pmRun(ctx, pmInstallArgs(pmTestResumeFlag)...)
	require.EqualError(t, err, "installation "+id+" is recorded; continue with --resume: failed to set organization "+pmTestOrg+" of installation "+id+" to enabled (plugin_service_user_organization_mismatch): "+
		"Service user "+pmTestServiceUser+" does not belong to organization "+pmTestOrg+". In v1alpha an installation serves only the organization of its service user. "+
		"Enable the organization of the installation's service user instead, or give the installation a service user of this organization: "+
		"create the service user there, bind it to installation "+id+" in IAM, record it with 'octl plugin-manager set-identity' "+
		"(only in state installed: uninstall and reinstall an active installation first), activate it, then enable the organization")
	require.Equal(t, []string{
		pmTestGetInstallations,
		pmTestGetPackages,
		"PUT /plugin-installations/" + id + "/identity",
		"POST /plugin-installations/" + id + "/actions/activate",
		"PUT /plugin-installations/" + id + "/organizations/" + pmTestOrg,
	}, fixture.requestLines(), "identity, activation, then the organization")
	require.Equal(t, cp.PluginInstallationStateActive, fixture.installations[0].State)
	require.Empty(t, fixture.installations[0].Organizations)

	fixture.clearRequests()
	_, _, err = pmRun(ctx, pmInstallArgs(pmTestResumeFlag, "--"+pluginManagerEnableOrgFlag, fixture.serviceUserOrg)...)
	require.NoError(t, err)
	require.Equal(t, []string{pmTestGetInstallations, pmTestGetPackages, "PUT /plugin-installations/" + id + "/organizations/" + fixture.serviceUserOrg}, fixture.requestLines(),
		"the recorded identity and activation are skipped")
	require.Equal(t, fixture.serviceUserOrg, fixture.installations[0].Organizations[0].OrgId)
}

func TestPluginManagerActivationDiagnosticsAreActionable(t *testing.T) {
	for code, hint := range map[string]string{
		pmTestUnreachable:                      "Check that the plugin runtime is running and that the Control Plane reaches the backend URL and trusts its certificate",
		"plugin_status_invalid":                "Check that the plugin runtime supports this Core's plugin contract and read its logs",
		"plugin_installation_mismatch":         "Configure the plugin runtime with installation %s and let it roll out",
		"plugin_package_digest_mismatch":       "Deploy the package Core expects (the pending upgrade's, if any) to the plugin runtime and wait for its rollout",
		"plugin_migration_generation_mismatch": "Let the plugin runtime finish its migrations",
	} {
		fixture, ctx := newPluginManagerServer(t)
		id := pmInstalled(t, fixture, ctx)
		fixture.activationRefusal = code
		revision := fixture.revision

		_, _, err := pmRun(ctx, pmTestCmd, pmTestActivate, id, reasonArg, "serve it")
		if strings.Contains(hint, "%s") {
			hint = fmt.Sprintf(hint, id)
		}
		require.ErrorContains(t, err, "failed to activate installation "+id+" ("+code+"): ", code)
		require.ErrorContains(t, err, hint+", then run the command again: a refused activation changed nothing", code)
		require.Equal(t, revision, fixture.revision, code)
	}
}

func TestPluginManagerLifecycle(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	pmRegister(t, ctx, pmTestNextPackage, "1.1.0")
	_, _, err := pmRun(ctx, append(pmInstallArgs(), "--approval-policy", "environment=two-person")...)
	require.NoError(t, err)
	id := fixture.installations[0].InstallationId.String()
	step := func(args ...string) string {
		t.Helper()
		_, stderr, err := pmRun(ctx, append(append([]string{pmTestCmd}, args...), reasonArg, "lifecycle")...)
		require.NoError(t, err)
		return stderr
	}

	stderr := step("upgrade", id, pmTestNextPackage)
	require.Contains(t, stderr, "active, availability available, upgrade to package "+pmTestNextPackage+" pending, revision ")
	require.Contains(t, stderr, "Core keeps serving package "+pmTestPackage+". Roll the plugin runtime to package "+pmTestNextPackage+", then switch with 'octl plugin-manager activate "+id+" --reason <text>'.")
	step(pmTestActivate, id)
	require.Equal(t, pmTestNextPackage, fixture.installations[0].PackageSha256)

	step("disable-org", id, pmTestOrg)
	put := fixture.writes()[len(fixture.writes())-1]
	require.Equal(t, "disabled", put.body["state"])
	require.Equal(t, "Europe/Berlin", put.body["timezone"], "a PUT keeps the organization's settings")
	require.Equal(t, map[string]any{pmTestPolicyScope: "two-person"}, put.body["approval_policies"])
	step("enable-org", id, pmTestOrg, "--timezone", pmTestTimezone)
	require.Equal(t, cp.PluginEnablement{OrgId: pmTestOrg, State: cp.PluginEnablementStateEnabled, Timezone: pmTestTimezone, ApprovalPolicies: map[string]string{pmTestPolicyScope: "two-person"}}, fixture.installations[0].Organizations[0])

	fixture.workUnsettled = true
	_, _, err = pmRun(ctx, pmTestCmd, "uninstall", id, reasonArg, "retire")
	require.EqualError(t, err, "failed to uninstall installation "+id+" (plugin_work_unsettled): Core still holds operation grants, dispatches or Module reservations of the plugin. "+
		"Core still holds work of this plugin (operation grants, dispatches or Module reservations) and uninstall has no force option. "+
		"Stop new work with 'octl plugin-manager disable "+id+" --reason <text>', let the running work settle, then uninstall again")
	require.Contains(t, step("disable", id), ": disabled, availability not_enabled, revision ")
	fixture.workUnsettled = false
	require.Contains(t, step("uninstall", id), ": uninstalled, availability not_installed, revision ")
	require.Contains(t, step("reinstall", id), ": installed, availability unavailable, revision ")
	step("uninstall", id)
	stderr = step("purge", id)
	require.Contains(t, stderr, ": purged, availability not_installed, revision ")
	require.Contains(t, stderr, "Core does not own plugin data: delete it through the plugin runtime.")

	_, _, err = pmRun(ctx, pmTestCmd, "enable", id, reasonArg, "too late")
	require.NoError(t, err, "a step whose target state cannot change is a no-op on the server")
	_, _, err = pmRun(ctx, pmTestCmd, "set-identity", id, "--service-user", pmTestServiceUser, "--admission-checker", pmTestAdmissionChecker, reasonArg, "x")
	require.ErrorContains(t, err, "(plugin_installation_state): The installation is purged")
}

func TestPluginManagerRemovesAnOrganizationEnablement(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	_, _, err := pmRun(ctx, pmInstallArgs()...)
	require.NoError(t, err)
	id := fixture.installations[0].InstallationId.String()
	revision := fixture.revision
	audit := len(fixture.audit)
	remove := []string{pmTestCmd, "remove-org", id, pmTestOrg, reasonArg, "offboard the organization", "--expected-revision", strconv.FormatInt(revision, 10), outFlag, jsonOutput}
	fixture.clearRequests()

	stdout, stderr, err := pmRun(ctx, remove...)
	require.NoError(t, err)
	require.Equal(t, []string{pmTestGetInstallations, "DELETE /plugin-installations/" + id + "/organizations/" + pmTestOrg}, fixture.requestLines())
	require.Equal(t, map[string]any{"expected_revision": float64(revision), "reason": "offboard the organization"}, fixture.writes()[0].body, "the command carries only the revision and the reason")
	require.Empty(t, fixture.installations[0].Organizations)
	require.Equal(t, revision+1, fixture.revision)
	require.Len(t, fixture.audit, audit+1)
	require.Equal(t, "offboard the organization", fixture.audit[audit].Reason)
	require.Contains(t, stderr, "Installation "+id+" of "+pmTestPlugin+": active, availability available, revision "+strconv.FormatInt(revision+1, 10)+".\n")
	require.NotContains(t, stderr, "nothing changed")
	var removed cp.PluginInstallation
	require.NoError(t, json.Unmarshal([]byte(stdout), &removed))
	require.Empty(t, removed.Organizations)

	again, stderr, err := pmRun(ctx, remove...)
	require.NoError(t, err, "the identical command after a lost answer replays it")
	require.Equal(t, 1, fixture.replays)
	require.Equal(t, stdout, again)
	require.NotContains(t, stderr, "nothing changed", "the replayed answer is the change")

	_, stderr, err = pmRun(ctx, pmTestCmd, "remove-org", id, pmTestOrg, reasonArg, "offboard it again")
	require.NoError(t, err, "removing an absent enablement is no error")
	require.Contains(t, stderr, "Installation "+id+" had no enablement for organization "+pmTestOrg+": nothing changed.\n")
	require.Equal(t, revision+1, fixture.revision)
	require.Len(t, fixture.audit, audit+1)

	_, _, err = pmRun(ctx, pmTestCmd, "remove-org", id, pmTestOrg, reasonArg, "stale", "--expected-revision", strconv.FormatInt(revision, 10))
	require.ErrorContains(t, err, "failed to remove organization "+pmTestOrg+" from installation "+id+" (plugin_manager_revision_conflict)")

	unknown := uuid.NewString()
	_, _, err = pmRun(ctx, pmTestCmd, "remove-org", unknown, pmTestOrg, reasonArg, "x")
	require.EqualError(t, err, "failed to remove organization "+pmTestOrg+" from installation "+unknown+" (plugin_installation_not_found): The plugin installation does not exist")
	require.Equal(t, revision+1, fixture.revision)
}

func TestPluginManagerRevisionAndIdempotency(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	id := pmInstalled(t, fixture, ctx)
	revision := strconv.FormatInt(fixture.revision, 10)

	_, _, err := pmRun(ctx, pmTestCmd, pmTestActivate, id, reasonArg, "serve it", "--expected-revision", "0")
	require.EqualError(t, err, "failed to activate installation "+id+" (plugin_manager_revision_conflict): The Plugin Manager is at revision "+revision+", not at the expected revision 0. Read the current state and retry. "+
		"Review the current state with 'octl get plugin-installations' and run the command again; without --expected-revision it uses the current revision")

	activate := []string{pmTestCmd, pmTestActivate, id, reasonArg, "serve it", "--expected-revision", revision, outFlag, jsonOutput}
	first, _, err := pmRun(ctx, activate...)
	require.NoError(t, err)
	again, stderr, err := pmRun(ctx, activate...)
	require.NoError(t, err, "the identical command after a lost answer replays it")
	require.Equal(t, 1, fixture.replays)
	require.Equal(t, first, again)
	require.Contains(t, stderr, ": active, availability available, revision ")
	writes := fixture.writes()
	require.Equal(t, writes[len(writes)-1].key, writes[len(writes)-2].key)

	_, _, err = pmRun(ctx, pmTestCmd, pmTestActivate, id, reasonArg, "another reason", "--expected-revision", revision)
	require.ErrorContains(t, err, "(plugin_manager_revision_conflict)", "another command gets another key")
	require.Equal(t, 1, fixture.replays)
}

func TestPluginManagerChangesRequireAReason(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	id := uuid.NewString()
	for _, args := range [][]string{
		{"register-package", "manifest.json", "--manifest-sha256", pmTestPackage},
		{"install", pmTestPackage, pmTestBackendFlag, pmTestBackend},
		{"set-identity", id, "--service-user", pmTestServiceUser, "--admission-checker", pmTestAdmissionChecker},
		{"activate", id}, {"disable", id}, {"enable", id}, {"uninstall", id}, {"reinstall", id}, {"purge", id},
		{"upgrade", id, pmTestNextPackage}, {"enable-org", id, pmTestOrg}, {"disable-org", id, pmTestOrg}, {"remove-org", id, pmTestOrg},
		{"import-entitlements", "bundle.jws"},
	} {
		_, _, err := pmRun(ctx, append([]string{pmTestCmd}, args...)...)
		require.ErrorContains(t, err, `required flag(s) "reason" not set`, args[0])
	}
	require.Empty(t, fixture.requestLines())
	_, _, err := pmRun(ctx, pmTestCmd, "set-identity", id, "--service-user", pmTestServiceUser, reasonArg, "x")
	require.ErrorContains(t, err, "if any flags in the group [service-user admission-checker] are set they must all be set")
}

func TestPluginManagerEntitlementsAndAuditTrail(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	bundle := writeTestFile(t, "bundle.jws", "grant grant revocation forged")
	stdout, stderr, err := pmRun(ctx, pmTestCmd, "import-entitlements", bundle, reasonArg, "licence renewal", outFlag, jsonOutput)
	require.NoError(t, err)
	require.Contains(t, stderr, "Imported 2 grants and 1 revocations, revision 2.\nSkipped a document (plugin_entitlement_signature_invalid): document 2 does not verify\n")
	require.Contains(t, stdout, `"imported_grants": 2`)
	require.Equal(t, "grant grant revocation forged", func() string {
		var body cp.PluginEntitlementBundleImportRequest
		raw, _ := json.Marshal(fixture.writes()[1].body)
		require.NoError(t, json.Unmarshal(raw, &body))
		return string(body.Bundle)
	}(), "the bundle is sent byte-identically")

	stdout, _, err = pmRun(ctx, testGetCmd, "plugin-audit-events", outFlag, jsonOutput)
	require.NoError(t, err)
	var events []cp.PluginAuditEvent
	require.NoError(t, json.Unmarshal([]byte(stdout), &events))
	require.Len(t, events, 2)
	require.Equal(t, "licence renewal", events[1].Reason)

	stdout, stderr, err = pmRun(ctx, testGetCmd, "plugin-audit-events", "--limit", "1")
	require.NoError(t, err)
	require.Contains(t, stdout, "register 1.0.0")
	require.NotContains(t, stdout, "licence renewal")
	require.Contains(t, stderr, "More events may follow: --after 1.")
	stdout, _, err = pmRun(ctx, testGetCmd, "plugin-audit-events", "--after", "1")
	require.NoError(t, err)
	require.Contains(t, stdout, "licence renewal")
}

func TestPluginManagerReads(t *testing.T) {
	fixture, ctx := newPluginManagerServer(t)
	pmRegister(t, ctx, pmTestPackage, "1.0.0")
	_, _, err := pmRun(ctx, pmInstallArgs()...)
	require.NoError(t, err)
	id := fixture.installations[0].InstallationId.String()

	stdout, stderr, err := pmRun(ctx, testGetCmd, "plugin-packages")
	require.NoError(t, err)
	require.Contains(t, stderr, "Plugin Manager revision 5.")
	require.Regexp(t, `PluginId\s+Version\s+ManifestSha256\s+CreatedAt\s*\n`+pmTestPlugin+`\s+1\.0\.0\s+`+pmTestPackage, stdout)

	stdout, _, err = pmRun(ctx, testGetCmd, "plugin-installations")
	require.NoError(t, err)
	require.Regexp(t, id+`\s+`+pmTestPlugin+`\s+active\s+available\s+`+pmTestPackage+`\s+-`, stdout)

	fixture.installations[0].Diagnostic = &cp.PluginInstallationDiagnostic{Code: cp.PluginDiagnosticPackageDigestMismatch, Message: "the plugin runs another package"}
	stdout, stderr, err = pmRun(ctx, testGetCmd, "plugin-installation", id, outFlag, jsonOutput)
	require.NoError(t, err)
	require.Contains(t, stderr, "Installation "+id+" of "+pmTestPlugin+": active, availability available, revision 5.\nDiagnostic plugin_package_digest_mismatch: the plugin runs another package\n")
	require.Contains(t, stdout, `"organizations": [`)

	fixture.hostDisabled = true
	_, _, err = pmRun(ctx, testGetCmd, "plugin-installations")
	require.EqualError(t, err, "failed to list plugin installations (plugin_host_disabled): The plugin host is disabled on this Control Plane. Enable the plugin host of this Control Plane first")
}
