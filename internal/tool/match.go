package tool

import "strings"

type span struct{ start, end int }

// Exact wins; failing that the two differences that are never meaningful —
// typographic punctuation and trailing whitespace — are forgiven.
func findSpans(content, old string) ([]span, string) {
	if spans := exactSpans(content, old); len(spans) > 0 {
		return spans, ""
	}
	if spans := normalizedSpans(content, old); len(spans) > 0 {
		return spans, "matched after normalising quotes and dashes"
	}
	if spans := looseLineSpans(content, old); len(spans) > 0 {
		return spans, "matched ignoring trailing whitespace"
	}
	return nil, ""
}

func exactSpans(content, old string) []span {
	var spans []span
	for at := 0; ; {
		i := strings.Index(content[at:], old)
		if i < 0 {
			return spans
		}
		start := at + i
		spans = append(spans, span{start, start + len(old)})
		at = start + len(old)
	}
}

func normalizedSpans(content, old string) []span {
	runes := []rune(content)
	offsets := make([]int, len(runes)+1)
	at := 0
	for i, r := range runes {
		offsets[i] = at
		at += len(string(r))
	}
	offsets[len(runes)] = len(content)

	haystack := normalize(runes)
	needle := normalize([]rune(old))
	if needle == "" {
		return nil
	}

	var spans []span
	hayRunes := []rune(haystack)
	needleRunes := []rune(needle)
	for i := 0; i+len(needleRunes) <= len(hayRunes); i++ {
		if string(hayRunes[i:i+len(needleRunes)]) == needle {
			spans = append(spans, span{offsets[i], offsets[i+len(needleRunes)]})
			i += len(needleRunes) - 1
		}
	}
	return spans
}

func normalize(runes []rune) string {
	var b strings.Builder
	b.Grow(len(runes))
	for _, r := range runes {
		switch r {
		case '‘', '’', '‚', '‛':
			b.WriteRune('\'')
		case '“', '”', '„', '‟':
			b.WriteRune('"')
		case '‒', '–', '—', '―', '−':
			b.WriteRune('-')
		case ' ', ' ', ' ':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func looseLineSpans(content, old string) []span {
	oldLines := strings.Split(strings.TrimSuffix(old, "\n"), "\n")
	if len(oldLines) == 0 {
		return nil
	}

	lines, starts := linesWithOffsets(content)
	if len(oldLines) > len(lines) {
		return nil
	}

	var spans []span
	for i := 0; i+len(oldLines) <= len(lines); i++ {
		match := true
		for j, want := range oldLines {
			if trimRight(lines[i+j]) != trimRight(want) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		last := i + len(oldLines) - 1
		spans = append(spans, span{starts[i], starts[last] + len(lines[last])})
		i = last
	}
	return spans
}

func linesWithOffsets(content string) ([]string, []int) {
	var lines []string
	var starts []int
	at := 0
	for at <= len(content) {
		end := strings.IndexByte(content[at:], '\n')
		if end < 0 {
			lines = append(lines, content[at:])
			starts = append(starts, at)
			break
		}
		lines = append(lines, content[at:at+end])
		starts = append(starts, at)
		at += end + 1
	}
	return lines, starts
}

func trimRight(s string) string { return strings.TrimRight(s, " \t\r") }
