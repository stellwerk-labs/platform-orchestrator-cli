// Collect the union of notices from the actual configured release build graphs.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type build struct {
	Main    string
	Targets []string
	Goos    []string
	Goarch  []string
	Ignore  []struct{ Goos, Goarch string }
}

type module struct {
	Path, Version, Dir, Sum string
	Main                    bool
	Replace                 *json.RawMessage
}

const nullablePath = "github.com/oapi-codegen/nullable"
const nullableVersion = "v1.2.0"
const nullableSum = "h1:VflFkDW980KhBPiFF7nWSyjg+r4Obqj8lXipV0UkP5w="
const nullableHeaderSHA256 = "d41218ff141de28756b85f79958bc33e077e9bfa6bd695b5e2b8736b779f5a9d"
const apacheSHA256 = "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30"

func verifyNullable(m module, header, supplement []byte) error {
	if m.Path != nullablePath || m.Version != nullableVersion || m.Sum != nullableSum || m.Replace != nil {
		return errors.New("nullable notice correction requires the exact unreplaced module identity")
	}
	h := sha256.Sum256(header)
	s := sha256.Sum256(supplement)
	if hex.EncodeToString(h[:]) != nullableHeaderSHA256 || hex.EncodeToString(s[:]) != apacheSHA256 {
		return errors.New("nullable notice correction content hash mismatch")
	}
	return nil
}

func noticeCorrection(root, output string, modules map[string]module) (string, error) {
	for path, m := range modules {
		if !strings.HasPrefix(path, nullablePath) {
			continue
		}
		// go-licenses --ignore matches prefixes. Never let this correction hide
		// another module, fork, major-version path or replacement.
		if path != nullablePath {
			return "", errors.New("module prefix collides with the nullable notice correction")
		}
		headerPath := filepath.Join(m.Dir, "LICENSE")
		supplementPath := filepath.Join(root, "scripts", "release-notices", "apache-2.0.txt")
		header, err := readFile(headerPath)
		if err != nil {
			return "", err
		}
		supplement, err := readFile(supplementPath)
		if err != nil {
			return "", err
		}
		if err := verifyNullable(m, header, supplement); err != nil {
			return "", err
		}
		for source, name := range map[string]string{headerPath: "LICENSE", supplementPath: "LICENSE-2.0.txt"} {
			if err := copyNotice(source, filepath.Join(output, nullablePath, name)); err != nil {
				return "", err
			}
		}
		provenance := map[string]string{
			"module": nullablePath, "version": nullableVersion, "moduleSum": nullableSum,
			"originalSource":   "https://raw.githubusercontent.com/oapi-codegen/nullable/v1.2.0/LICENSE",
			"originalSHA256":   nullableHeaderSHA256,
			"supplementSource": "https://www.apache.org/licenses/LICENSE-2.0.txt", "supplementSHA256": apacheSHA256,
			"reason": "Original copyright and Apache-2.0 header preserved; go-licenses excludes header-only classifier matches. Full authoritative license supplied without changing dependency bytes.",
		}
		data, err := json.MarshalIndent(provenance, "", "  ")
		if err != nil {
			return "", err
		}
		if err := writeFile(filepath.Join(output, nullablePath, "notice-provenance.json"), append(data, '\n')); err != nil {
			return "", err
		}
	}
	if _, ok := modules[nullablePath]; ok {
		return nullablePath, nil
	}
	return "", nil
}

func targets(config []byte, supported map[string]bool) ([]string, error) {
	var c struct{ Builds []build }
	if err := yaml.Unmarshal(config, &c); err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, b := range c.Builds {
		if b.Main != "" && b.Main != "." {
			return nil, errors.New("notice collector must be updated for a new main package")
		}
		requested := b.Targets
		if len(requested) == 0 {
			for _, goos := range b.Goos {
				for _, goarch := range b.Goarch {
					ignored := false
					for _, item := range b.Ignore {
						ignored = ignored || item.Goos == goos && item.Goarch == goarch
					}
					if !ignored {
						requested = append(requested, goos+"_"+goarch)
					}
				}
			}
		}
		for _, target := range requested {
			parts := strings.Split(target, "_")
			if len(parts) < 2 || len(parts) > 3 {
				return nil, errors.New("unrecognized release target")
			}
			if len(parts) == 3 && parts[2] != "v1" {
				return nil, errors.New("notice collector must be updated for a new microarchitecture variant")
			}
			// Current builds use only the baseline architecture variants.
			// GoReleaser also filters unsupported OS/architecture combinations.
			pair := parts[0] + "/" + parts[1]
			if supported[pair] {
				set[pair] = true
			}
		}
	}
	var result []string
	for target := range set {
		result = append(result, target)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil, errors.New("no supported release targets")
	}
	return result, nil
}

