package printer

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Diagnostic fixture from Runner internal/runner/runner_test.go, not customer logs.
const diagnosticLog = `{"@level":"error","@message":"Error: test error","@module":"tofu.ui","@timestamp":"2025-01-08T10:00:00.000000Z","diagnostic":{"severity":"error","summary":"Test Summary","detail":"Test Detail","snippet":{"context":"resource","code":"first\nsecond"}},"type":"diagnostic"}`

func TestFormatRunnerLogsProducerRecords(t *testing.T) {
	for name, fixture := range map[string]struct {
		body, message string
		retained      []string
	}{
		"slog":         {`{"time":"2026-11-12T20:00:00Z","level":"INFO","msg":"IaC binary is available","runner":{"org_id":"fixture-org","version":"OpenTofu v1.10.0\non darwin_arm64\n"}}`, "IaC binary is available", []string{"fixture-org", "OpenTofu v1.10.0", "runner"}},
		"diagnostic":   {diagnosticLog, "Error: test error", []string{"tofu.ui", "Test Summary", "Test Detail", "first\\nsecond"}},
		"future event": {`{"@level":"info","@message":"cost ready\nnext line","type":"cost_estimate","hook":{"resource":"example"},"cost":{"monthly":12345678901234567890},"extra":[true,null]}`, "cost ready\nnext line", []string{"cost_estimate", "hook", "monthly", "12345678901234567890", "\"extra\": [\n    true,\n    null\n  ]"}},
		"unknown JSON": {`{"message":"not a known record","extra":{"value":42}}`, `"message": "not a known record"`, []string{"\n", `"extra": {`, "42"}},
	} {
		t.Run(name, func(t *testing.T) {
			text := string(FormatRunnerLogs([]byte(fixture.body), false))
			require.Contains(t, text, fixture.message)
			for _, retained := range fixture.retained {
				assert.Contains(t, text, retained)
			}
			assert.NotContains(t, text, "\x1b")
		})
	}
}

func TestFormatRunnerLogsKeepsFallbackContent(t *testing.T) {
	oversized := `{"@message":"` + strings.Repeat("x", maxRunnerLogRecordBytes) + `","@level":"info"}`
	expanded := strings.Repeat("[", 64) + strings.Repeat("0,", 31999) + "0" + strings.Repeat("]", 64)
	deep := strings.Repeat("[", 1000) + "0" + strings.Repeat("]", 1000)
	for _, body := range []string{"", "\n\nplain\n\n", "plain\n{malformed\n{\"partial\":", "雪\nline without newline", oversized, expanded, deep, "```json\n{not JSON}\n```"} {
		assert.Equal(t, body, string(FormatRunnerLogs([]byte(body), false)))
	}
	assert.Equal(t, `tail\xff`, string(FormatRunnerLogs([]byte{'t', 'a', 'i', 'l', 0xff}, false)))
}

func TestFormatRunnerLogsNeutralizesDecodedAndPlainTerminalControls(t *testing.T) {
	body := `{"time":"\u001b]52;c;metadata\u0007","level":"INFO\r","msg":"\u001b]52;c;payload\u0007\b\r\u009b31m","extra":"\u001b[31m"}` + "\nplain\x1b[31m\r\b\x9d"
	text := string(FormatRunnerLogs([]byte(body), false))
	for _, control := range []string{"\x1b", "\r", "\b", "\a", "\u009b", "\u009d"} {
		assert.NotContains(t, text, control)
	}
	assert.Contains(t, text, `\x1b]52;c;payload`)
	assert.Contains(t, text, `INFO\r`)
}

func TestRunnerLogsANSIRequiresDestinationTerminal(t *testing.T) {
	assert.False(t, RunnerLogsTerminal(&bytes.Buffer{}))
	assert.False(t, RunnerLogsColor(&bytes.Buffer{}))
	pipe, err := os.CreateTemp(t.TempDir(), "logs")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pipe.Close() })
	assert.False(t, RunnerLogsColor(pipe))
	for _, environment := range []struct{ key, value string }{{"NO_COLOR", ""}, {"TERM", "dumb"}} {
		t.Run(environment.key, func(t *testing.T) {
			t.Setenv(environment.key, environment.value)
			assert.False(t, RunnerLogsColor(pipe))
			assert.Contains(t, string(FormatRunnerLogs([]byte(diagnosticLog), false)), "Error: test error\n")
		})
	}
	assert.Contains(t, string(FormatRunnerLogs([]byte(diagnosticLog), true)), "\x1b[")
	assert.Contains(t, string(FormatRunnerLogs([]byte(`{"unknown":42}`), true)), "\x1b[")
}

func TestRunnerLogsRealTerminalColorPolicy(t *testing.T) {
	if !RunnerLogsTerminal(os.Stdout) {
		t.Skip("run the compiled test binary in a terminal to exercise TTY color policy")
	}
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	require.NoError(t, os.Unsetenv("NO_COLOR"))
	assert.True(t, RunnerLogsColor(os.Stdout))
	t.Setenv("NO_COLOR", "")
	assert.False(t, RunnerLogsColor(os.Stdout), "even an empty NO_COLOR disables ANSI")
	require.NoError(t, os.Unsetenv("NO_COLOR"))
	t.Setenv("TERM", "dumb")
	assert.False(t, RunnerLogsColor(os.Stdout))
	assert.Contains(t, string(FormatRunnerLogs([]byte(diagnosticLog), RunnerLogsColor(os.Stdout))), "Error: test error\n")
}
