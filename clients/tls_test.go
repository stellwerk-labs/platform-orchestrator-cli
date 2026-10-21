package clients

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSecureTransportTrustsExplicitCAWithoutDisablingVerification(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	certificate := server.Certificate()
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600))
	transport, err := NewSecureTransport(caFile)
	require.NoError(t, err)
	client := &http.Client{Transport: transport}

	response, err := client.Get(server.URL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.False(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestNewSecureTransportRejectsInvalidBundle(t *testing.T) {
	t.Parallel()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, []byte("not a certificate"), 0o600))
	_, err := NewSecureTransport(caFile)
	require.ErrorContains(t, err, "no valid PEM certificates")
}
