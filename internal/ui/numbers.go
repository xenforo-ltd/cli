package ui

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// formatCount follows the numeric locale, falling back to English grouping.
// The C and POSIX locales retain their ungrouped numeric output.
func formatCount(n int) string {
	var locale string
	for _, key := range []string{"LC_ALL", "LC_NUMERIC", "LANG"} {
		if locale = os.Getenv(key); locale != "" {
			break
		}
	}
	locale, _, _ = strings.Cut(locale, ".")
	locale, _, _ = strings.Cut(locale, "@")
	if locale == "C" || locale == "POSIX" {
		return strconv.Itoa(n)
	}

	tag := language.English
	if parsed, err := language.Parse(strings.ReplaceAll(locale, "_", "-")); locale != "" && err == nil {
		tag = parsed
	}
	return message.NewPrinter(tag).Sprintf("%d", n)
}