func run(env []string, command string, args ...string) ([]byte, error) {
	if command != "go" && command != "go-licenses" {
		return nil, errors.New("unsupported release tool")
	}
	// #nosec G204 -- Only the two allowlisted build tools; args are structured tool arguments, never shell source.
	cmd := exec.Command(command, args...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		// Tool diagnostics can include absolute module-cache paths. Keep public
		// build logs scoped to the command and failure, not the local machine.
		return nil, fmt.Errorf("%s failed: %w", command, err)
	}
	return output, nil
}

func readFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(filepath.Base(path))
}

func writeFile(path string, data []byte) error {
	// #nosec G301 G703 -- Explicit CLI output plus Go-validated module paths; public notices must be traversable by the image's nonroot UID.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.WriteFile(filepath.Base(path), data, 0o644)
}

func copyNotice(source, destination string) error {
	return copyFile(source, destination, true)
}

func copyFile(source, destination string, nonempty bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || nonempty && info.Size() == 0 {
		return errors.New("notice must be a nonempty regular file")
	}
	data, err := readFile(source)
	if err != nil {
		return err
	}
	if existing, err := readFile(destination); err == nil && !bytes.Equal(data, existing) {
		return errors.New("conflicting target notice bytes")
	}
	return writeFile(destination, data)
}

func collect(root, output, config string) error {
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return err
	}
	defer func() { _ = parent.Close() }()
	if _, err := parent.Lstat(filepath.Base(output)); !os.IsNotExist(err) {
		return errors.New("license destination already exists or is inaccessible")
	}
	env := os.Environ()
	info, err := run(env, "go", "version", "-m", mustLookPath("go-licenses"))
	if err != nil || !bytes.Contains(info, []byte("github.com/google/go-licenses/v2\tv2.0.1")) {
		return errors.New("verified go-licenses v2.0.1 is required")
	}
	ports, err := run(env, "go", "tool", "dist", "list")
	if err != nil {
		return err
	}
	supported := map[string]bool{}
	for _, port := range strings.Fields(string(ports)) {
		supported[port] = true
	}
	data, err := readFile(config)
	if err != nil {
		return err
	}
	platforms, err := targets(data, supported)
	if err != nil {
		return err
	}
	if _, err := run(env, "go", "mod", "verify"); err != nil {
		return err
	}
	own, err := run(env, "go", "list", "-m")
	if err != nil {
		return err
	}
	ownModule := strings.TrimSpace(string(own))
	if err := parent.Mkdir(filepath.Base(output), 0o755); err != nil {
		return err
	}
	outputRoot, err := parent.OpenRoot(filepath.Base(output))
	if err != nil {
		return err
	}
	defer func() { _ = outputRoot.Close() }()
	graphs := map[string]map[string]string{}
	for _, platform := range platforms {
		parts := strings.Split(platform, "/")
		targetEnv := append(append([]string{}, env...), "GOOS="+parts[0], "GOARCH="+parts[1], "CGO_ENABLED=0",
			"GOAMD64=v1", "GOARM=6", "GOARM64=v8.0")
		packages, err := run(targetEnv, "go", "list", "-deps", "-json", ".")
		if err != nil {
			return err
		}
		modules := map[string]module{}
		decoder := json.NewDecoder(bytes.NewReader(packages))
		for {
			var pkg struct{ Module *module }
			if err := decoder.Decode(&pkg); err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			if pkg.Module != nil && !pkg.Module.Main {
				if strings.HasPrefix(pkg.Module.Path, ownModule) {
					return errors.New("dependency collides with application license exclusion")
				}
				modules[pkg.Module.Path] = *pkg.Module
			}
		}
		correction, err := noticeCorrection(root, output, modules)
		if err != nil {
			return err
		}
		ignore := ownModule
		if correction != "" {
			ignore += "," + correction
		}
		if _, err := run(targetEnv, "go-licenses", "check", ".", "--ignore", ignore, "--disallowed_types=unknown"); err != nil {
			return fmt.Errorf("license classification for %s: %w", platform, err)
		}
		temporary, err := os.MkdirTemp("", "release-notices-")
		if err != nil {
			return err
		}
		if _, err := run(targetEnv, "go-licenses", "save", ".", "--ignore", ignore, "--save_path", filepath.Join(temporary, "licenses")); err != nil {
			return errors.Join(fmt.Errorf("license collection for %s: %w", platform, err), os.RemoveAll(temporary))
		}
		copyErr := filepath.WalkDir(filepath.Join(temporary, "licenses"), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			relative, err := filepath.Rel(filepath.Join(temporary, "licenses"), path)
			if err != nil {
				return err
			}
			// go-licenses also saves required source for reciprocal licenses.
			// Preserve empty source placeholders, but never empty legal notices.
			name := strings.ToUpper(entry.Name())
			legal := strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "LICENCE") ||
				strings.HasPrefix(name, "COPYING") || strings.HasPrefix(name, "NOTICE") || strings.HasPrefix(name, "PATENTS")
			return copyFile(path, filepath.Join(output, relative), legal)
		})
		if err := errors.Join(copyErr, os.RemoveAll(temporary)); err != nil {
			return err
		}
		graph := map[string]string{}
		for _, m := range modules {
			graph[m.Path] = m.Version
			entries, err := os.ReadDir(m.Dir)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				name := strings.ToUpper(entry.Name())
				if strings.HasPrefix(name, "PATENTS") || strings.HasPrefix(name, "NOTICE") {
					if err := copyNotice(filepath.Join(m.Dir, entry.Name()), filepath.Join(output, m.Path, entry.Name())); err != nil {
						return err
					}
				}
			}
		}
		graphs[platform] = graph
	}
	goroot, err := run(env, "go", "env", "GOROOT")
	if err != nil {
		return err
	}
	for _, name := range []string{"LICENSE", "PATENTS"} {
		if err := copyNotice(filepath.Join(strings.TrimSpace(string(goroot)), name), filepath.Join(output, "go", name)); err != nil {
			return err
		}
	}
	if err := copyNotice(filepath.Join(root, "LICENSE"), filepath.Join(output, ownModule, "LICENSE")); err != nil {
		return err
	}
	graphJSON, err := json.MarshalIndent(graphs, "", "  ")
	if err != nil {
		return err
	}
	if err := outputRoot.WriteFile("modules.json", append(graphJSON, '\n'), 0o644); err != nil {
		return err
	}
	checksums := map[string]string{}
	if err := fs.WalkDir(outputRoot.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := outputRoot.ReadFile(path)
		if err != nil {
			return errors.New("missing collected distribution file")
		}
		digest := sha256.Sum256(data)
		checksums[filepath.ToSlash(path)] = hex.EncodeToString(digest[:])
		return nil
	}); err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(checksums, "", "  ")
	if err != nil {
		return err
	}
	if err := outputRoot.WriteFile("notices.sha256.json", append(manifest, '\n'), 0o644); err != nil {
		return err
	}
	return fs.WalkDir(outputRoot.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return outputRoot.Chtimes(path, time.Unix(0, 0), time.Unix(0, 0))
	})
}

