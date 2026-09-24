package internal

import (
	"bytes"
	"context"
	"io"
	"net"
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
	v100    = "v1.0.0"
	v110    = "v1.1.0"
	v121    = "v1.2.1"
	v200rc1 = "2.0.0-rc.1"
	v200    = "v2.0.0"
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
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedResult string
	}{
		{
			name:           "uses tag rather than human release title",
			serverResponse: `{"tag_name":"v1.41.0","name":"Module Management"}`,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedResult: "v1.41.0",
		},
		{
			name:           "server error",
			serverResponse: `{}`,
			serverStatus:   http.StatusInternalServerError,
			expectedError:  true,
			expectedResult: "",
		},
		{
			name:           "missing tag is not replaced by title",
			serverResponse: `{"name":"v1.41.0"}`,
			serverStatus:   http.StatusOK,
			expectedError:  true,
			expectedResult: "",
		},
		{
			name:           "invalid tag",
			serverResponse: `{"tag_name":"not-a-version"}`,
			serverStatus:   http.StatusOK,
			expectedError:  true,
		},
		{
			name:           "malformed response",
			serverResponse: `{`,
			serverStatus:   http.StatusOK,
			expectedError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vc := &VersionChecker{
				httpClient: versionCheckerTestClient(t, tc.serverStatus, tc.serverResponse),
			}
			version, err := vc.fetchLatestVersion(context.Background())
			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expectedResult, version)
		})
	}
}

func versionCheckerTestClient(t *testing.T, status int, body string) *http.Client {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/stellwerk-labs/platform-orchestrator-cli/releases/latest", r.URL.Path)
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		assert.Equal(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))
		w.WriteHeader(status)
		_, err := io.WriteString(w, body)
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	transport := client.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	client.Transport = transport
	client.Timeout = versionCheckTimeout
	t.Cleanup(transport.CloseIdleConnections)
	return client
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
		{"newer installed stable", v200, v121, true},
		{"RC ahead of old stable", v200rc1, v121, true},
		{"stable supersedes RC", v200rc1, v200, false},
		{"stable ahead of RC", "2.0.0", "v2.0.0-rc.1", true},
		{"numeric minor ordering", "1.9.0", "v1.10.0", false},
		{"numeric prerelease ordering", "2.0.0-rc.2", "v2.0.0-rc.10", false},
		{"build metadata does not order releases", "2.0.0+local", "v2.0.0+release", true},
		{"development build", "dev", v200, true},
		{"unknown build", "", v200, true},
		{"invalid remote version", v200, "broken", true},
		{"leading whitespace", "  v2.0.0 abc123", v121, true},
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
	t.Run("notifies once for a newer release", func(t *testing.T) {
		tmpDir := t.TempDir()

		vc := &VersionChecker{
			currentVersion: v100,
			configDir:      tmpDir,
			httpClient:     versionCheckerTestClient(t, http.StatusOK, `{"tag_name":"v2.0.0","name":"Module Management"}`),
		}

		// First, verify that shouldCheckVersion returns true
		assert.True(t, vc.shouldCheckVersion())

		result := vc.Check(context.Background())
		require.NotNil(t, result)
		assert.True(t, result.NewVersionAvailable)
		assert.Equal(t, v200, result.LatestVersion)
		assert.Nil(t, vc.Check(context.Background()))

		// Verify that shouldCheckVersion now returns false
		assert.False(t, vc.shouldCheckVersion())

		// Verify the last check file exists
		lastCheckFile := filepath.Join(tmpDir, versionCheckLastCheckFile)
		_, err := os.Stat(lastCheckFile)
		assert.NoError(t, err)
	})
	t.Run("RC never recommends the older stable release", func(t *testing.T) {
		vc := &VersionChecker{
			currentVersion: v200rc1,
			configDir:      t.TempDir(),
			httpClient:     versionCheckerTestClient(t, http.StatusOK, `{"tag_name":"v1.2.1"}`),
		}
		assert.Nil(t, vc.Check(context.Background()))
	})
}
