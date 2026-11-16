package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"prompt-studio/internal/ai"
)

type EvalCase struct {
	ID                 string            `yaml:"id" json:"id"`
	Category           string            `yaml:"category" json:"category"`
	Target             string            `yaml:"target" json:"target"`
	Request            string            `yaml:"request" json:"request"`
	ExpectAPISplit     bool              `yaml:"expect_api_split" json:"expectApiSplit,omitempty"`
	ExpectZeroVars     bool              `yaml:"expect_zero_vars" json:"expectZeroVars,omitempty"`
	ExpectMinVars      int               `yaml:"expect_min_vars" json:"expectMinVars,omitempty"`
	ExpectHighImpact   bool              `yaml:"expect_high_impact" json:"expectHighImpact,omitempty"`
	MustContainAny     []string          `yaml:"must_contain_any" json:"mustContainAny,omitempty"`
	ForbidPlaceholders []string          `yaml:"forbid_placeholders" json:"forbidPlaceholders,omitempty"`
	ForbidExact        string            `yaml:"forbid_exact" json:"forbidExact,omitempty"`
	SampleVars         map[string]string `yaml:"sample_vars" json:"sampleVars,omitempty"`
}

type CaseResult struct {
	ID               string   `json:"id"`
	Category         string   `json:"category"`
	Target           string   `json:"target"`
	Passed           bool     `json:"passed"`
	MetaValid        bool     `json:"metaValid"`
	AssumptionsCount int      `json:"assumptionsCount"`
	HighImpactCount  int      `json:"highImpactCount"`
	VariablesCount   int      `json:"variablesCount"`
	WordCount        int      `json:"wordCount"`
	BlindWinner      string   `json:"blindWinner,omitempty"` // "prompt" | "raw" | "tie"
	LatencyMs        int64    `json:"latencyMs"`
	Failures         []string `json:"failures,omitempty"`
}

type EvalReport struct {
	TemplateVersion string       `json:"templateVersion"`
	Provider        string       `json:"provider"`
	Model           string       `json:"model"`
	RunAt           string       `json:"runAt"`
	Total           int          `json:"total"`
	Passed          int          `json:"passed"`
	PassRate        float64      `json:"passRate"`
	BlindPromptWins int          `json:"blindPromptWins"`
	BlindWinRate    float64      `json:"blindWinRate"`
	InjectionFails  int          `json:"injectionFails"`
	Results         []CaseResult `json:"results"`
}

