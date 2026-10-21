package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseArchivesKeepCompleteNoticeTree(t *testing.T) {
	const owner = "root"
	var config struct {
		Before struct{ Hooks []string }
		Builds []struct {
			Flags        []string
			ModTimestamp string `yaml:"mod_timestamp"`
		}
		Archives []struct {
			BuildsInfo struct{ Mtime, Owner, Group string } `yaml:"builds_info"`
			Files      []struct {
				Src  string
				Info struct {
					Mtime, Owner, Group string
					Mode                int
				}
			}
		}
	}
	data, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, hook := range config.Before.Hooks {
		found = found || hook == "./scripts/collect-licenses.sh --replace-generated licenses"
	}
	if !found {
		t.Fatal("release hook no longer generates the complete notice union")
	}
	if len(config.Builds) == 0 || len(config.Archives) == 0 {
		t.Fatal("release build/archive configuration is missing")
	}
	for _, build := range config.Builds {
		trimpath := false
		for _, flag := range build.Flags {
			trimpath = trimpath || flag == "-trimpath"
		}
		if !trimpath || build.ModTimestamp != "{{ .CommitTimestamp }}" {
			t.Fatal("release binary path/timestamp normalization is missing")
		}
	}
	for _, archive := range config.Archives {
		if archive.BuildsInfo.Mtime != "{{ .CommitDate }}" || archive.BuildsInfo.Owner != owner || archive.BuildsInfo.Group != owner {
			t.Fatal("archive binary metadata is not normalized")
		}
		required := map[string]bool{"LICENSE": false, "README.md": false, "licenses/**/*": false, "licenses/modules.json": false, "licenses/notices.sha256.json": false}
		for _, file := range archive.Files {
			if _, exists := required[file.Src]; !exists {
				continue
			}
			required[file.Src] = true
			if file.Info.Mtime != "{{ .CommitDate }}" || file.Info.Owner != owner || file.Info.Group != owner || file.Info.Mode != 0o644 {
				t.Fatal("archive notice metadata is not normalized")
			}
		}
		for name, found := range required {
			if !found {
				t.Fatalf("release archive omits %s", name)
			}
		}
	}
}

func TestReleaseImageKeepsNoticesAndPinnedBase(t *testing.T) {
	data, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Dockers []struct {
			Disable    bool
			ExtraFiles []string `yaml:"extra_files"`
			Flags      []string
		} `yaml:"dockers_v2"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Dockers) == 0 {
		t.Fatal("release image configuration is missing")
	}
	for _, image := range config.Dockers {
		if !image.Disable {
			t.Fatal("GoReleaser must not rebuild or push an uninspected image")
		}
		notices, epoch := false, false
		for _, file := range image.ExtraFiles {
			notices = notices || file == "licenses"
		}
		for _, flag := range image.Flags {
			epoch = epoch || flag == "--build-arg=SOURCE_DATE_EPOCH={{ .CommitTimestamp }}"
		}
		if !notices || !epoch {
			t.Fatal("release image omits notice context or source timestamp")
		}
	}
	dockerfile, err := os.ReadFile("../../goreleaser.Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"COPY licenses /licenses", "gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3"} {
		if !strings.Contains(string(dockerfile), required) {
			t.Fatalf("release Dockerfile omits %s", required)
		}
	}
}
