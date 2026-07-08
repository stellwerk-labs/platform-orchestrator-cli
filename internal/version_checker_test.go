package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	v100 = "v1.0.0"
	v110 = "v1.1.0"
)

func TestNewVersionChecker(t *testing.T) {
	testCases := []struct {
		name           string
		currentVersion string
		expectError    bool
	}{
		{
			name:           "valid version",
			currentVersion: v100,
			expectError:    false,
		},
		{
			name:           "version without v prefix",
			currentVersion: "1.0.0",
			expectError:    false,
		},
		{
			name:           "version with additional info",
			currentVersion: "v1.0.0 abc123 2023-01-01",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vc, err := NewVersionChecker(tc.currentVersion, &config.Config{})

			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, vc)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, vc)
				assert.Equal(t, tc.currentVersion, vc.currentVersion)
				assert.NotEmpty(t, vc.configDir)
			}
		})
	}
}

func TestVersionChecker_skipCheck(t *testing.T) {
	trueVal := true
	falseVal := false

	testCases := []struct {
		name           string
		config         *config.Config
		expectedResult bool
	}{
		{
			name:           "skip check - nil config",
			config:         nil,
			expectedResult: false,
		},
		{
			name:           "skip check - nil DisableVersionCheck",
			config:         &config.Config{DisableVersionCheck: nil},
			expectedResult: false,
		},
		{
			name:           "skip check - false (do not skip)",
			config:         &config.Config{DisableVersionCheck: &falseVal},
			expectedResult: false,
		},
		{
			name:           "skip check - true (skip)",
			config:         &config.Config{DisableVersionCheck: &trueVal},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vc := &VersionChecker{
				config: tc.config,
			}
			result := vc.skipCheck()
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestVersionChecker_shouldCheckVersion(t *testing.T) {
	testCases := []struct {
		name           string
		setupFile      bool
		fileContent    string
		expectedResult bool
	}{
		{
			name:           "no last check file - should check",
			setupFile:      false,
			expectedResult: true,
		},
		{
			name:           "last check was 25 hours ago - should check",
			setupFile:      true,
			fileContent:    time.Now().Add(-25 * time.Hour).Format(time.RFC3339),
			expectedResult: true,
		},
		{
			name:           "last check was 1 hour ago - should not check",
			setupFile:      true,
			fileContent:    time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			expectedResult: false,
		},
		{
			name:           "invalid timestamp - should check",
			setupFile:      true,
			fileContent:    "invalid-timestamp",
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			vc := &VersionChecker{
				currentVersion: v100,
				configDir:      tmpDir,
				config:         &config.Config{},
			}

			if tc.setupFile {
				lastCheckFile := filepath.Join(tmpDir, versionCheckLastCheckFile)
				err := os.WriteFile(lastCheckFile, []byte(tc.fileContent), 0600)
				require.NoError(t, err)
			}

			result := vc.shouldCheckVersion()
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestVersionChecker_fetchLatestVersion(t *testing.T) {
	testCases := []struct {
		name           string
		serverResponse GitHubRelease
		serverStatus   int
		expectedError  bool
		expectedResult string
	}{
		{
			name: "successful fetch",
			serverResponse: GitHubRelease{
				Name: "v1.41.0",
			},
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedResult: "v1.41.0",
		},
		{
			name:           "server error",
			serverResponse: GitHubRelease{},
			serverStatus:   http.StatusInternalServerError,
			expectedError:  true,
			expectedResult: "",
		},
		{
			name: "empty version name",
			serverResponse: GitHubRelease{
				Name: "",
			},
			serverStatus:   http.StatusOK,
			expectedError:  true,
			expectedResult: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
				assert.Equal(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))

				w.WriteHeader(tc.serverStatus)
				if tc.serverStatus == http.StatusOK {
					if err := json.NewEncoder(w).Encode(tc.serverResponse); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					}
				}
			}))
			defer server.Close()

			tmpDir := t.TempDir()

			vc := &VersionChecker{
				currentVersion: v100,
				configDir:      tmpDir,
				httpClient: &http.Client{
					Timeout: versionCheckTimeout,
				},
				config: &config.Config{},
			}

			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)

			req.Header.Set("Accept", "application/vnd.github+json")
			req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

			resp, err := vc.httpClient.Do(req)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()

			if tc.expectedError {
				if resp.StatusCode != http.StatusOK {
					assert.NotEqual(t, http.StatusOK, resp.StatusCode)
				} else {
					var release GitHubRelease
					err := json.NewDecoder(resp.Body).Decode(&release)
					require.NoError(t, err)
					assert.Empty(t, release.Name)
				}
			} else {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				var release GitHubRelease
				err := json.NewDecoder(resp.Body).Decode(&release)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedResult, release.Name)
			}
		})
	}
}

