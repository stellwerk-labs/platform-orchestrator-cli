package printer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

const maxRunnerLogRecordBytes = 64 * 1024

// RunnerLogsTerminal checks the destination, rather than stdin or color.Output.
func RunnerLogsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
}

// RunnerLogsColor leaves readable formatting enabled when ANSI is disabled.
func RunnerLogsColor(w io.Writer) bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	return RunnerLogsTerminal(w) && !noColor && os.Getenv("TERM") != "dumb"
}

// FormatRunnerLogs formats complete, decrypted logs. Callers retain the original
// bytes for encrypted, raw and machine output. Unknown payload fields are kept.
func FormatRunnerLogs(body []byte, colorful bool) []byte {
	var out strings.Builder
	first := true
	for line := range strings.SplitSeq(string(body), "\n") {
		if !first {
			out.WriteByte('\n')
		}
		first = false
		out.WriteString(formatRunnerLogLine(line, colorful))
	}
	return []byte(out.String())
}

func formatRunnerLogLine(line string, colorful bool) string {
	if len(line) > maxRunnerLogRecordBytes || !json.Valid([]byte(line)) || runnerLogExceedsLayoutBudget(line) {
		return safeRunnerLogText(line)
	}
	fields := make(map[string]json.RawMessage)
	if json.Unmarshal([]byte(line), &fields) == nil && fields != nil {
		message, ui := runnerLogString(fields["@message"])
		_, uiLevel := runnerLogString(fields["@level"])
		_, slogTime := runnerLogString(fields["time"])
		_, slogLevel := runnerLogString(fields["level"])
		if ui && uiLevel {
			return formatRunnerLogRecord(fields, message, "@message", []string{"@timestamp", "@level", "@module", "type"}, colorful)
		}
		if message, ok := runnerLogString(fields["msg"]); ok && slogTime && slogLevel {
			return formatRunnerLogRecord(fields, message, "msg", []string{"time", "level"}, colorful)
		}
	}
	return runnerLogJSON([]byte(line), colorful)
}

func runnerLogString(value json.RawMessage) (string, bool) {
	var text string
	if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &text) != nil {
		return "", false
	}
	return text, true
}

func formatRunnerLogRecord(fields map[string]json.RawMessage, message, messageKey string, metadataKeys []string, colorful bool) string {
	metadata := make([]string, 0, len(metadataKeys))
	delete(fields, messageKey)
	for _, key := range metadataKeys {
		if value, ok := runnerLogString(fields[key]); ok {
			metadata = append(metadata, safeRunnerLogText(value))
			delete(fields, key)
		}
	}
	header := strings.Join(metadata, " · ")
	if colorful {
		header = runnerLogColor(header, color.FgCyan)
	}
	text := header + "\n" + safeRunnerLogText(message)
	if len(fields) != 0 {
		payload, _ := json.Marshal(fields)
		text += "\n" + runnerLogJSON(payload, colorful)
	}
	return text
}

func runnerLogJSON(value []byte, colorful bool) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, value, "", "  "); err != nil {
		return safeRunnerLogText(string(value))
	}
	if pretty.Len() > maxRunnerLogRecordBytes {
		return safeRunnerLogText(string(value))
	}
	text := safeRunnerLogText(pretty.String())
	if !colorful || len(text) > maxRunnerLogRecordBytes {
		return text
	}
	// Decoder offsets identify JSON tokens without a second syntax grammar.
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var highlighted strings.Builder
	start := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			highlighted.WriteString(text[start:])
			break
		}
		end := int(decoder.InputOffset())
		fragment := text[start:end]
		switch token.(type) {
		case string:
			fragment = runnerLogColor(fragment, color.FgGreen)
		case json.Number, bool, nil:
			fragment = runnerLogColor(fragment, color.FgYellow)
		}
		highlighted.WriteString(fragment)
		start = end
	}
	return highlighted.String()
}

// Decoding JSON can turn escaped terminal controls into active commands. Make
// every untrusted human-visible field inert before adding our own ANSI colors.
func safeRunnerLogText(text string) string {
	var safe strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		switch {
		case r == utf8.RuneError && size == 1:
			_, _ = fmt.Fprintf(&safe, "\\x%02x", text[0])
		case unicode.IsControl(r) && r != '\n' && r != '\t':
			escaped := strconv.QuoteRuneToASCII(r)
			safe.WriteString(escaped[1 : len(escaped)-1])
		default:
			safe.WriteRune(r)
		}
		text = text[size:]
	}
	return safe.String()
}

// Limit indentation growth for hostile nested JSON while retaining the line.
func runnerLogExceedsLayoutBudget(line string) bool {
	decoder := json.NewDecoder(strings.NewReader(line))
	depth := 0
	prettySize := len(line)
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
		// Conservatively allow a separator, newline and indentation per token.
		prettySize += 2 + depth*2
		if depth > 64 || prettySize > maxRunnerLogRecordBytes {
			return true
		}
	}
}

func runnerLogColor(text string, attribute color.Attribute) string {
	style := color.New(attribute)
	style.EnableColor()
	return style.Sprint(text)
}
