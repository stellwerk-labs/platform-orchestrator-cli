package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/fatih/color"
)

const (
	versionCheckGitHubURL     = "https://api.github.com/repos/stellwerk-labs/platform-orchestrator-cli/releases/latest"
	versionCheckTimeout       = 5 * time.Second
	versionCheckInterval      = 24 * time.Hour
	versionCheckLastCheckFile = "last-version-check"
	versionCheckDebugPrefix   = "version checker:"
	versionCheckDocsURL       = "[Documentation url]"

	versionCheckSpacer   = "═════════════════════════════════════════════════════════════════════════════════"
	versionCheckTemplate = `
` + versionCheckSpacer + `
 A new version of octl is available!

 Current version: %s
 Latest version:  %s
%s` + versionCheckSpacer + `
`

	versionCheckWithCommand = `
 To update, run:
   %s
`

	versionCheckWithDocs = `
 See installation docs for update instructions:
   %s
`
)

type GitHubRelease struct {
	Name string `json:"name"`
}

type VersionChecker struct {
	currentVersion string
	configDir      string
	httpClient     *http.Client
	config         *config.Config
}

type VersionCheckResult struct {
	CurrentVersion      string
	LatestVersion       string
	NewVersionAvailable bool
	UpdateInstructions  string
}

func NewVersionChecker(currentVersion string, cfg *config.Config) (*VersionChecker, error) {
	configDir, err := config.Dir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	return &VersionChecker{
		currentVersion: currentVersion,
		configDir:      configDir,
		httpClient: &http.Client{
			Timeout: versionCheckTimeout,
		},
		config: cfg,
	}, nil
}

func (vc *VersionChecker) debugLog(msg string, args ...any) {
	slog.Debug(versionCheckDebugPrefix+" "+msg, args...)
}

func (vc *VersionChecker) skipCheck() bool {
	if vc.config == nil || vc.config.DisableVersionCheck == nil {
		return false
	}
	return *vc.config.DisableVersionCheck
}

func (vc *VersionChecker) Check(ctx context.Context) *VersionCheckResult {
	if vc.skipCheck() {
		vc.debugLog("checking is disabled")
		return nil
	}

	if !vc.shouldCheckVersion() {
		vc.debugLog("skipped - last check was within 24 hours")
		return nil
	}

	if err := vc.updateLastCheckTime(); err != nil {
		vc.debugLog("failed to update last check time", slog.String("error", err.Error()))
		return nil
	}

	latestVersion, err := vc.fetchLatestVersion(ctx)
	if err != nil {
		vc.debugLog("failed to fetch latest version", slog.String("error", err.Error()))
		return nil
	}

	vc.debugLog("successfully fetched latest version", slog.String("version", latestVersion))

	if vc.isCurrentVersionUpToDate(latestVersion) {
		vc.debugLog("current version is up to date")
		return nil
	}

	vc.debugLog("new version available", slog.String("current", vc.currentVersion), slog.String("latest", latestVersion))
	return &VersionCheckResult{
		CurrentVersion:      vc.currentVersion,
		LatestVersion:       latestVersion,
		NewVersionAvailable: true,
		UpdateInstructions:  vc.getUpdateInstructions(),
	}
}

func (vc *VersionChecker) shouldCheckVersion() bool {
	lastCheckFile := filepath.Join(vc.configDir, versionCheckLastCheckFile)
	lastCheckFile = filepath.Clean(lastCheckFile)

	data, err := os.ReadFile(lastCheckFile)
	if err != nil {
		return true
	}

	lastCheck, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}

	return time.Since(lastCheck) >= versionCheckInterval
}

func (vc *VersionChecker) fetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionCheckGitHubURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// https://docs.github.com/en/rest/releases/releases?apiVersion=2022-11-28#get-the-latest-release
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := vc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if release.Name == "" {
		return "", fmt.Errorf("empty version name in response")
	}

	return release.Name, nil
}

func (vc *VersionChecker) updateLastCheckTime() error {
	if err := os.MkdirAll(vc.configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	lastCheckFile := filepath.Join(vc.configDir, versionCheckLastCheckFile)
	timestamp := time.Now().Format(time.RFC3339)

	if err := os.WriteFile(lastCheckFile, []byte(timestamp), 0600); err != nil {
		return fmt.Errorf("failed to write last check file: %w", err)
	}

	return nil
}

func (vc *VersionChecker) isCurrentVersionUpToDate(latestVersion string) bool {
	if latestVersion == "" {
		return true
	}

	current := strings.TrimPrefix(vc.currentVersion, "v")
	latest := strings.TrimPrefix(latestVersion, "v")

	currentFields := strings.Fields(current)
	latestFields := strings.Fields(latest)

	if len(currentFields) == 0 || len(latestFields) == 0 {
		return true
	}

	current = currentFields[0]
	latest = latestFields[0]

	return current == latest || latest == ""
}

func (r *VersionCheckResult) DisplayNotification(stderr io.Writer) {
	var instructions string
	if r.UpdateInstructions != "" {
		instructions = fmt.Sprintf(versionCheckWithCommand, r.UpdateInstructions)
	} else {
		instructions = fmt.Sprintf(versionCheckWithDocs, versionCheckDocsURL)
	}

	message := fmt.Sprintf(versionCheckTemplate, r.CurrentVersion, r.LatestVersion, instructions)

	hiYellow := color.New(color.FgHiYellow)
	_, _ = hiYellow.Fprintf(stderr, "%s", message)
}

func (vc *VersionChecker) getUpdateInstructions() string {
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}

	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realPath = execPath
	}

	lowerPath := strings.ToLower(realPath)

	if strings.Contains(lowerPath, "homebrew") || strings.Contains(lowerPath, "linuxbrew") {
		return "brew upgrade stellwerk-labs/tap/octl"
	}

	if strings.Contains(lowerPath, "scoop") {
		return "scoop update octl"
	}

	return ""
}

// StartVersionCheck starts the version check in the background and returns a channel with the result.
func StartVersionCheck(ctx context.Context, currentVersion string, cfg *config.Config) <-chan *VersionCheckResult {
	resultChan := make(chan *VersionCheckResult, 1)
	go func() {
		checker, err := NewVersionChecker(currentVersion, cfg)
		if err != nil {
			resultChan <- nil
			return
		}
		resultChan <- checker.Check(ctx)
	}()
	return resultChan
}