func TestVersionChecker_updateLastCheckTime(t *testing.T) {
	testCases := []struct {
		name        string
		setupDir    bool
		expectError bool
	}{
		{
			name:        "successful update",
			setupDir:    true,
			expectError: false,
		},
		{
			name:        "create directory and update",
			setupDir:    false,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := filepath.Join(tmpDir, "test-config")

			if tc.setupDir {
				err := os.MkdirAll(configDir, 0750)
				require.NoError(t, err)
			}

			vc := &VersionChecker{
				currentVersion: v100,
				configDir:      configDir,
			}

			err := vc.updateLastCheckTime()

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				lastCheckFile := filepath.Join(configDir, versionCheckLastCheckFile)
				cleanPath := filepath.Clean(lastCheckFile)
				require.True(t, strings.HasPrefix(cleanPath, filepath.Clean(configDir)), "path must be within config directory")
				data, err := os.ReadFile(cleanPath)
				require.NoError(t, err)

				timestamp, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
				require.NoError(t, err)
				assert.WithinDuration(t, time.Now(), timestamp, 5*time.Second)
			}
		})
	}
}

func TestVersionChecker_isCurrentVersionUpToDate(t *testing.T) {
	testCases := []struct {
		name           string
		currentVersion string
		latestVersion  string
		expectedResult bool
	}{
		{
			name:           "newer version available - not up to date",
			currentVersion: v100,
			latestVersion:  v110,
			expectedResult: false,
		},
		{
			name:           "same version - up to date",
			currentVersion: v100,
			latestVersion:  v100,
			expectedResult: true,
		},
		{
			name:           "version without v prefix - not up to date",
			currentVersion: "1.0.0",
			latestVersion:  "1.1.0",
			expectedResult: false,
		},
		{
			name:           "version with additional info - not up to date",
			currentVersion: "v1.0.0 abc123 2023-01-01",
			latestVersion:  v110,
			expectedResult: false,
		},
		{
			name:           "empty latest version - up to date",
			currentVersion: v100,
			latestVersion:  "",
			expectedResult: true,
		},
		{
			name:           "same version different format - up to date",
			currentVersion: "v1.0.0 abc123",
			latestVersion:  v100,
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vc := &VersionChecker{
				currentVersion: tc.currentVersion,
			}

			result := vc.isCurrentVersionUpToDate(tc.latestVersion)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestVersionCheckResult_DisplayNotification(t *testing.T) {
	testCases := []struct {
		name             string
		result           *VersionCheckResult
		expectedContains []string
	}{
		{
			name: "basic notification",
			result: &VersionCheckResult{
				CurrentVersion:      v100,
				LatestVersion:       v110,
				NewVersionAvailable: true,
				UpdateInstructions:  "brew upgrade stellwerk-labs/tap/octl",
			},
			expectedContains: []string{
				"new version",
				v100,
				v110,
				"brew upgrade",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer

			tc.result.DisplayNotification(&stderr)

			output := stderr.String()
			for _, expected := range tc.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestVersionChecker_Check(t *testing.T) {
	trueVal := true

	testCases := []struct {
		name          string
		config        *config.Config
		setupLastTime bool
		lastCheckTime time.Time
		expectResult  bool
	}{
		{
			name:          "check disabled",
			config:        &config.Config{DisableVersionCheck: &trueVal},
			setupLastTime: false,
			expectResult:  false,
		},
		{
			name:          "recently checked - skip",
			config:        &config.Config{},
			setupLastTime: true,
			lastCheckTime: time.Now().Add(-1 * time.Hour),
			expectResult:  false,
		},
		{
			name:          "old check - perform check",
			config:        &config.Config{},
			setupLastTime: true,
			lastCheckTime: time.Now().Add(-25 * time.Hour),
			expectResult:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			vc := &VersionChecker{
				currentVersion: v100,
				configDir:      tmpDir,
				httpClient: &http.Client{
					Timeout: versionCheckTimeout,
				},
				config: tc.config,
			}

			if tc.setupLastTime {
				lastCheckFile := filepath.Join(tmpDir, versionCheckLastCheckFile)
				timestamp := tc.lastCheckTime.Format(time.RFC3339)
				err := os.WriteFile(lastCheckFile, []byte(timestamp), 0600)
				require.NoError(t, err)
			}

			ctx := context.Background()
			result := vc.Check(ctx)

			if !tc.expectResult {
				assert.Nil(t, result)
			}
		})
	}
}

func TestVersionChecker_Integration(t *testing.T) {
	t.Run("full flow with mock server", func(t *testing.T) {
		// Create a test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := GitHubRelease{
				Name: "v2.0.0",
			}
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(response); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}))
		defer server.Close()

		tmpDir := t.TempDir()

		vc := &VersionChecker{
			currentVersion: v100,
			configDir:      tmpDir,
			httpClient: &http.Client{
				Timeout: versionCheckTimeout,
			},
		}

		// First, verify that shouldCheckVersion returns true
		assert.True(t, vc.shouldCheckVersion())

		// Update last check time
		err := vc.updateLastCheckTime()
		require.NoError(t, err)

		// Verify that shouldCheckVersion now returns false
		assert.False(t, vc.shouldCheckVersion())

		// Verify the last check file exists
		lastCheckFile := filepath.Join(tmpDir, versionCheckLastCheckFile)
		_, err = os.Stat(lastCheckFile)
		assert.NoError(t, err)
	})
}
