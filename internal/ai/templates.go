package ai

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

//go:embed templates/*.md templates/*.json templates/targets/*.md
var templateFS embed.FS

const (
	GenerateTemplateVersion = "generate.v2"
	RefineTemplateVersion   = "refine.v2"
)

type GenerateTemplateData struct {
	Request     string
	Language    string
	Target      string
	TargetGuide string
}

type RefineTemplateData struct {
	Request       string
	CurrentPrompt string
	Instruction   string
	Language      string
	Target        string
	TargetGuide   string
}

type StarterPreset struct {
	Target string `json:"target"`
}

type StarterTemplate struct {
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	Request string        `json:"request"`
	Select  string        `json:"select"`
	Preset  StarterPreset `json:"preset"`
}

func NormalizeTarget(target string) string {
	t := strings.ToLower(strings.TrimSpace(target))
	switch t {
	case "chat", "api", "agent":
		return t
	default:
		return "chat"
	}
}

func ResolveLanguageInstruction(settingVal string) string {
	switch strings.ToLower(strings.TrimSpace(settingVal)) {
	case "vi", "vietnamese", "tiếng việt":
		return "Tiếng Việt"
	case "en", "english":
		return "English"
	default:
		return "Theo đúng ngôn ngữ chính của <user_request> (mặc định Tiếng Việt nếu không rõ)"
	}
}

func LoadTargetGuide(target string) string {
	t := NormalizeTarget(target)
	b, err := templateFS.ReadFile(fmt.Sprintf("templates/targets/%s.md", t))
	if err != nil {
		return "## Nơi dùng (chat)\n- Tối ưu cho hội thoại trực tiếp trên ChatGPT, Claude, Gemini với tiêu đề Markdown rõ ràng."
	}
	return strings.TrimSpace(string(b))
}

func RenderGeneratePrompt(data GenerateTemplateData) (string, error) {
	data.Target = NormalizeTarget(data.Target)
	data.Language = ResolveLanguageInstruction(data.Language)
	if data.TargetGuide == "" {
		data.TargetGuide = LoadTargetGuide(data.Target)
	}
	raw, err := templateFS.ReadFile("templates/generate.v2.md")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("generate.v2").Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func RenderRefinePrompt(data RefineTemplateData) (string, error) {
	data.Target = NormalizeTarget(data.Target)
	data.Language = ResolveLanguageInstruction(data.Language)
	if data.TargetGuide == "" {
		data.TargetGuide = LoadTargetGuide(data.Target)
	}
	raw, err := templateFS.ReadFile("templates/refine.v2.md")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("refine.v2").Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func LoadStarters() ([]StarterTemplate, error) {
	raw, err := templateFS.ReadFile("templates/starters.vi.json")
	if err != nil {
		return nil, err
	}
	var list []StarterTemplate
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}
