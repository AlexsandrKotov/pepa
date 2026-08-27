package ai

import (
	"regexp"
	"strings"
)

var thinkRe = regexp.MustCompile(`(?s)<think>\s*.*?\s*
</think>

`)
var thinkRe2 = regexp.MustCompile(`(?s)<think>\s*.*?\s*
</think>

`)

func StripThinkBlocks(s string) string {
	if !strings.Contains(s, "<think>") {
		return s
	}
	s = thinkRe.ReplaceAllString(s, "")
	s = thinkRe2.ReplaceAllString(s, "")
	if idx := strings.Index(s, "<think>"); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}
