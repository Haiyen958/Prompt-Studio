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

func TestStreamSplitterHighImpactPriorityCapsAndNormalization(t *testing.T) {
	raw := "Prompt an toàn\n===META===\n" + `{
		"intent": "Xóa dữ liệu trùng an toàn",
		"assumptions": [
			{"label": "Định dạng", "value": "SQL script", "alternatives": ["SQL script", "CLI tool", "  "], "impact": "low", "question": "Không nên giữ"},
			{"label": "An toàn dữ liệu", "value": "Dry-run trước", "alternatives": ["Soft-delete", "Hard-delete"], "impact": "CRITICAL", "question": "Dữ liệu nào được phép xóa?"},
			{"label": "Sao lưu", "value": "Tạo bảng backup", "alternatives": ["Dump ra file"], "impact": "high", "question": "Cơ chế sao lưu trước khi xóa?"},
			{"label": "Phê duyệt", "value": "Chờ xác nhận tay", "alternatives": ["Chạy tự động"], "impact": "HIGH", "question": "Có cần duyệt tay không?"},
			{"label": "Thừa mức cao thứ 4", "value": "Mặc định", "alternatives": ["Khác"], "impact": "high", "question": "Phải bị hạ xuống thường và cắt ở giới hạn 4"}
		]
	}`

	sp := NewStreamSplitter(nil)
	sp.Feed(raw)
	res := sp.Finalize()
	if !res.MetaPresent || !res.MetaValid {
		t.Fatalf("expected valid meta, got %+v", res)
	}
	if len(res.Meta.Assumptions) != 4 {
		t.Fatalf("expected total assumptions capped at 4, got %d: %+v", len(res.Meta.Assumptions), res.Meta.Assumptions)
	}

	// High-impact items (max 3) must be stable-sorted ahead of normal items.
	wantLabels := []string{"An toàn dữ liệu", "Sao lưu", "Phê duyệt", "Định dạng"}
	wantImpacts := []string{"high", "high", "high", ""}
	for i := range wantLabels {
		got := res.Meta.Assumptions[i]
		if got.Label != wantLabels[i] || got.Impact != wantImpacts[i] {
			t.Fatalf("assumption[%d] = {Label:%q Impact:%q}, want {Label:%q Impact:%q}",
				i, got.Label, got.Impact, wantLabels[i], wantImpacts[i])
		}
	}
	if res.Meta.Assumptions[0].Question != "Dữ liệu nào được phép xóa?" {
		t.Fatalf("unexpected question on high-impact item: %q", res.Meta.Assumptions[0].Question)
	}
	if res.Meta.Assumptions[3].Question != "" {
		t.Fatalf("normal assumption must not retain question, got %q", res.Meta.Assumptions[3].Question)
	}
	// Duplicate/blank/value-matching alternatives are stripped.
	if len(res.Meta.Assumptions[3].Alternatives) != 1 || res.Meta.Assumptions[3].Alternatives[0] != "CLI tool" {
		t.Fatalf("unexpected deduplicated alternatives: %+v", res.Meta.Assumptions[3].Alternatives)
	}
}

func TestTemplates4TierRulesAndNoForcedAgentPlaceholders(t *testing.T) {
	if GenerateTemplateVersion != "generate.v2.1" || RefineTemplateVersion != "refine.v2.1" {
		t.Fatalf("unexpected template versions: %q / %q", GenerateTemplateVersion, RefineTemplateVersion)
	}
	agentPrompt, err := RenderGeneratePrompt(GenerateTemplateData{
		Request:  "Sửa lỗi API thanh toán trả 500 khi trùng request",
		Language: "vi",
		Target:   "agent",
	})
	if err != nil {
		t.Fatalf("RenderGeneratePrompt(agent): %v", err)
	}
	if strings.Contains(agentPrompt, "Dùng biến `{{pham_vi_module}}`") {
		t.Fatalf("agent target guide still forces redundant {{pham_vi_module}} placeholder")
	}
	if !strings.Contains(agentPrompt, `"impact": "high"`) || !strings.Contains(agentPrompt, "khảo sát") {
		t.Fatalf("expected 4-tier high-impact and repo-discovery rules in rendered agent meta-prompt")
	}
}
