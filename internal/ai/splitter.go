package ai

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MetaSeparator = "===META==="
	UserSeparator = "===USER==="
	// Hold back enough trailing bytes so `\r\n===META===\r\n` never leaks across chunk boundaries.
	splitterHoldback = len("\r\n" + MetaSeparator + "  \r\n")
)

var PromptVariableRegex = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

type Assumption struct {
	Label        string   `json:"label"`
	Value        string   `json:"value"`
	Alternatives []string `json:"alternatives,omitempty"`
	Impact       string   `json:"impact,omitempty"`
	Question     string   `json:"question,omitempty"`
}

type PromptMeta struct {
	Intent      string       `json:"intent"`
	Assumptions []Assumption `json:"assumptions,omitempty"`
}

type SplitResult struct {
	Prompt      string
	Meta        PromptMeta
	MetaPresent bool
	MetaValid   bool
}

// StreamSplitter streams plain-text Markdown prompt deltas in real time until it
// encounters `===META===`, holding back a small tail buffer so split delimiters
// never leak into the visible prompt output.
type StreamSplitter struct {
	onDelta    func(string)
	buf        string
	promptFull strings.Builder
	metaBuf    strings.Builder
	inMeta     bool
}

func NewStreamSplitter(onDelta func(string)) *StreamSplitter {
	return &StreamSplitter{onDelta: onDelta}
}

func (s *StreamSplitter) Feed(chunk string) {
	if chunk == "" {
		return
	}
	if s.inMeta {
		s.metaBuf.WriteString(chunk)
		return
	}

	s.buf += chunk
	if idx, afterIdx := s.findMetaLine(false); idx >= 0 {
		before := strings.TrimRight(s.buf[:idx], "\r\n")
		if before != "" {
			s.emitPrompt(before)
		}
		after := s.buf[afterIdx:]
		s.buf = ""
		s.inMeta = true
		s.metaBuf.WriteString(after)
		return
	}

	if len(s.buf) > splitterHoldback {
		safeEnd := len(s.buf) - splitterHoldback
		// Step back if safeEnd splits a multi-byte UTF-8 rune
		for safeEnd > 0 && !utf8.ValidString(s.buf[:safeEnd]) {
			safeEnd--
		}
		if safeEnd > 0 {
			s.emitPrompt(s.buf[:safeEnd])
			s.buf = s.buf[safeEnd:]
		}
	}
}

func (s *StreamSplitter) findMetaLine(atEOF bool) (int, int) {
	searchFrom := 0
	for searchFrom < len(s.buf) {
		rel := strings.Index(s.buf[searchFrom:], MetaSeparator)
		if rel < 0 {
			return -1, -1
		}
		idx := searchFrom + rel
		lineStartOk := (idx == 0 && s.promptFull.Len() == 0) || (idx > 0 && s.buf[idx-1] == '\n')
		afterStart := idx + len(MetaSeparator)
		if lineStartOk {
			j := afterStart
			for j < len(s.buf) && (s.buf[j] == ' ' || s.buf[j] == '\t') {
				j++
			}
			if j < len(s.buf) && (s.buf[j] == '\n' || s.buf[j] == '\r') {
				return idx, j
			}
			if atEOF && (j == len(s.buf) || s.buf[j] == '{') {
				return idx, j
			}
			if !atEOF && j == len(s.buf) {
				// Potential standalone ===META=== waiting for trailing newline in next chunk
				return -1, -1
			}
		}
		searchFrom = afterStart
	}
	return -1, -1
}

func (s *StreamSplitter) emitPrompt(delta string) {
	if delta == "" {
		return
	}
	s.promptFull.WriteString(delta)
	if s.onDelta != nil {
		s.onDelta(delta)
	}
}

func (s *StreamSplitter) CurrentPrompt() string {
	return strings.TrimSpace(s.promptFull.String() + s.buf)
}

