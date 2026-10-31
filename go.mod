module github.com/stellwerk-labs/platform-orchestrator-cli

go 1.26.0

require (
	filippo.io/age v1.2.1
	github.com/fatih/color v1.18.0
	github.com/getkin/kin-openapi v0.152.0
	github.com/google/uuid v1.6.0
	github.com/gosuri/uitable v0.0.4
	github.com/justinrixx/retryhttp v1.0.1
	github.com/mattn/go-isatty v0.0.20
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/oapi-codegen/nullable v1.2.0
	github.com/oapi-codegen/runtime v1.7.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/pkg/errors v0.9.1
	github.com/score-spec/score-go v1.11.0
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6
	github.com/stretchr/testify v1.11.1
	github.com/wI2L/jsondiff v0.7.0
	go.uber.org/mock v0.5.1
	golang.org/x/mod v0.41.0
	gopkg.in/yaml.v3 v3.0.1
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/bitfield/gotestdox v0.2.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dnephin/pflag v1.0.7 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/swag/jsonname v0.26.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/oapi-codegen/oapi-codegen/v2 v2.8.0 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/speakeasy-api/jsonpath v0.6.3 // indirect
	github.com/speakeasy-api/openapi v1.24.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	gotest.tools/gotestsum v1.12.1 // indirect
)

tool (
	github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
	go.uber.org/mock/mockgen
	gotest.tools/gotestsum
)

// Retain the patched transitive graph version for GHSA-c9gr-gmg4-2vqv.
replace github.com/andybalholm/brotli => github.com/andybalholm/brotli v1.2.7
