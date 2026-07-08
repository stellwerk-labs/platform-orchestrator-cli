package command

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
)

const PromptTimeout = 300 * time.Second

// PromptTextAndEnterToContinue attempts to wait for the required text and an enter character on stdin.
// An interrupt will cancel the context or will interrupt the whole process, either way closing the goroutine.
func PromptTextAndEnterToContinue(ctx context.Context, stdin io.Reader, requiredText string) error {
	// We should check for TTY before this or disable the prompt.
	if isAFile, ok := stdin.(*os.File); ok && !isatty.IsTerminal(isAFile.Fd()) {
		return errors.New("attempted to prompt user to continue but stdin is not a terminal. please specify --no-prompt")
	}

	// Setup cancellation
	ctx, cancel := context.WithTimeout(ctx, PromptTimeout)
	defer cancel()

	// Wait in a goroutine and communicate back to the main process by cancellation and or channel.
	entered := make(chan struct{})
	go func() {
		reader := bufio.NewReader(stdin)
		if text, err := reader.ReadString('\n'); err != nil {
			failureMessageF("failed to read input: %v", err)
			cancel()
		} else if text = strings.TrimSpace(text); text != requiredText {
			failureMessageF("expected '%s', got '%s'", requiredText, text)
			cancel()
		}
		close(entered)
	}()

	// Wait and continue.
	var prefix string
	if requiredText != "" {
		prefix = "Type '" + requiredText + "' and then "
	}
	_, _ = fmt.Fprintf(color.Output, "%sEnter to continue within %s, or Ctrl+C to abort... ", prefix, PromptTimeout)
	select {
	case <-entered:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PromptYesNo prompts the user with a yes/no question and returns true if the user confirms with yes/y/Y.
// Returns false for any other input. An error is returned only on read failure or timeout.
func PromptYesNo(ctx context.Context, stdin io.Reader, prompt string) (bool, error) {
	// We should check for TTY before this or disable the prompt.
	if isAFile, ok := stdin.(*os.File); ok && !isatty.IsTerminal(isAFile.Fd()) {
		return false, errors.New("attempted to prompt user but stdin is not a terminal. please specify --no-prompt")
	}

	// Setup cancellation
	ctx, cancel := context.WithTimeout(ctx, PromptTimeout)
	defer cancel()

	// Channel to communicate the user's response
	type result struct {
		confirmed bool
		err       error
	}
	resultChan := make(chan result)

	go func() {
		reader := bufio.NewReader(stdin)
		text, err := reader.ReadString('\n')
		if err != nil {
			resultChan <- result{confirmed: false, err: errors.Wrap(err, "failed to read input")}
			return
		}
		text = strings.TrimSpace(text)
		confirmed := text == stringYes || text == stringY || text == strings.ToUpper(stringY)
		resultChan <- result{confirmed: confirmed, err: nil}
	}()

	// Display the prompt
	_, _ = fmt.Fprintf(color.Output, "%s (yes/y/Y) [timeout: %s, Ctrl+C to abort]: ", prompt, PromptTimeout)

	select {
	case res := <-resultChan:
		return res.confirmed, res.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func infoMessageF(format string, args ...any) {
	_, _ = fmt.Fprintf(color.Output, format+"\n", args...)
}

func successMessageF(format string, args ...any) {
	color.HiGreen(format, args...)
}

func changedMessageF(format string, args ...any) {
	color.HiYellow(format, args...)
}

func failureMessageF(format string, args ...any) {
	color.HiRed(format, args...)
}
