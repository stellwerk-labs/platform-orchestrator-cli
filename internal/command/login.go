package command

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

var (
	noBrowserFlag bool
)

var Login = &cobra.Command{
	Use:   "login",
	Args:  cobra.NoArgs,
	Short: "Log in to the system",
	Annotations: map[string]string{
		SkipConfigContextAnnotation: stringTrue,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		iamClient := MustIamClient(cmd.Context())

		resp, err := iamClient.RequestDeviceLoginWithResponse(cmd.Context(), &iam.RequestDeviceLoginParams{
			UserAgent: ref.Ref(fmt.Sprintf("octl %s %s", internal.ModulePath, internal.ModuleVersion)),
		}, iam.RequestDeviceLoginJSONRequestBody{})
		if err != nil {
			return err
		}

		if resp.StatusCode() != 201 {
			return fmt.Errorf("failed to initiate device login: status code: %d, body: %s", resp.StatusCode(), string(resp.Body))
		}

		successMessageF("Please visit the following URL to login: %s", resp.JSON201.ApprovalUrl)
		if !noBrowserFlag {
			if err := openBrowser(resp.JSON201.ApprovalUrl); err != nil {
				infoMessageF("Failed to open browser automatically: %v", err)
			}
		}

		timeout := time.NewTimer(3 * time.Minute)
		ticker := time.NewTicker(500 * time.Millisecond)
		animationTicker := time.NewTicker(500 * time.Millisecond)

		defer timeout.Stop()
		defer ticker.Stop()
		defer animationTicker.Stop()

		animationCtx, animationCancel := context.WithCancel(cmd.Context())
		defer animationCancel()

		go func() {
			dots := []string{"", ".", "..", "..."}
			i := 0
			for {
				select {
				case <-animationCtx.Done():
					return
				case <-animationTicker.C:
					fmt.Print("\rWaiting for approval in your browser" + dots[i] + "   ")
					i = (i + 1) % len(dots)
				}
			}
		}()

		var acceptedRequest *iam.AcceptedDeviceLoginRequest
		for acceptedRequest == nil {
			select {
			case <-timeout.C:
				stopAnimation(animationCancel)
				return fmt.Errorf("device login timed out after 3 minutes")
			case <-ticker.C:
				pollResp, err := iamClient.PollDeviceLoginRequestWithResponse(cmd.Context(), resp.JSON201.Id, &iam.PollDeviceLoginRequestParams{
					PollingToken: resp.JSON201.PollingToken,
				})
				if err != nil {
					stopAnimation(animationCancel)
					return fmt.Errorf("failed to poll device login: %v", err)
				}

				if pollResp.StatusCode() == 202 {
					continue
				}
				stopAnimation(animationCancel)

				if pollResp.StatusCode() != 200 {
					return fmt.Errorf("failed to poll device login: status code: %d, body: %s", pollResp.StatusCode(), string(pollResp.Body))
				}

				acceptedRequest = pollResp.JSON200
			}
		}

		successMessageF("Login successful!")
		successMessageF("Token expires at: %s", acceptedRequest.ExpiresAt.Format(time.RFC3339))

		cu, err := iamClient.GetCurrentUserWithResponse(cmd.Context(), func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+acceptedRequest.Token)
			return nil
		})
		if err != nil {
			return err
		} else if cu.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting current user using new token: %s", cu.StatusCode(), string(cu.Body))
		}
		orgNames := slices.Sorted(func(yield func(string) bool) {
			for _, m := range cu.JSON200.OrganizationMemberships {
				yield(m.Id)
			}
		})

		cfg, err := config.ReadFile()
		if err != nil {
			return fmt.Errorf("failed to read configuration: %v", err)
		}
		cfg.Token = acceptedRequest.Token

		if len(orgNames) == 1 {
			cfg.DefaultOrg = orgNames[0]
			successMessageF("We set your org to %s", cfg.DefaultOrg)
		} else if !slices.Contains(orgNames, cfg.DefaultOrg) {
			infoMessageF("Warning: User does not belong to configured organization '%s'.", cfg.DefaultOrg)
			infoMessageF("Consider setting PO_ORG_ID or use 'octl config set-org' to one of the orgs: %s.", strings.Join(orgNames, ", "))
		}

		if err := config.SaveFile(cfg); err != nil {
			return fmt.Errorf("failed to save configuration: %v", err)
		}
		return nil
	},
}

func stopAnimation(cancel context.CancelFunc) {
	cancel()
	fmt.Println()
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // G204: intentional browser open with server-provided URL
	case "linux":
		cmd = exec.Command("xdg-open", url) //nolint:gosec // G204: intentional browser open with server-provided URL
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // G204: intentional browser open with server-provided URL
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return cmd.Start()
}

func init() {
	Login.Flags().BoolVar(&noBrowserFlag, "no-browser", false, "Don't automatically open browser with approval URL")
	RootCmd.AddCommand(Login)
}
