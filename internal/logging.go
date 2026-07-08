package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"strings"
	"time"
)

func SetupLogging(debug bool, stderr io.Writer) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	handler := &slogWriter{Output: stderr, Level: level}
	slog.SetDefault(slog.New(handler))
	log.SetOutput(handler)
}

type slogWriter struct {
	Output io.Writer
	Attrs  []slog.Attr
	Groups []string
	Debug  bool
	Level  slog.Level
}

func (s *slogWriter) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= s.Level
}

func (s *slogWriter) Handle(ctx context.Context, record slog.Record) error {
	sb := new(bytes.Buffer)
	sb.WriteString(record.Time.Format(time.DateTime))
	sb.WriteByte(' ')
	sb.WriteString(record.Level.String())
	sb.WriteByte(' ')
	if len(s.Groups) > 0 {
		sb.WriteByte('[')
		sb.WriteString(strings.Join(s.Groups, ","))
		sb.WriteString("] ")
	}
	sb.WriteString(strings.TrimSuffix(record.Message, "\n"))
	for _, attr := range s.Attrs {
		sb.WriteByte(' ')
		sb.WriteString(attr.Key)
		sb.WriteByte('=')
		if attr.Value.Kind() == slog.KindAny {
			_ = json.NewEncoder(sb).Encode(attr.Value.Any())
			sb.Truncate(sb.Len() - 1)
		} else {
			_, _ = fmt.Fprintf(sb, "%q", attr.Value.String())
		}
	}
	record.Attrs(func(attr slog.Attr) bool {
		sb.WriteByte(' ')
		sb.WriteString(attr.Key)
		sb.WriteByte('=')
		if attr.Value.Kind() == slog.KindAny {
			_ = json.NewEncoder(sb).Encode(attr.Value.Any())
			sb.Truncate(sb.Len() - 1)
		} else {
			_, _ = fmt.Fprintf(sb, "%q", attr.Value.String())
		}
		return true
	})
	sb.WriteByte('\n')
	_, err := s.Output.Write(sb.Bytes())
	return err
}

func (s *slogWriter) WithAttrs(attrs []slog.Attr) slog.Handler {
	sw := *s
	sw.Attrs = append(sw.Attrs, attrs...)
	return &sw
}

func (s *slogWriter) WithGroup(name string) slog.Handler {
	sw := *s
	sw.Groups = append(sw.Groups, name)
	return &sw
}

func (s *slogWriter) Write(p []byte) (n int, err error) {
	err = s.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, string(p), 0))
	return len(p), err
}
