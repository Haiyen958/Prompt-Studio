package ai

import (
	"strings"
	"testing"
)

func TestStreamSplitterSplitAcrossDelimiterAndUTF8(t *testing.T) {
	raw := "### Vai trò\nBạn là kỹ sư sản phẩm cho {{san_pham}} (không dùng dấu ===META=== trong câu).\n===META===\n{\"intent\":\"Thiết kế tìm kiếm video\",\"assumptions\":[{\"label\":\"Nền tảng\",\"value\":\"Web desktop\",\"alternatives\":[\"Mobile app\",\"Cả hai\"]}]}"

	var streamed strings.Builder
	sp := NewStreamSplitter(func(delta string) {
		streamed.WriteString(delta)
	})

	// Feed 3 bytes at a time (cuts right across multi-byte Vietnamese UTF-8 runes and across `===META===`)
	for i := 0; i < len(raw); i += 3 {
		end := i + 3
		if end > len(raw) {
			end = len(raw)
		}
		sp.Feed(raw[i:end])
	}

	res := sp.Finalize()
	if strings.Contains(streamed.String(), "\n===META===") {
		t.Fatalf("delimiter leaked into streamed output: %q", streamed.String())
	}
	expectedPrompt := "### Vai trò\nBạn là kỹ sư sản phẩm cho {{san_pham}} (không dùng dấu ===META=== trong câu)."
	if res.Prompt != expectedPrompt || strings.TrimSpace(streamed.String()) != expectedPrompt {
		t.Fatalf("unexpected prompt: got=%q streamed=%q want=%q", res.Prompt, streamed.String(), expectedPrompt)
	}
	if !res.MetaPresent || !res.MetaValid {
		t.Fatalf("expected valid meta, got present=%v valid=%v", res.MetaPresent, res.MetaValid)
	}
	if res.Meta.Intent != "Thiết kế tìm kiếm video" || len(res.Meta.Assumptions) != 1 {
		t.Fatalf("unexpected meta: %+v", res.Meta)
	}
}

func TestStreamSplitterGracefulOnBrokenOrMissingMeta(t *testing.T) {
	sp := NewStreamSplitter(nil)
	sp.Feed("Prompt nội dung chuẩn\n===META===\n{broken json")
	res := sp.Finalize()

	if res.Prompt != "Prompt nội dung chuẩn" {
		t.Fatalf("expected prompt preserved when meta broken, got %q", res.Prompt)
	}
	if !res.MetaPresent || res.MetaValid {
		t.Fatalf("expected MetaPresent=true MetaValid=false, got %+v", res)
	}
}

func TestExtractAndFillVariablesAndTemplateDelimiters(t *testing.T) {
	text := "Phân tích {{chu_de}} cho {{ doi_tuong }} và lặp lại {{chu_de}}, bỏ qua {{123_invalid}} và {{kebab-case}}."
	vars := ExtractVariables(text)
	if len(vars) != 2 || vars[0] != "chu_de" || vars[1] != "doi_tuong" {
		t.Fatalf("unexpected variables: %+v", vars)
	}

	filled := FillVariables(text, map[string]string{
		"chu_de":    "onboarding",
		"doi_tuong": "khách hàng SaaS",
	})
	if !strings.Contains(filled, "Phân tích onboarding cho khách hàng SaaS") {
		t.Fatalf("FillVariables failed: %q", filled)
	}

	// Render template containing `{{user_var}}` inside request without Go template collision
	rendered, err := RenderGeneratePrompt(GenerateTemplateData{
		Request:  "Soạn email với biến {{ten_khach_hang}} và {{ma_don}}",
		Language: "Tiếng Việt",
		Target:   "api",
	})
	if err != nil {
		t.Fatalf("RenderGeneratePrompt failed on {{...}} input: %v", err)
	}
	if !strings.Contains(rendered, "{{ten_khach_hang}}") {
		t.Fatalf("expected {{ten_khach_hang}} preserved verbatim in rendered template")
	}
}
