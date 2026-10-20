package scripts

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"
)

func releaseConfig(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func configSection(t *testing.T, config, name string) string {
	t.Helper()
	lines := strings.Split(config, "\n")
	var section []string
	found := false
	for _, line := range lines {
		if line == name+":" {
			found = true
			continue
		}
		if !found {
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#") {
			break
		}
		section = append(section, line)
	}
	if !found {
		t.Fatalf("missing %s config section", name)
	}
	return strings.Join(section, "\n")
}

func renderReleaseTemplate(t *testing.T, source string, data map[string]string) string {
	t.Helper()
	tpl, err := template.New("release").Option("missingkey=error").Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := tpl.Execute(&output, data); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestReleaseChannelTemplates(t *testing.T) {
	config := releaseConfig(t)
	release := configSection(t, config, "release")
	if !strings.Contains(release, "  prerelease: auto") {
		t.Fatal("release must derive prerelease status from its tag")
	}
	if !strings.Contains(release, "  use_existing_draft: true") {
		t.Fatal("signed assets must upload to the reserved draft before publication")
	}
	latest := regexp.MustCompile(`(?m)^  make_latest: "(.+)"$`).FindStringSubmatch(release)
	if len(latest) != 2 {
		t.Fatal("missing explicit latest template")
	}
	docker := configSection(t, config, "dockers_v2")
	_, tags, found := strings.Cut(docker, "    tags:\n")
	if !found {
		t.Fatal("missing docker tags")
	}
	tags, _, found = strings.Cut(tags, "    labels:")
	if !found {
		t.Fatal("missing docker label boundary")
	}
	for _, test := range []struct {
		name, version, prerelease, latest string
		tags                              []string
	}{
		{"stable", "2.0.0", "", "true", []string{"2.0.0", "2", "2.0", "latest"}},
		{"candidate", "2.0.0-rc.1", "rc.1", "false", []string{"2.0.0-rc.1"}},
		{"later candidate", "2.0.0-rc.12", "rc.12", "false", []string{"2.0.0-rc.12"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := map[string]string{"Version": test.version, "Major": "2", "Minor": "0", "Prerelease": test.prerelease}
			if got := renderReleaseTemplate(t, latest[1], data); got != test.latest {
				t.Fatalf("make_latest = %q, want %q", got, test.latest)
			}
			var rendered []string
			for _, line := range strings.Split(strings.TrimSpace(tags), "\n") {
				value, ok := strings.CutPrefix(strings.TrimSpace(line), "- ")
				if !ok {
					t.Fatalf("unexpected docker tag line: %q", line)
				}
				value = strings.Trim(value, `"`)
				if tag := renderReleaseTemplate(t, value, data); tag != "" {
					rendered = append(rendered, tag)
				}
			}
			if !reflect.DeepEqual(rendered, test.tags) {
				t.Fatalf("image tags = %v, want %v", rendered, test.tags)
			}
		})
	}
}

func TestCandidateSkipsStablePackageManagersAndKeepsSigning(t *testing.T) {
	config := releaseConfig(t)
	for _, section := range []string{"homebrew_casks", "scoops"} {
		if strings.Count(configSection(t, config, section), "    skip_upload: auto") != 1 {
			t.Fatalf("%s must skip prereleases while retaining stable uploads", section)
		}
	}
	signs := configSection(t, config, "signs")
	for _, required := range []string{"cmd: ./scripts/sign-qualified-artifact.py", "- cosign", "artifacts: all", "${signature}", "${certificate}", "{{ .FullCommit }}"} {
		if !strings.Contains(signs, required) {
			t.Fatalf("existing release signing contract lost %q", required)
		}
	}
	wrapper, err := os.ReadFile("sign-qualified-artifact.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"release.recheck(", "--oidc-provider=github-actions", "--output-signature=", "--output-certificate=", "--certificate-identity", "--certificate-oidc-issuer"} {
		if !strings.Contains(string(wrapper), required) {
			t.Fatalf("qualified signer lost existing Cosign contract %q", required)
		}
	}
}

func TestReleaseWorkflowsPinTheVerifiedCosignV2Contract(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/ci.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name, Uses string
				With       map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	installers := 0
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if !strings.HasPrefix(step.Uses, "sigstore/cosign-installer@") {
				continue
			}
			installers++
			if step.Uses != "sigstore/cosign-installer@7e8b541eb2e61bf99390e1afd4be13a184e9ebc5" || step.With["cosign-release"] != "v2.6.1" {
				t.Fatal("release signer must use the verified immutable installer and explicit Cosign v2 version")
			}
		}
	}
	if installers != 2 {
		t.Fatal("both candidate and stable releases must install the pinned signer")
	}
}

func releaseBuilderPolicy(data []byte) error {
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string
				With map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return err
	}
	for _, name := range []string{"candidate-release", "release"} {
		job, exists := workflow.Jobs[name]
		if !exists {
			return fmt.Errorf("missing release job %s", name)
		}
		builders := 0
		for _, step := range job.Steps {
			if !strings.HasPrefix(step.Uses, "docker/setup-buildx-action@") {
				continue
			}
			builders++
			if step.Uses != "docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e" ||
				step.With["version"] != "v0.38.0" {
				return fmt.Errorf("%s must pin the verified Buildx installer and version", name)
			}
			want := []string{
				"image=moby/buildkit@sha256:cec9f139f45e93c5c69c60f8b07cfad9f43f4ef6b6a6cd917527fea5ff2e3dea",
				"provenance-add-gha=false",
			}
			if !reflect.DeepEqual(strings.Fields(step.With["driver-opts"]), want) {
				return fmt.Errorf("%s must preserve pinned BuildKit and omit only auxiliary GitHub context", name)
			}
		}
		if builders != 1 {
			return fmt.Errorf("%s must configure exactly one release builder", name)
		}
	}
	return nil
}

