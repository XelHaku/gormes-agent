package cli

import (
	"fmt"
	"io"
	"strings"
)

func FormatInfo(text string) string {
	return fmt.Sprintf("  %s\n", text)
}

func FormatSuccess(text string) string {
	return fmt.Sprintf("✓ %s\n", text)
}

func FormatWarning(text string) string {
	return fmt.Sprintf("⚠ %s\n", text)
}

func FormatError(text string) string {
	return fmt.Sprintf("✗ %s\n", text)
}

func FormatHeader(text string) string {
	return fmt.Sprintf("\n  %s\n", text)
}

func WriteInfo(w io.Writer, text string) error {
	_, err := io.WriteString(w, FormatInfo(text))
	return err
}

func WriteSuccess(w io.Writer, text string) error {
	_, err := io.WriteString(w, FormatSuccess(text))
	return err
}

func WriteWarning(w io.Writer, text string) error {
	_, err := io.WriteString(w, FormatWarning(text))
	return err
}

func WriteError(w io.Writer, text string) error {
	_, err := io.WriteString(w, FormatError(text))
	return err
}

func WriteHeader(w io.Writer, text string) error {
	_, err := io.WriteString(w, FormatHeader(text))
	return err
}

func FormatPrompt(question string, defaultValue string) string {
	suffix := ""
	if defaultValue != "" {
		suffix = fmt.Sprintf(" [%s]", defaultValue)
	}
	return fmt.Sprintf("  %s%s: ", question, suffix)
}

func ResolvePromptInput(input string, defaultValue string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return defaultValue
	}
	return value
}

func FormatYesNoPrompt(question string, defaultAnswer bool) string {
	hint := "y/N"
	if defaultAnswer {
		hint = "Y/n"
	}
	return FormatPrompt(fmt.Sprintf("%s (%s)", question, hint), "")
}

func ResolveYesNoAnswer(answer string, defaultAnswer bool) bool {
	value := ResolvePromptInput(answer, "")
	if value == "" {
		return defaultAnswer
	}
	return strings.HasPrefix(strings.ToLower(value), "y")
}
