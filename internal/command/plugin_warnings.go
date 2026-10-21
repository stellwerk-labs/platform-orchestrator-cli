package command

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/fatih/color"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

const pluginWarningsContextKey contextKey = "plugin-warnings"

// pluginWarnings prints a plugin licence grace warning at most once per
// command run, however many discovery and invoke responses repeat it.
type pluginWarnings struct {
	out  io.Writer
	once sync.Once
}

func withPluginWarnings(ctx context.Context, out io.Writer) context.Context {
	return context.WithValue(ctx, pluginWarningsContextKey, &pluginWarnings{out: out})
}

// warnPluginEntitlementGrace shows Core's grace warning. It never changes the
// outcome: while a feature is in grace, forward work still succeeds.
func warnPluginEntitlementGrace(ctx context.Context, warning *cp.PluginEntitlementWarning) {
	if warning == nil {
		return
	}
	sink, ok := ctx.Value(pluginWarningsContextKey).(*pluginWarnings)
	if !ok {
		sink = &pluginWarnings{out: color.Output}
	}
	sink.once.Do(func() {
		_, _ = fmt.Fprintf(sink.out, "Warning: %s (%s, grace until %s)\n", warning.Message, warning.Code, warning.GraceUntil.UTC().Format(time.RFC3339))
	})
}
