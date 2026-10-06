package scripts

import (
	"bytes"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"text/template"
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
	for _, required := range []string{"cmd: cosign", "--oidc-provider=github-actions", "artifacts: all", "--output-signature", "--output-certificate"} {
		if !strings.Contains(signs, required) {
			t.Fatalf("existing release signing contract lost %q", required)
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
