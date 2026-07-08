package command

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justinrixx/retryhttp"
	"github.com/pkg/errors"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

type contextKey string

const (
	ExtraRequestHeadersEnvVar = "PO_EXTRA_REQUEST_HEADERS"
	AuthTokenEnvVar           = "PO_AUTH_TOKEN" // #nosec G101

	CpClientContextKey  contextKey = "cp-client"
	DpClientContextKey  contextKey = "dp-client"
	IamClientContextKey contextKey = "iam-client"
	ConfigContextKey    contextKey = "config"
	PrinterContextKey   contextKey = "printer"
)

func MustCpClient(c context.Context) cp.ClientWithResponsesInterface {
	v, ok := c.Value(CpClientContextKey).(cp.ClientWithResponsesInterface)
	if !ok {
		panic("no client in context")
	}
	return v
}

func MustDpClient(c context.Context) dp.ClientWithResponsesInterface {
	v, ok := c.Value(DpClientContextKey).(dp.ClientWithResponsesInterface)
	if !ok {
		panic("no client in context")
	}
	return v
}

func MustIamClient(c context.Context) iam.ClientWithResponsesInterface {
	v, ok := c.Value(IamClientContextKey).(iam.ClientWithResponsesInterface)
	if !ok {
		panic("no IAM client in context")
	}
	return v
}

func MustConfiguration(c context.Context) config.Config {
	v, ok := c.Value(ConfigContextKey).(config.Config)
	if !ok {
		panic("no configuration in context")
	}
	return v
}

func MustPrinter(c context.Context) printer.Printer {
	v, ok := c.Value(PrinterContextKey).(printer.Printer)
	if !ok {
		panic("no printer in context")
	}
	return v
}

func ShouldApiUrl(c context.Context) (string, error) {
	v, ok := c.Value(ConfigContextKey).(config.Config)
	if !ok || v.ApiUrl == "" {
		return "", errors.Errorf("No API URL set. Please use 'octl config set-url' to set the URL.")
	}
	return v.ApiUrl, nil
}

func ShouldOrg(c context.Context) (string, error) {
	v, ok := c.Value(ConfigContextKey).(config.Config)
	if !ok || v.DefaultOrg == "" {
		return "", errors.Errorf("No organization set. Please use 'octl config set-org' to set the default organization or specify it with the --org flag or environment variable.")
	}
	return v.DefaultOrg, nil
}

func withConfiguration(ctx context.Context, cfg config.Config) context.Context {
	return context.WithValue(ctx, ConfigContextKey, cfg)
}

func withPrinter(ctx context.Context, outputFormat string, supportedFormats []string) (context.Context, error) {
	found := false
	for _, f := range supportedFormats {
		if outputFormat == f {
			found = true
			break
		}
	}
	if !found {
		return ctx, fmt.Errorf("invalid output format %q, expected one of %v", outputFormat, supportedFormats)
	}

	switch outputFormat {
	case printer.JsonPrinterType:
		return context.WithValue(ctx, PrinterContextKey, &printer.JsonPrinter{}), nil
	case printer.YamlPrinterType:
		return context.WithValue(ctx, PrinterContextKey, &printer.YamlPrinter{}), nil
	case printer.TablePrinterType:
		return context.WithValue(ctx, PrinterContextKey, &printer.TablePrinter{}), nil
	default:
		return ctx, fmt.Errorf("invalid output format %q, expected one of %v", outputFormat, supportedFormats)
	}
}

func withIamClient(ctx context.Context, apiPrefix string) (context.Context, error) {
	// Skip this function if the test has already set this context up
	if ctx.Value(IamClientContextKey) != nil {
		return ctx, nil
	}

	client := &http.Client{
		Transport: retryhttp.New(retryhttp.WithTransport(clients.NewDebuggingTransport(http.DefaultTransport))),
		Timeout:   30 * time.Second,
	}

	iamc, err := iam.NewClientWithResponses(apiPrefix, iam.WithHTTPClient(client))
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup iam client")
	}
	ctx = context.WithValue(ctx, IamClientContextKey, iamc)
	return ctx, nil
}

func withClients(ctx context.Context, apiPrefix, org, authToken string) (context.Context, error) {
	// Skip this function if the test has already set this context up
	if ctx.Value(CpClientContextKey) != nil {
		return ctx, nil
	}

	client := &http.Client{
		Transport: retryhttp.New(retryhttp.WithTransport(clients.NewDebuggingTransport(http.DefaultTransport))),
		Timeout:   30 * time.Second,
	}

	u, _ := url.Parse(apiPrefix)
	extraHeaders := make(http.Header)

	if authToken != "" {
		extraHeaders.Set("Authorization", "Bearer "+authToken)
	} else if u.Hostname() == "localhost" {
		// For the local version, our auth is to just set the 'From' header directly.
		extraHeaders.Set("From", uuid.Nil.String())
	} else {
		return nil, errors.Errorf("Authentication token must be provided in configuration or environment variable. Consider using 'octl login', 'octl config set-token', or setting the %s environment variable.", AuthTokenEnvVar)
	}

	extraHeadersEditor := func(ctx context.Context, req *http.Request) error {
		maps.Copy(req.Header, extraHeaders)
		return nil
	}
	if v := os.Getenv(ExtraRequestHeadersEnvVar); v != "" {
		// However, for development testing, we may need a way to force set the headers to support alternative schemes
		// like basic auth.
		for s := range strings.FieldsFuncSeq(v, func(r rune) bool { return r == ',' }) {
			if parts := strings.SplitN(s, ":", 2); len(parts) == 2 {
				extraHeaders.Set(parts[0], parts[1])
			}
		}
	}

	slog.Debug("Setting up cp client", slog.String("url", apiPrefix))
	cpc, err := cp.NewClientWithResponses(apiPrefix, cp.WithRequestEditorFn(extraHeadersEditor), cp.WithHTTPClient(client))
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup cp client")
	}
	ctx = context.WithValue(ctx, CpClientContextKey, cpc)

	// For testing and development, allow a separate DP api url to be set
	apiDpPrefix := apiPrefix
	if dpUrl := os.Getenv("PO_DP_API_URL"); dpUrl != "" {
		apiDpPrefix = dpUrl
	}

	slog.Debug("Setting up dp client", slog.String("url", apiDpPrefix)) //nolint:gosec // G706: logs a developer-controlled env var
	dpc, err := dp.NewClientWithResponses(apiDpPrefix, dp.WithRequestEditorFn(extraHeadersEditor), dp.WithHTTPClient(client))
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup dp client")
	}
	ctx = context.WithValue(ctx, DpClientContextKey, dpc)

	// For testing and development, allow a separate IAM api url to be set
	apiIamPrefix := apiPrefix
	if e := os.Getenv("PO_IAM_API_URL"); e != "" {
		apiIamPrefix = e
	}

	slog.Debug("Setting up iam client", slog.String("url", apiIamPrefix)) //nolint:gosec // G706: logs a developer-controlled env var
	iamc, err := iam.NewClientWithResponses(apiIamPrefix, iam.WithRequestEditorFn(extraHeadersEditor), iam.WithHTTPClient(client))
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup iam client")
	}
	ctx = context.WithValue(ctx, IamClientContextKey, iamc)

	return ctx, nil
}
