package clients

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

func TestCollectAll_not_ok(t *testing.T) {

	items, err := CollectAll[cp.Project, cp.ListProjectsResponse](func(s string) (cp.ListProjectsResponse, error) {
		return cp.ListProjectsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
			Body:         []byte(`{"b": "a"}`),
		}, nil

	}, func(response cp.ListProjectsResponse) ([]cp.Project, *string) {
		return response.JSON200.Items, response.JSON200.NextPageToken
	})
	require.EqualError(t, err, `unexpected status code 500 when listing items: {"b": "a"}`)
	require.Empty(t, items)

}
