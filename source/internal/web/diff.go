package web

import (
	"regexp"
	"strconv"
	"strings"
)

type diffLine struct{ Kind, Old, New, Text string }

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)`)

// parseDiff presents unified diff as escaped lines with independent old/new numbers.
func parseDiff(diff string) []diffLine {
	var result []diffLine
	oldLine, newLine := 0, 0
	inHunk := false
	for _, line := range strings.Split(strings.TrimSuffix(diff, "\n"), "\n") {
		if line == "" && diff == "" {
			break
		}
		if !inHunk && (strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ")) {
			continue
		}
		item := diffLine{Kind: "meta", Text: line}
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
			item.Kind = "file"
			if idx := strings.LastIndex(line, " b/"); idx >= 0 {
				item.Text = line[idx+3:]
			}
		case strings.HasPrefix(line, "@@"):
			m := hunkHeader.FindStringSubmatch(line)
			if m != nil {
				oldLine, _ = strconv.Atoi(m[1])
				newLine, _ = strconv.Atoi(m[2])
				inHunk = true
			}
			item.Kind = "hunk"
		case inHunk && strings.HasPrefix(line, "+"):
			item.Kind = "add"
			item.New = strconv.Itoa(newLine)
			newLine++
		case inHunk && strings.HasPrefix(line, "-"):
			item.Kind = "remove"
			item.Old = strconv.Itoa(oldLine)
			oldLine++
		case inHunk && strings.HasPrefix(line, " "):
			item.Kind = "context"
			item.Old = strconv.Itoa(oldLine)
			item.New = strconv.Itoa(newLine)
			oldLine++
			newLine++
		}
		result = append(result, item)
	}
	return result
}