func (s *StreamSplitter) Finalize() SplitResult {
	if !s.inMeta && s.buf != "" {
		if idx, afterIdx := s.findMetaLine(true); idx >= 0 {
			before := strings.TrimRight(s.buf[:idx], "\r\n")
			if before != "" {
				s.emitPrompt(before)
			}
			s.inMeta = true
			s.metaBuf.WriteString(s.buf[afterIdx:])
		} else {
			s.emitPrompt(s.buf)
		}
		s.buf = ""
	}

	promptText := strings.TrimSpace(s.promptFull.String())
	res := SplitResult{
		Prompt:      promptText,
		MetaPresent: s.inMeta,
	}
	if !s.inMeta {
		return res
	}

	rawMeta := cleanJSONFence(s.metaBuf.String())
	var parsed PromptMeta
	if err := json.Unmarshal([]byte(rawMeta), &parsed); err != nil {
		// Graceful degradation per PLAN.md §3 & CLAUDE.md: keep prompt even if meta is broken
		res.MetaValid = false
		return res
	}

	parsed.Intent = strings.TrimSpace(parsed.Intent)
	var highItems []Assumption
	var normalItems []Assumption
	for _, a := range parsed.Assumptions {
		lbl := strings.TrimSpace(a.Label)
		val := strings.TrimSpace(a.Value)
		if lbl == "" || val == "" {
			continue
		}
		var alts []string
		seenAlt := map[string]struct{}{val: {}}
		for _, alt := range a.Alternatives {
			t := strings.TrimSpace(alt)
			if t == "" {
				continue
			}
			if _, exists := seenAlt[t]; exists {
				continue
			}
			seenAlt[t] = struct{}{}
			alts = append(alts, t)
			if len(alts) >= 3 {
				break
			}
		}
		impact := normalizeAssumptionImpact(a.Impact)
		question := truncateRunes(strings.TrimSpace(a.Question), 180)
		item := Assumption{
			Label:        lbl,
			Value:        val,
			Alternatives: alts,
			Impact:       impact,
			Question:     question,
		}
		if impact == "high" && len(highItems) < 3 {
			highItems = append(highItems, item)
		} else {
			item.Impact = ""
			item.Question = ""
			normalItems = append(normalItems, item)
		}
	}
	cleaned := append(highItems, normalItems...)
	if len(cleaned) > 4 {
		cleaned = cleaned[:4]
	}
	parsed.Assumptions = cleaned
	res.Meta = parsed
	res.MetaValid = true
	return res
}

func normalizeAssumptionImpact(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "high", "critical":
		return "high"
	default:
		return ""
	}
}

func truncateRunes(s string, maxRunes int) string {
	if s == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

func cleanJSONFence(raw string) string {
	t := strings.TrimSpace(raw)
	if strings.HasPrefix(t, "```") {
		if nl := strings.IndexByte(t, '\n'); nl >= 0 {
			t = t[nl+1:]
		}
		if end := strings.LastIndex(t, "```"); end >= 0 {
			t = t[:end]
		}
	}
	return strings.TrimSpace(t)
}

// SplitAPIParts splits a target="api" prompt on `===USER===` into system and user blocks.
func SplitAPIParts(content string) (systemPrompt string, userTemplate string, hasSplit bool) {
	idx := strings.Index(content, UserSeparator)
	if idx < 0 {
		return strings.TrimSpace(content), "", false
	}
	sys := strings.TrimSpace(content[:idx])
	usr := strings.TrimSpace(content[idx+len(UserSeparator):])
	return sys, usr, true
}

// ExtractVariables returns distinct variable names matching `{{ten_bien}}` in order of appearance.
func ExtractVariables(content string) []string {
	matches := PromptVariableRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	var out []string
	for _, m := range matches {
		if len(m) > 1 {
			name := m[1]
			if _, exists := seen[name]; !exists {
				seen[name] = struct{}{}
				out = append(out, name)
			}
		}
	}
	return out
}

// FillVariables substitutes `{{ten_bien}}` occurrences using the provided values map.
func FillVariables(content string, values map[string]string) string {
	if len(values) == 0 {
		return content
	}
	return PromptVariableRegex.ReplaceAllStringFunc(content, func(match string) string {
		sub := PromptVariableRegex.FindStringSubmatch(match)
		if len(sub) > 1 {
			if val, ok := values[sub[1]]; ok && strings.TrimSpace(val) != "" {
				return val
			}
		}
		return match
	})
}
