package scripts

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestReleaseBinarySecurity(t *testing.T) {
	var config struct {
		Builds []struct {
			Flags   []string `yaml:"flags"`
			Ldflags []string `yaml:"ldflags"`
		} `yaml:"builds"`
	}
	data, err := os.ReadFile("../.goreleaser.yaml")
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &config))
	require.Len(t, config.Builds, 1)
	build := config.Builds[0]
	require.NotEmpty(t, build.Flags)
	require.NotEmpty(t, build.Ldflags)
	parsed, err := template.New("ldflags").Option("missingkey=error").Parse(strings.Join(build.Ldflags, " "))
	require.NoError(t, err)
	var rendered bytes.Buffer
	require.NoError(t, parsed.Execute(&rendered, map[string]string{"Version": "2.4.0", "Commit": strings.Repeat("1", 40)}))
	for key, value := range map[string]string{
		"GOTOOLCHAIN": "go1.26.11", "GOWORK": "off", "GOENV": "off", "CGO_ENABLED": "0",
		"GOFLAGS": "", "GOEXPERIMENT": "", "GOARM": "", "GOAMD64": "",
	} {
		t.Setenv(key, value)
	}
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.com/release-symbols\n\ngo 1.26.0\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\nimport \"fmt\"\nvar version, commit string\nfunc main(){fmt.Println(version,commit)}\n"), 0600))
	// These real formats cover symbol-table differences; GoReleaser owns its target matrix.
	for _, target := range []string{"linux/amd64", "windows/amd64", "darwin/arm64"} {
		t.Run(target, func(t *testing.T) {
			osName, arch, _ := strings.Cut(target, "/")
			binary := filepath.Join(t.TempDir(), "client")
			for _, stripped := range []bool{false, true} {
				t.Setenv("GOOS", osName)
				t.Setenv("GOARCH", arch)
				flags := rendered.String()
				if stripped {
					flags += " -s"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				command := exec.CommandContext(ctx, "go", "build")
				command.Args = append(command.Args, build.Flags...)
				command.Args = append(command.Args, "-ldflags", flags, "-o", binary, ".")
				command.Dir = source
				output, buildErr := command.CombinedOutput()
				cancel()
				require.NoError(t, buildErr, "%s", output)
				// The scanner must execute on its host despite an inherited foreign target.
				t.Setenv("GOOS", "windows")
				t.Setenv("GOARCH", "386")
				ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
				command = exec.CommandContext(ctx, "sh", "check-go-security.sh", "binary")
				command.Args = append(command.Args, binary)
				output, scanErr := command.CombinedOutput()
				cancel()
				if stripped {
					require.Error(t, scanErr, "stripped executable passed: %s", output)
				} else {
					require.NoError(t, scanErr, "%s", output)
				}
			}
		})
	}
}
