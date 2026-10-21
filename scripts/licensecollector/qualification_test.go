package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseQualifiesBeforeSigningAndChecksumsBindMetadata(t *testing.T) {
	data, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		SBOMs []struct {
			Artifacts, Cmd string
			Documents, Env []string
		} `yaml:"sboms"`
		Signs    []struct{ Cmd string }
		Checksum struct {
			ExtraFiles []struct{ Glob string } `yaml:"extra_files"`
		}
		Release struct {
			ExtraFiles []struct{ Glob string } `yaml:"extra_files"`
		}
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.SBOMs) != 1 || config.SBOMs[0].Artifacts != "any" ||
		config.SBOMs[0].Cmd != "qualify-release-content.sh" ||
		len(config.SBOMs[0].Documents) != 1 || config.SBOMs[0].Documents[0] != "*.spdx.json" {
		t.Fatal("every archive must pass the OSS SBOM qualification pipe before checksums/signing")
	}
	env := strings.Join(config.SBOMs[0].Env, "\n")
	for _, required := range []string{"QUALIFY_SOURCE_SHA={{ .FullCommit }}", "QUALIFY_VERSION={{ .Version }}",
		"QUALIFY_TAG={{ .Tag }}", "QUALIFY_REPOSITORY={{ .Env.GITHUB_REPOSITORY }}",
		"QUALIFY_WORKFLOW_REF={{ .Env.GITHUB_WORKFLOW_REF }}", "QUALIFY_WORKFLOW_SHA={{ .Env.GITHUB_WORKFLOW_SHA }}",
		"QUALIFY_RUN_ID={{ .Env.GITHUB_RUN_ID }}", "QUALIFY_RUN_ATTEMPT={{ .Env.GITHUB_RUN_ATTEMPT }}"} {
		if !strings.Contains(env, required) {
			t.Fatalf("qualification lost immutable producer input %s", required)
		}
	}
	if len(config.Signs) != 1 || config.Signs[0].Cmd != "./scripts/sign-qualified-artifact.py" {
		t.Fatal("release signing must recheck qualified bytes")
	}
	for _, glob := range []string{"dist/*.provenance.json", "dist/release-content.json"} {
		checksummed, shipped := false, false
		for _, file := range config.Checksum.ExtraFiles {
			checksummed = checksummed || file.Glob == glob
		}
		for _, file := range config.Release.ExtraFiles {
			shipped = shipped || file.Glob == glob
		}
		if !checksummed || !shipped {
			t.Fatalf("metadata %s must ship and be included in the signed checksum contract", glob)
		}
	}
}