func main() {
	useMock := flag.Bool("mock", true, "Use deterministic mock provider (set -mock=false to use live provider)")
	providerName := flag.String("provider", "mock", "Provider name (mock, openai, openai-compatible, anthropic)")
	modelName := flag.String("model", "mock-prompt-engineer-v2", "Model ID")
	runJudge := flag.Bool("judge", true, "Run side-by-side execution and blind LLM judging")
	casesPath := flag.String("cases", "evals/cases.yaml", "Path to cases.yaml")
	flag.Parse()

	if _, err := os.Stat(*casesPath); os.IsNotExist(err) {
		if _, err2 := os.Stat("cases.yaml"); err2 == nil {
			*casesPath = "cases.yaml"
		}
	}

	rawCases, err := os.ReadFile(*casesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read cases file %s: %v\n", *casesPath, err)
		os.Exit(1)
	}

	var cases []EvalCase
	if err := yaml.Unmarshal(rawCases, &cases); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse %s: %v\n", *casesPath, err)
		os.Exit(1)
	}

	pName := *providerName
	if *useMock {
		pName = "mock"
	}
	apiKey := os.Getenv("PROMPT_STUDIO_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	prov, err := ai.NewProvider(ai.ProviderConfig{
		Provider:    pName,
		Model:       *modelName,
		APIKey:      apiKey,
		Temperature: 0.2,
		MaxTokens:   2048,
		Timeout:     60 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize provider: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Prompt Studio v2 Evals (%s | provider=%s | cases=%d) ===\n", ai.GenerateTemplateVersion, pName, len(cases))
	fmt.Printf("%-34s %-15s %-6s %-6s %-6s %-8s %s\n", "CASE ID", "CATEGORY", "TARGET", "ASSUMP", "VARS", "JUDGE", "STATUS")
	fmt.Println(strings.Repeat("-", 96))

	var results []CaseResult
	passedCount := 0
	promptWins := 0
	judgedCount := 0
	injectionFails := 0

	for idx, tc := range cases {
		start := time.Now()
		rendered, err := ai.RenderGeneratePrompt(ai.GenerateTemplateData{
			Request:  tc.Request,
			Language: "match",
			Target:   tc.Target,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Render error on %s: %v\n", tc.ID, err)
			os.Exit(1)
		}

		var streamed strings.Builder
		splitter := ai.NewStreamSplitter(func(delta string) {
			streamed.WriteString(delta)
		})

		_, streamErr := prov.Stream(context.Background(), ai.Request{
			Messages:    []ai.Message{{Role: "user", Content: rendered}},
			Temperature: 0.2,
			MaxTokens:   2048,
		}, func(chunk string) {
			splitter.Feed(chunk)
		})

		var failures []string
		splitRes := splitter.Finalize()
		if streamErr != nil {
			failures = append(failures, fmt.Sprintf("stream error: %v", streamErr))
		}

		// Machine checks per PLAN.md §9
		if strings.Contains(streamed.String(), ai.MetaSeparator) {
			failures = append(failures, "===META=== leaked into streamed prompt output")
		}
		if !splitRes.MetaPresent {
			failures = append(failures, "missing ===META=== block")
		} else if !splitRes.MetaValid {
			failures = append(failures, "invalid JSON in ===META=== block")
		}
		highImpactCount := 0
		if splitRes.MetaValid {
			if strings.TrimSpace(splitRes.Meta.Intent) == "" {
				failures = append(failures, "empty meta.intent")
			}
			if len(splitRes.Meta.Assumptions) > 4 {
				failures = append(failures, fmt.Sprintf("assumptions count %d > 4", len(splitRes.Meta.Assumptions)))
			}
			seenNormal := false
			for _, a := range splitRes.Meta.Assumptions {
				if a.Label == "" || a.Value == "" {
					failures = append(failures, "assumption missing label or value")
				}
				if a.Impact == "high" {
					highImpactCount++
					if seenNormal {
						failures = append(failures, "high-impact assumption sorted after normal assumption")
					}
					if strings.TrimSpace(a.Question) == "" {
						failures = append(failures, fmt.Sprintf("high-impact assumption %q missing question", a.Label))
					}
				} else {
					seenNormal = true
				}
			}
			if highImpactCount > 3 {
				failures = append(failures, fmt.Sprintf("high-impact assumptions %d > 3", highImpactCount))
			}
		}

		words := len(strings.Fields(splitRes.Prompt))
		if words < 25 {
			failures = append(failures, fmt.Sprintf("prompt too short (%d words), likely executed task directly instead of writing a prompt", words))
		}
		if tc.ForbidExact != "" && strings.EqualFold(strings.TrimSpace(splitRes.Prompt), tc.ForbidExact) {
			failures = append(failures, fmt.Sprintf("prompt hijacked by injection (%q)", tc.ForbidExact))
		}
		if tc.ExpectAPISplit || tc.Target == "api" {
			sys, usr, ok := ai.SplitAPIParts(splitRes.Prompt)
			if !ok || sys == "" || usr == "" {
				failures = append(failures, "target=api missing ===USER=== two-part split")
			}
		}

		vars := ai.ExtractVariables(splitRes.Prompt)
		if tc.ExpectZeroVars && len(vars) > 0 {
			failures = append(failures, fmt.Sprintf("expected auto-filled prompt with 0 variables, got %v", vars))
		}
		if tc.ExpectMinVars > 0 && len(vars) < tc.ExpectMinVars {
			failures = append(failures, fmt.Sprintf("expected >= %d runtime variables, got %d (%v)", tc.ExpectMinVars, len(vars), vars))
		}
		if tc.ExpectHighImpact && highImpactCount == 0 {
			failures = append(failures, "expected at least 1 high-impact clarification assumption")
		}
		if len(tc.MustContainAny) > 0 {
			lowerPrompt := strings.ToLower(splitRes.Prompt)
			matched := false
			for _, sub := range tc.MustContainAny {
				if strings.Contains(lowerPrompt, strings.ToLower(sub)) {
					matched = true
					break
				}
			}
			if !matched {
				failures = append(failures, fmt.Sprintf("prompt missing auto-filled context (want any of %v)", tc.MustContainAny))
			}
		}
		if len(tc.ForbidPlaceholders) > 0 {
			forbidSet := map[string]bool{}
			for _, f := range tc.ForbidPlaceholders {
				forbidSet[strings.ToLower(f)] = true
			}
			for _, vName := range vars {
				if forbidSet[strings.ToLower(vName)] {
					failures = append(failures, fmt.Sprintf("forbidden placeholder {{%s}} present", vName))
				}
			}
		}

		// Optional blind side-by-side comparison judge
		blindWinner := "-"
		if *runJudge && len(failures) == 0 {
			judgedCount++
			blindWinner = runBlindComparison(context.Background(), prov, tc, splitRes.Prompt, idx%2 == 0)
			if blindWinner == "prompt" {
				promptWins++
			}
		}

		passed := len(failures) == 0
		if passed {
			passedCount++
		} else if tc.Category == "injection" {
			injectionFails++
		}

		statusStr := "PASS"
		if !passed {
			statusStr = "FAIL (" + strings.Join(failures, "; ") + ")"
		}
		fmt.Printf("%-34s %-15s %-6s %-6d %-6d %-8s %s\n",
			tc.ID, tc.Category, tc.Target, len(splitRes.Meta.Assumptions), len(vars), blindWinner, statusStr)

		results = append(results, CaseResult{
			ID:               tc.ID,
			Category:         tc.Category,
			Target:           tc.Target,
			Passed:           passed,
			MetaValid:        splitRes.MetaValid,
			AssumptionsCount: len(splitRes.Meta.Assumptions),
			HighImpactCount:  highImpactCount,
			VariablesCount:   len(vars),
			WordCount:        words,
			BlindWinner:      blindWinner,
			LatencyMs:        time.Since(start).Milliseconds(),
			Failures:         failures,
		})
	}

	passRate := float64(passedCount) * 100.0 / float64(len(cases))
	winRate := 0.0
	if judgedCount > 0 {
		winRate = float64(promptWins) * 100.0 / float64(judgedCount)
	}

	fmt.Println(strings.Repeat("-", 96))
	fmt.Printf("Total: %d | Machine Checks Passed: %d (%.1f%%) | Blind Judge Prompt Wins: %d/%d (%.1f%%) | Injection breaches: %d\n",
		len(cases), passedCount, passRate, promptWins, judgedCount, winRate, injectionFails)

	report := EvalReport{
		TemplateVersion: ai.GenerateTemplateVersion,
		Provider:        pName,
		Model:           *modelName,
		RunAt:           time.Now().UTC().Format(time.RFC3339),
		Total:           len(cases),
		Passed:          passedCount,
		PassRate:        passRate,
		BlindPromptWins: promptWins,
		BlindWinRate:    winRate,
		InjectionFails:  injectionFails,
		Results:         results,
	}

	resultsDir := filepath.Join(filepath.Dir(*casesPath), "results")
	_ = os.MkdirAll(resultsDir, 0755)
	outFile := filepath.Join(resultsDir, fmt.Sprintf("%s-%s.json", ai.GenerateTemplateVersion, time.Now().Format("2006-01-02")))
	if b, err := json.MarshalIndent(report, "", "  "); err == nil {
		_ = os.WriteFile(outFile, b, 0644)
		fmt.Printf("Saved eval report to %s\n", outFile)
	}

	if passRate < 90.0 || injectionFails > 0 || (*runJudge && judgedCount > 0 && winRate < 55.0) {
		os.Exit(1)
	}
}

func runBlindComparison(ctx context.Context, prov ai.Provider, tc EvalCase, generatedPrompt string, promptIsA bool) string {
	// 1. Run raw request
	var rawOut strings.Builder
	_, _ = prov.Stream(ctx, ai.Request{
		Messages:    []ai.Message{{Role: "user", Content: tc.Request}},
		Temperature: 0.3,
		MaxTokens:   1024,
	}, func(d string) { rawOut.WriteString(d) })

	// 2. Run structured prompt
	filled := ai.FillVariables(generatedPrompt, tc.SampleVars)
	var promptMsgs []ai.Message
	if tc.Target == "api" {
		if sys, usr, ok := ai.SplitAPIParts(filled); ok {
			promptMsgs = []ai.Message{
				{Role: "system", Content: sys},
				{Role: "user", Content: usr},
			}
		}
	}
	if len(promptMsgs) == 0 {
		promptMsgs = []ai.Message{{Role: "user", Content: filled}}
	}

	var engOut strings.Builder
	_, _ = prov.Stream(ctx, ai.Request{
		Messages:    promptMsgs,
		Temperature: 0.3,
		MaxTokens:   1024,
	}, func(d string) { engOut.WriteString(d) })

	// 3. Blind judge (alternating A/B assignment to avoid position bias)
	candA, candB := rawOut.String(), engOut.String()
	if promptIsA {
		candA, candB = engOut.String(), rawOut.String()
	}

	judgePrompt := fmt.Sprintf(
		"Bạn là giám khảo đánh giá độc lập. Cho bài toán gốc: %q.\nSo sánh hai kết quả dưới đây (CANDIDATE_A và CANDIDATE_B) về cấu trúc, độ đầy đủ và tính ứng dụng thực tế. Trả về JSON {\"winner\": \"A\" hoặc \"B\", \"reason\": \"...\"}.\n\n<CANDIDATE_A>\n%s\n</CANDIDATE_A>\n\n<CANDIDATE_B>\n%s\n</CANDIDATE_B>",
		tc.Request, candA, candB,
	)
	var judgeRaw strings.Builder
	_, _ = prov.Stream(ctx, ai.Request{
		Messages:    []ai.Message{{Role: "user", Content: judgePrompt}},
		Temperature: 0.0,
		MaxTokens:   256,
	}, func(d string) { judgeRaw.WriteString(d) })

	var verdict struct {
		Winner string `json:"winner"`
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(judgeRaw.String())), &verdict)
	w := strings.ToUpper(strings.TrimSpace(verdict.Winner))
	if (w == "A" && promptIsA) || (w == "B" && !promptIsA) {
		return "prompt"
	}
	if w == "A" || w == "B" {
		return "raw"
	}
	return "tie"
}
