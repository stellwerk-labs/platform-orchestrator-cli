package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const (
	createUpdateCmdSetFlag     = "set"
	createUpdateCmdSetJsonFlag = "set-json"
	createUpdateCmdSetYamlFlag = "set-yaml"
)

var CreateCmd = &cobra.Command{
	GroupID:       CrudGroup.ID,
	Use:           "create <type>",
	Short:         "Create an object of a given type",
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		out, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), out, []string{printer.JsonPrinterType, printer.YamlPrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)
		return nil
	},
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

func readSetJsonFlag(cmd *cobra.Command, v string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if v == "-" {
		slog.Debug("reading JSON from stdin")
		dec := json.NewDecoder(cmd.InOrStdin())
		if err := dec.Decode(&out); err != nil {
			return nil, errors.Wrap(err, "failed to decode JSON from stdin")
		}
		return out, nil
	} else if strings.HasPrefix(v, "@") {
		slog.Debug("reading JSON from file", slog.String("file", v[1:]))
		if f, err := os.ReadFile(v[1:]); err != nil {
			return nil, errors.Wrapf(err, "failed to read file '%s'", v[1:])
		} else if err := json.Unmarshal(f, &out); err != nil {
			return nil, errors.Wrap(err, "failed to decode JSON from file")
		}
		return out, nil
	}
	slog.Debug("reading JSON from string")
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, errors.Wrap(err, "failed to decode JSON from string")
	}
	return out, nil
}

func readSetYamlFlag(cmd *cobra.Command, v string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if v == "-" {
		slog.Debug("reading JSON from stdin")
		dec := yaml.NewDecoder(cmd.InOrStdin())
		if err := dec.Decode(&out); err != nil {
			return nil, errors.Wrap(err, "failed to decode YAML from stdin")
		}
		return out, nil
	} else if strings.HasPrefix(v, "@") {
		slog.Debug("reading YAML from file", slog.String("file", v[1:]))
		if f, err := os.ReadFile(v[1:]); err != nil {
			return nil, errors.Wrapf(err, "failed to read file '%s'", v[1:])
		} else if err := yaml.Unmarshal(f, &out); err != nil {
			return nil, errors.Wrap(err, "failed to decode YAML from file")
		}
		return out, nil
	}
	slog.Debug("reading YAML from string")
	if err := yaml.Unmarshal([]byte(v), &out); err != nil {
		return nil, errors.Wrap(err, "failed to decode YAML from string")
	}
	return out, nil
}

func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, i2 := range s {
		if !unicode.IsDigit(i2) {
			return false
		}
	}
	return true
}

func readSetFlag(cmd *cobra.Command, v []string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for _, s := range v {
		if i := strings.Index(s, "="); i < 0 {
			return nil, errors.Errorf("invalid set flag '%s'", s)
		} else {
			prefix, suffix := s[:i], s[i+1:]
			out[prefix] = suffix
			if strings.HasPrefix(suffix, "[") || strings.HasPrefix(suffix, "{") || suffix == stringTrue || suffix == stringFalse || suffix == stringNull || isNumeric(suffix) {
				var v2 interface{}
				if err := json.Unmarshal([]byte(suffix), &v2); err == nil {
					out[prefix] = v2
				}
			}
		}
	}
	return out, nil
}

func readSetFlagsIntoType[T any](cmd *cobra.Command) (*T, error) {
	intermediate := map[string]interface{}{}
	if v, _ := cmd.Flags().GetString(createUpdateCmdSetJsonFlag); v != "" {
		if m, err := readSetJsonFlag(cmd, v); err != nil {
			return nil, err
		} else {
			intermediate = m
		}
	} else if v, _ := cmd.Flags().GetString(createUpdateCmdSetYamlFlag); v != "" {
		if m, err := readSetYamlFlag(cmd, v); err != nil {
			return nil, err
		} else {
			intermediate = m
		}
	}
	if v, _ := cmd.Flags().GetStringArray(createUpdateCmdSetFlag); len(v) > 0 {
		if m, err := readSetFlag(cmd, v); err != nil {
			return nil, err
		} else {
			for k, v := range m {
				intermediate[k] = v
			}
		}
	}
	slog.Debug("read input fields", slog.Any("input", intermediate))
	var typed T
	raw, _ := json.Marshal(intermediate)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&typed); err != nil {
		return nil, errors.Wrap(err, "failed to map input fields")
	}
	return &typed, nil
}

func init() {
	CreateCmd.PersistentFlags().String(createUpdateCmdSetJsonFlag, "", "Set JSON input as either a raw string '{..}', stdin '-', or a @-prefixed json file path")
	CreateCmd.PersistentFlags().String(createUpdateCmdSetYamlFlag, "", "Set YAML input as either a raw string '{..}', stdin '-', or a @-prefixed yaml file path")
	CreateCmd.PersistentFlags().StringArray(createUpdateCmdSetFlag, []string{}, "Set key=value pairs")
	printer.SetupSingleOutputFormatFlag(CreateCmd.PersistentFlags())
	CreateCmd.MarkFlagsMutuallyExclusive(createUpdateCmdSetYamlFlag, createUpdateCmdSetJsonFlag)
}

func generateTopLevelSetFields(bod interface{}) string {
	t := reflect.TypeOf(bod)
	fields := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if jsonTag := f.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			jsonName := strings.Split(jsonTag, ",")[0]
			typ := f.Type
			if typ.Kind() == reflect.Pointer {
				typ = f.Type.Elem()
			}

			var typStr = typ.String()
			if typ.Kind() == reflect.Struct || typ.Kind() == reflect.Map || typ.Kind() == reflect.Interface {
				typStr = "map"
			} else if typ.Kind() == reflect.Slice {
				typStr = "list"
			} else if typStr != "string" && typ.ConvertibleTo(reflect.TypeOf("string")) {
				typStr = "string"
			}
			fields = append(fields, fmt.Sprintf("%s (%s)", jsonName, typStr))
		}
	}
	return strings.Join(fields, ", ")
}
