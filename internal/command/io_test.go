package command

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptTextAndEnterToContinue_ok_newline(t *testing.T) {
	buff := strings.NewReader("\n")
	require.NoError(t, PromptTextAndEnterToContinue(t.Context(), buff, ""))
}

func TestPromptTextAndEnterToContinue_ok_text(t *testing.T) {
	buff := strings.NewReader("hello world\n")
	require.NoError(t, PromptTextAndEnterToContinue(t.Context(), buff, "hello world"))
}

func TestPromptTextAndEnterToContinue_bad_text(t *testing.T) {
	buff := strings.NewReader("baaaaad\n")
	require.EqualError(t, PromptTextAndEnterToContinue(t.Context(), buff, "hello world"), "context canceled")
}

func TestPromptTextAndEnterToContinue_eof(t *testing.T) {
	buff := strings.NewReader("")
	require.EqualError(t, PromptTextAndEnterToContinue(t.Context(), buff, ""), "context canceled")
}

func TestPromptYesNo_yes(t *testing.T) {
	buff := strings.NewReader("yes\n")
	confirmed, err := PromptYesNo(t.Context(), buff, "Test prompt")
	require.NoError(t, err)
	require.True(t, confirmed)
}

func TestPromptYesNo_y(t *testing.T) {
	buff := strings.NewReader("y\n")
	confirmed, err := PromptYesNo(t.Context(), buff, "Test prompt")
	require.NoError(t, err)
	require.True(t, confirmed)
}

func TestPromptYesNo_Y(t *testing.T) {
	buff := strings.NewReader("Y\n")
	confirmed, err := PromptYesNo(t.Context(), buff, "Test prompt")
	require.NoError(t, err)
	require.True(t, confirmed)
}

func TestPromptYesNo_no(t *testing.T) {
	buff := strings.NewReader("no\n")
	confirmed, err := PromptYesNo(t.Context(), buff, "Test prompt")
	require.NoError(t, err)
	require.False(t, confirmed)
}

func TestPromptYesNo_other_input(t *testing.T) {
	buff := strings.NewReader("maybe\n")
	confirmed, err := PromptYesNo(t.Context(), buff, "Test prompt")
	require.NoError(t, err)
	require.False(t, confirmed)
}

func TestPromptYesNo_empty(t *testing.T) {
	buff := strings.NewReader("\n")
	confirmed, err := PromptYesNo(t.Context(), buff, "Test prompt")
	require.NoError(t, err)
	require.False(t, confirmed)
}

func TestPromptYesNo_eof(t *testing.T) {
	buff := strings.NewReader("")
	_, err := PromptYesNo(t.Context(), buff, "Test prompt")
	require.Error(t, err)
}
