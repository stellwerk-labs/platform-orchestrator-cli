package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNullableCorrectionRequiresEveryExactBoundary(t *testing.T) {
	header, err := os.ReadFile("testdata/nullable-header.txt")
	if err != nil {
		t.Fatal(err)
	}
	supplement, err := os.ReadFile("../release-notices/apache-2.0.txt")
	if err != nil {
		t.Fatal(err)
	}
	valid := module{Path: nullablePath, Version: nullableVersion, Sum: nullableSum}
	if err := verifyNullable(valid, header, supplement); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"path", "version", "sum", "replacement", "header", "supplement"} {
		t.Run(kind, func(t *testing.T) {
			m, original, full := valid, append([]byte{}, header...), append([]byte{}, supplement...)
			switch kind {
			case "path":
				m.Path += "-collision"
			case "version":
				m.Version = "v1.1.0"
			case "sum":
				m.Sum += "changed"
			case "replacement":
				replacement := json.RawMessage(`{}`)
				m.Replace = &replacement
			case "header":
				original[0] ^= 1
			case "supplement":
				full[0] ^= 1
			}
			if verifyNullable(m, original, full) == nil {
				t.Fatal("changed notice boundary was accepted")
			}
		})
	}
	for _, collision := range []string{nullablePath + "-fork", nullablePath + "/v2"} {
		_, err := noticeCorrection(t.TempDir(), t.TempDir(), map[string]module{collision: {Path: collision}})
		if err == nil || !strings.Contains(err.Error(), "collides") {
			t.Fatal("prefix collision was not rejected")
		}
	}
}

func TestConfiguredTargetUnion(t *testing.T) {
	ports := map[string]bool{"linux/amd64": true, "linux/arm64": true, "darwin/arm64": true, "windows/amd64": true, "freebsd/386": true}
	for _, test := range []struct {
		config   string
		expected []string
	}{
		{"builds:\n- targets: [linux_amd64_v1, darwin_arm64, windows_amd64_v1]\n", []string{"darwin/arm64", "linux/amd64", "windows/amd64"}},
		{"builds:\n- goos: [linux, freebsd]\n  goarch: [amd64, arm64, '386']\n  ignore:\n  - goos: linux\n    goarch: arm64\n", []string{"freebsd/386", "linux/amd64"}},
	} {
		got, err := targets([]byte(test.config), ports)
		if err != nil || !reflect.DeepEqual(got, test.expected) {
			t.Fatalf("targets=%v error=%v", got, err)
		}
	}
	for _, config := range []string{"builds: []", "builds:\n- main: ./other\n  targets: [linux_amd64_v1]", "builds:\n- targets: [linux_amd64_v2]"} {
		if _, err := targets([]byte(config), ports); err == nil {
			t.Fatal("unhandled build configuration accepted")
		}
	}
}