func TestReleaseBuildersPinVersionAndOmitAuxiliaryGitHubContext(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/ci.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseBuilderPolicy(data); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"candidate-release", "release"} {
		for _, mutation := range []struct{ name, old, replacement string }{
			{"floating-version", "          version: v0.38.0\n", ""},
			{"different-version", "version: v0.38.0", "version: v0.37.2"},
			{"missing-opt-out", "            provenance-add-gha=false\n", ""},
			{"enabled-context", "provenance-add-gha=false", "provenance-add-gha=true"},
			{"unpinned-buildkit", "image=moby/buildkit@sha256:cec9f139f45e93c5c69c60f8b07cfad9f43f4ef6b6a6cd917527fea5ff2e3dea", "image=moby/buildkit:latest"},
		} {
			t.Run(name+"/"+mutation.name, func(t *testing.T) {
				prefix, section, found := strings.Cut(string(data), "\n  "+name+":\n")
				if !found || !strings.Contains(section, mutation.old) {
					t.Fatal("mutation must apply to the actual release job")
				}
				mutated := prefix + "\n  " + name + ":\n" + strings.Replace(section, mutation.old, mutation.replacement, 1)
				if err := releaseBuilderPolicy([]byte(mutated)); err == nil {
					t.Fatal("unsafe release builder mutation accepted")
				}
			})
		}
	}
}

func TestChannelMetadataUsesHomebrewCompatibleStyle(t *testing.T) {
	config := releaseConfig(t)
	for _, section := range []string{"homebrew_casks", "scoops"} {
		metadata := configSection(t, config, section)
		for _, required := range []string{`homepage: "https://docs.stellwerk.dev/"`, `description: "Stellwerk CLI"`} {
			if !strings.Contains(metadata, required) {
				t.Fatalf("%s must retain valid shared package metadata: %s", section, required)
			}
		}
	}
}
