package clients

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

const maxCAFileSize = 4 << 20

// NewSecureTransport clones the standard proxy-aware transport, requires
// TLS 1.2 or newer, and optionally appends an operator-provided CA bundle.
func NewSecureTransport(caFile string) (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default HTTP transport has unexpected type %T", http.DefaultTransport)
	}
	transport := base.Clone()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		info, err := os.Stat(filepath.Clean(caFile))
		if err != nil {
			return nil, fmt.Errorf("inspect CA file: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() > maxCAFileSize {
			return nil, fmt.Errorf("CA file must be a regular file no larger than %d bytes", maxCAFileSize)
		}
		bundle, err := os.ReadFile(filepath.Clean(caFile))
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(bundle) {
			return nil, fmt.Errorf("CA file contains no valid PEM certificates")
		}
		tlsConfig.RootCAs = roots
	}
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}