func mustLookPath(name string) string {
	path, _ := exec.LookPath(name)
	return path
}

func resetNotices(root, output string) error {
	absolute, err := filepath.Abs(output)
	if err != nil || absolute != filepath.Join(root, "licenses") {
		return errors.New("only the generated repository licenses directory can be replaced")
	}
	parent, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = parent.Close() }()
	info, err := parent.Lstat("licenses")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("existing notice root is unsafe")
	}
	generated, err := parent.OpenRoot("licenses")
	if err != nil {
		return err
	}
	defer func() { _ = generated.Close() }()
	data, err := generated.ReadFile("notices.sha256.json")
	if err != nil {
		return errors.New("existing notices lack their generated manifest")
	}
	var checksums map[string]string
	if err := json.Unmarshal(data, &checksums); err != nil || len(checksums) == 0 {
		return errors.New("existing notice manifest is invalid")
	}
	allowed := map[string]bool{".": true, "notices.sha256.json": true}
	for name, digest := range checksums {
		if filepath.IsAbs(name) || filepath.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") {
			return errors.New("existing notice manifest has an unsafe path")
		}
		data, err := generated.ReadFile(name)
		if err != nil {
			return errors.New("existing generated notice is missing")
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != digest {
			return errors.New("existing generated notice was modified")
		}
		allowed[name] = true
		for directory := filepath.Dir(name); directory != "."; directory = filepath.Dir(directory) {
			allowed[directory] = true
		}
	}
	if err := fs.WalkDir(generated.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !allowed[path] || entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return errors.New("existing notice tree contains untracked or unsafe content")
		}
		return nil
	}); err != nil {
		return err
	}
	if err := generated.Close(); err != nil {
		return err
	}
	return parent.RemoveAll("licenses")
}

func main() {
	config := ".goreleaser.yaml"
	if _, err := os.Stat(config); os.IsNotExist(err) {
		config = ".goreleaser.yml"
	}
	root, err := os.Getwd()
	if err == nil && len(os.Args) == 3 && os.Args[1] == "--replace-generated" {
		err = resetNotices(root, os.Args[2])
		if err == nil {
			err = collect(root, os.Args[2], config)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err == nil && len(os.Args) == 2 {
		err = collect(root, os.Args[1], config)
	} else {
		err = errors.New("usage: licensecollector NEW_OUTPUT_DIRECTORY")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