func TestNoticeFilesMustBeNonemptyRegularAndIdentical(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	if copyNotice(source, destination) == nil {
		t.Fatal("missing notice accepted")
	}
	if err := os.WriteFile(source, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if copyNotice(source, destination) == nil {
		t.Fatal("empty notice accepted")
	}
	if err := copyFile(source, filepath.Join(root, ".gitkeep"), false); err != nil {
		t.Fatal("empty reciprocal-license source placeholder was dropped")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destination, source); err != nil {
		t.Fatal(err)
	}
	if copyNotice(source, destination) == nil {
		t.Fatal("symlink notice accepted")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("notice"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyNotice(source, destination); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if copyNotice(source, destination) == nil {
		t.Fatal("conflicting notice accepted")
	}
}

func TestRegenerationOnlyReplacesAnUnchangedGeneratedTree(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "licenses")
	if resetNotices(root, root) == nil {
		t.Fatal("broad reset destination accepted")
	}
	if err := os.Mkdir(output, 0o750); err != nil {
		t.Fatal(err)
	}
	if resetNotices(root, output) == nil {
		t.Fatal("unmarked existing directory accepted")
	}
	content := []byte("generated legal notice\n")
	hash := sha256.Sum256(content)
	manifest, err := json.Marshal(map[string]string{"LICENSE": hex.EncodeToString(hash[:])})
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(output, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("notices.sha256.json", manifest)
	write("LICENSE", []byte("changed"))
	if resetNotices(root, output) == nil {
		t.Fatal("changed generated content accepted")
	}
	write("LICENSE", content)
	write("personal-file", []byte("keep"))
	if resetNotices(root, output) == nil {
		t.Fatal("untracked file accepted")
	}
	if _, err := os.Stat(filepath.Join(output, "personal-file")); err != nil {
		t.Fatal("untracked file was removed")
	}
	if err := os.Remove(filepath.Join(output, "personal-file")); err != nil {
		t.Fatal(err)
	}
	if err := resetNotices(root, output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("generated tree not replaced")
	}
}

func TestRealCollectorIncludesWindowsOnlyNoticesAndRejectsUnknown(t *testing.T) {
	// These are real modules and the real pinned classifier, not a mocked command.
	t.Setenv("GOTOOLCHAIN", "go1.26.10")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOENV", "off")
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOEXPERIMENT", "")
	if _, err := exec.LookPath("go-licenses"); err != nil {
		tools := t.TempDir()
		// #nosec G204 -- Fixed repository installer and a test-owned temporary destination, without shell interpolation.
		command := exec.Command("bash", "../collect-licenses.sh", "--install-tool", tools)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("pinned license tool installation: %v\n%s", err, output)
		}
		t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	license, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(goroot)), "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	ownLicense, err := os.ReadFile("../../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	fixture, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(); err != nil {
			t.Error(err)
		}
	})
	write := func(name string, data []byte) {
		t.Helper()
		if err := fixture.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := fixture.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", []byte("module example.com/application\n\ngo 1.26.0\n\nrequire (\nexample.com/common v0.0.0\nexample.com/windows-only v0.0.0\n)\nreplace example.com/common => ./common\nreplace example.com/windows-only => ./windows-only\n"))
	write("main.go", []byte("package main\nimport _ \"example.com/common\"\nfunc main() {}\n"))
	write("main_windows.go", []byte("//go:build windows\n\npackage main\nimport _ \"example.com/windows-only\"\n"))
	write("LICENSE", ownLicense)
	for _, name := range []string{"common", "windows-only"} {
		write(name+"/go.mod", []byte("module example.com/"+name+"\n\ngo 1.26.0\n"))
		write(name+"/package.go", []byte("package dependency\n"))
		write(name+"/LICENSE", license)
		write(name+"/PATENTS", []byte("Patent notice fixture\n"))
	}
	write(".goreleaser.yaml", []byte("builds:\n- targets: [linux_amd64_v1, windows_amd64_v1]\n"))
	t.Chdir(root)
	output := filepath.Join(root, "out")
	if err := collect(root, output, ".goreleaser.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"example.com/common/PATENTS", "example.com/windows-only/PATENTS", "go/PATENTS", "modules.json", "notices.sha256.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatal(err)
		}
	}
	if actual, err := readFile(filepath.Join(output, "example.com/application/LICENSE")); err != nil || !bytes.Equal(actual, ownLicense) {
		t.Fatal("application license changed or missing")
	}
	var graphs map[string]map[string]string
	data, err := readFile(filepath.Join(output, "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &graphs); err != nil {
		t.Fatal(err)
	}
	if _, exists := graphs["linux/amd64"]["example.com/windows-only"]; exists {
		t.Fatal("Windows-only module incorrectly appears in Linux graph")
	}
	if _, exists := graphs["windows/amd64"]["example.com/windows-only"]; !exists {
		t.Fatal("Windows-only compiled dependency omitted")
	}
	second := filepath.Join(root, "second")
	if err := collect(root, second, ".goreleaser.yaml"); err != nil {
		t.Fatal(err)
	}
	firstManifest, err := readFile(filepath.Join(output, "notices.sha256.json"))
	if err != nil {
		t.Fatal(err)
	}
	secondManifest, err := readFile(filepath.Join(second, "notices.sha256.json"))
	if err != nil || !bytes.Equal(firstManifest, secondManifest) {
		t.Fatal("repeated collection changed distribution bytes")
	}
	info, err := os.Stat(filepath.Join(second, "modules.json"))
	if err != nil || info.ModTime().Unix() != 0 {
		t.Fatal("generated notice timestamp is not normalized")
	}
	if err := collect(root, output, ".goreleaser.yaml"); err == nil {
		t.Fatal("existing notices overwritten")
	}
	write("windows-only/LICENSE", []byte("Unrecognized license fixture\n"))
	if err := collect(root, filepath.Join(root, "unknown-out"), ".goreleaser.yaml"); err == nil || !strings.Contains(err.Error(), "classification for windows/amd64") {
		t.Fatalf("unknown Windows license accepted: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "windows-only/LICENSE")); err != nil {
		t.Fatal(err)
	}
	if err := collect(root, filepath.Join(root, "missing-out"), ".goreleaser.yaml"); err == nil {
		t.Fatal("missing dependency license accepted")
	}
}
