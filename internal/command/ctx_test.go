package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithClients(t *testing.T) {
	var err error

	tests := []struct {
		name      string
		apiPrefix string
		apiToken  string
		errMsg    string
	}{
		{
			name:      "success",
			apiPrefix: "https://test-api.platform-orchestrator.io",
			apiToken:  "SOMETOKEN",
		},
		{
			name:      "success with localhost",
			apiPrefix: "http://localhost:8080",
		},
		{
			name:      "failed - token not set",
			apiPrefix: "https://test-api.platform-orchestrator.io",
			errMsg:    "Authentication token must be provided in configuration or environment variable. Consider using 'octl login', 'octl config set-token', or setting the " + AuthTokenEnvVar + " environment variable.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			ctx, err = withClients(ctx, test.apiPrefix, "my-org", test.apiToken)
			if test.errMsg == "" {
				require.NoError(t, err)
				assert.NotNil(t, ctx.Value(CpClientContextKey))
				assert.NotNil(t, ctx.Value(DpClientContextKey))
			} else {
				assert.ErrorContains(t, err, test.errMsg)
			}
		})
	}
}
