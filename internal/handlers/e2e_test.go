package handlers_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/handlers"
	"prompt-studio/internal/secrets"
	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
	"prompt-studio/web"
)

type parsedSSEEvent struct {
	Event string
	Data  map[string]any
}

func parseSSEStream(t *testing.T, r io.Reader) []parsedSSEEvent {
	t.Helper()
	var events []parsedSSEEvent
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var curEvent string
	var dataLines []string

	flush := func() {
		if curEvent == "" && len(dataLines) == 0 {
			return
		}
		raw := strings.Join(dataLines, "\n")
		payload := map[string]any{}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatalf("failed to unmarshal SSE event %q payload %q: %v", curEvent, raw, err)
			}
		}
		events = append(events, parsedSSEEvent{Event: curEvent, Data: payload})
		curEvent = ""
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			curEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("SSE scanner error: %v", err)
	}
	flush()
	return events
}

func setupTestEnv(t *testing.T, token string) (*httptest.Server, *studio.Service, string) {
	t.Helper()
	tmpDir := t.TempDir()
	secPath := filepath.Join(tmpDir, "secrets.json")
	dbPath := filepath.Join(tmpDir, "prompt-studio.db")

	secStore := secrets.NewFileStore(secPath)
	db, err := storage.OpenDB(dbPath, secStore)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := storage.NewRepo(db, secStore)
	svc := studio.NewService(repo)
	svc.SetProviderFactory(func(cfg ai.ProviderConfig) (ai.Provider, error) {
		return ai.NewMockProvider(cfg.Model), nil
	})

	router := handlers.NewRouter(svc, web.Assets, server.SecurityConfig{
		SessionToken: token,
		RequireToken: token != "",
	})
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return ts, svc, secPath
}

func doJSONRequest(t *testing.T, client *http.Client, method, url, token string, body any) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Prompt-Studio-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	return resp
}

func TestV2FullLifecycleGenerateAssumptionSaveReopenAndRefine(t *testing.T) {
	const token = "test-session-token-v2"
	ts, _, secPath := setupTestEnv(t, token)
	client := ts.Client()

	// 1. Exchange startup ?token=... into HttpOnly cookie (302 redirect stripping ?token=)
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	tokResp, err := noRedirectClient.Get(ts.URL + "/?token=" + token)
	if err != nil {
		t.Fatalf("GET /?token=... failed: %v", err)
	}
	_ = tokResp.Body.Close()
	if tokResp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302 redirect on token exchange, got %d", tokResp.StatusCode)
	}
	if loc := tokResp.Header.Get("Location"); loc != "/" {
		t.Fatalf("expected clean redirect location '/', got %q", loc)
	}

	// 2. Configure Settings with untested API key -> must NOT block save, must return warning & never leak raw key
	rawKey := "sk-live-super-secret-9z8x"
	putSetResp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/settings", token, map[string]any{
		"provider":       "mock",
		"model":          "gpt-4o-mini",
		"promptLanguage": "vi",
		"defaultTarget":  "api",
		"apiKey":         rawKey,
		"tested":         false,
	})
	defer putSetResp.Body.Close()
	if putSetResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/settings status = %d", putSetResp.StatusCode)
	}
	putBodyBytes, _ := io.ReadAll(putSetResp.Body)
	if strings.Contains(string(putBodyBytes), rawKey) {
		t.Fatalf("SECURITY BREACH: raw API key appeared in PUT /api/settings response")
	}
	var setOut handlers.SettingsResponse
	if err := json.Unmarshal(putBodyBytes, &setOut); err != nil {
		t.Fatalf("unmarshal settings response: %v", err)
	}
	if !setOut.HasKey || setOut.KeyHint != "••••••••••9z8x" {
		t.Fatalf("expected hasKey=true and hint ••••••••••9z8x, got hasKey=%v hint=%q", setOut.HasKey, setOut.KeyHint)
	}
	if setOut.Warning == "" {
		t.Fatalf("expected non-blocking warning when saving untested API key")
	}
	info, err := os.Stat(secPath)
	if err != nil {
		t.Fatalf("secrets.json was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected secrets.json perm 0600, got %04o", perm)
	}

	// Test Connection endpoint
	testConnResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/settings/test", token, map[string]any{
		"provider": "mock",
		"model":    "gpt-4o-mini",
	})
	defer testConnResp.Body.Close()
	if testConnResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/settings/test status = %d", testConnResp.StatusCode)
	}

	// 3. Create Session & Generate v1 (Target = "api")
	genResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions", token, map[string]any{
		"request":  "Phân loại email hỗ trợ khách hàng thành JSON cho hệ thống ticket.",
		"language": "vi",
		"target":   "api",
	})
	defer genResp.Body.Close()
	if genResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/sessions status = %d", genResp.StatusCode)
	}
	genEvents := parseSSEStream(t, genResp.Body)

	var sessionID, v1ID, v1Content string
	var streamedPrompt strings.Builder
	var v1Assumptions []any
	for _, ev := range genEvents {
		switch ev.Event {
		case "session":
			if sessMap, ok := ev.Data["session"].(map[string]any); ok {
				sessionID, _ = sessMap["id"].(string)
			}
		case "delta":
			txt, _ := ev.Data["delta"].(string)
			streamedPrompt.WriteString(txt)
		case "done":
			if verMap, ok := ev.Data["version"].(map[string]any); ok {
				v1ID, _ = verMap["id"].(string)
				v1Content, _ = verMap["content"].(string)
				v1Assumptions, _ = verMap["assumptions"].([]any)
			}
		case "error":
			t.Fatalf("unexpected SSE error on generate: %v", ev.Data)
		}
	}
	if sessionID == "" || v1ID == "" {
		t.Fatalf("missing sessionID (%q) or v1ID (%q)", sessionID, v1ID)
	}
	if streamedPrompt.Len() == 0 {
		t.Fatalf("expected non-empty delta stream")
	}
	if strings.Contains(streamedPrompt.String(), "===META===") {
		t.Fatalf("streamed prompt leaked ===META=== delimiter: %q", streamedPrompt.String())
	}
	sysPart, usrPart, hasSplit := ai.SplitAPIParts(v1Content)
	if !hasSplit || sysPart == "" || usrPart == "" {
		t.Fatalf("expected target=api prompt to split on ===USER=== into system & user blocks, got: %q", v1Content)
	}
	if len(v1Assumptions) == 0 || len(v1Assumptions) > 4 {
		t.Fatalf("expected 1..4 assumptions on v1, got %d", len(v1Assumptions))
	}

	// 4. Change Assumption -> creates v2 (seq=2, parentId=v1ID)
	refAssumpResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/"+sessionID+"/refine", token, map[string]any{
		"fromVersionId": v1ID,
		"source":        "assumption",
		"instruction":   "Đổi giả định 'Độ nghiêm ngặt JSON' thành 'Bắt buộc trả về đúng JSON Schema, không kèm markdown'.",
	})
	defer refAssumpResp.Body.Close()
	assumpEvents := parseSSEStream(t, refAssumpResp.Body)
	var v2ID string
	for _, ev := range assumpEvents {
		if ev.Event == "done" {
			verMap, _ := ev.Data["version"].(map[string]any)
			v2ID, _ = verMap["id"].(string)
			if int(verMap["seq"].(float64)) != 2 {
				t.Fatalf("expected v2 seq=2, got %v", verMap["seq"])
			}
			if verMap["parentId"] != v1ID {
				t.Fatalf("expected v2 parentId=%s, got %v", v1ID, verMap["parentId"])
			}
		}
	}
	if v2ID == "" {
		t.Fatalf("expected v2ID from assumption refine")
	}

	// 5. One-click Save to Library -> defaults to "Chưa phân loại" (col_uncategorized)
	saveResp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/sessions/"+sessionID+"/save", token, map[string]any{})
	defer saveResp.Body.Close()
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/sessions/{id}/save status = %d", saveResp.StatusCode)
	}
	var savedSess storage.Session
	if err := json.NewDecoder(saveResp.Body).Decode(&savedSess); err != nil {
		t.Fatalf("decode saved session: %v", err)
	}
	if savedSess.SavedAt == nil {
		t.Fatalf("expected savedAt != nil after Save")
	}
	if savedSess.CollectionID != storage.SystemCollectionID {
		t.Fatalf("expected default collection %q, got %q", storage.SystemCollectionID, savedSess.CollectionID)
	}

	// 6. Reopen Session & Refine again (source="improve") -> creates v3 (seq=3, parentId=v2ID)
	getDetail1 := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/sessions/"+sessionID, token, nil)
	_ = getDetail1.Body.Close()
	if getDetail1.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/sessions/{id} status = %d", getDetail1.StatusCode)
	}

	refImproveResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/"+sessionID+"/refine", token, map[string]any{
		"fromVersionId": v2ID,
		"source":        "improve",
		"instruction":   "Thêm hướng dẫn xử lý trường hợp email trống hoặc chỉ có chữ ký.",
	})
	defer refImproveResp.Body.Close()
	improveEvents := parseSSEStream(t, refImproveResp.Body)
	var v3ID string
	for _, ev := range improveEvents {
		if ev.Event == "done" {
			verMap, _ := ev.Data["version"].(map[string]any)
			v3ID, _ = verMap["id"].(string)
		}
	}
	if v3ID == "" {
		t.Fatalf("expected v3ID from improve refine")
	}

	// Verify session now has 3 immutable versions with intact lineage
	getDetail2 := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/sessions/"+sessionID, token, nil)
	defer getDetail2.Body.Close()
	var detail storage.SessionDetail
	if err := json.NewDecoder(getDetail2.Body).Decode(&detail); err != nil {
		t.Fatalf("decode session detail: %v", err)
	}
	if len(detail.Versions) != 3 {
		t.Fatalf("expected 3 versions in session, got %d", len(detail.Versions))
	}
	if detail.Versions[0].Seq != 1 || detail.Versions[1].Seq != 2 || detail.Versions[2].Seq != 3 {
		t.Fatalf("unexpected version sequence numbers: %+v", detail.Versions)
	}
	if detail.Versions[1].ParentID != v1ID || detail.Versions[2].ParentID != v2ID {
		t.Fatalf("unexpected parentId lineage: v2.Parent=%q v3.Parent=%q", detail.Versions[1].ParentID, detail.Versions[2].ParentID)
	}
}

func TestPasteDuplicateCollectionsFallbackAndExportImport(t *testing.T) {
	const token = "paste-col-export-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	// 1. Create custom collection
	colResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/collections", token, map[string]any{
		"name": "Backend Go",
	})
	defer colResp.Body.Close()
	if colResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/collections status = %d", colResp.StatusCode)
	}
	var customCol storage.Collection
	_ = json.NewDecoder(colResp.Body).Decode(&customCol)

	// 2. Paste prompt into custom collection
	pasteResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/paste", token, map[string]any{
		"title":        "Review PR Go",
		"description":  "Kiểm tra race condition và SQL injection",
		"content":      "Bạn là Senior Go Engineer.\nHãy kiểm tra đoạn code: {{git_diff}}",
		"collectionId": customCol.ID,
		"target":       "agent",
	})
	defer pasteResp.Body.Close()
	if pasteResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/sessions/paste status = %d", pasteResp.StatusCode)
	}
	var pasteOut struct {
		Session storage.Session       `json:"session"`
		Version storage.PromptVersion `json:"version"`
	}
	_ = json.NewDecoder(pasteResp.Body).Decode(&pasteOut)
	if pasteOut.Session.Origin != "pasted" || pasteOut.Session.VariablesCount != 1 {
		t.Fatalf("unexpected pasted session: %+v", pasteOut.Session)
	}

	// 3. Duplicate paste with different whitespace -> 409 Conflict + existingSessionId
	dupResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/paste", token, map[string]any{
		"title":   "Bản trùng",
		"content": "  Bạn   là Senior Go Engineer.   Hãy kiểm tra đoạn code: {{git_diff}} \n\n ",
		"target":  "agent",
	})
	defer dupResp.Body.Close()
	if dupResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on normalized duplicate paste, got %d", dupResp.StatusCode)
	}
	var dupBody map[string]any
	_ = json.NewDecoder(dupResp.Body).Decode(&dupBody)
	if dupBody["existingSessionId"] != pasteOut.Session.ID {
		t.Fatalf("expected existingSessionId=%q, got %v", pasteOut.Session.ID, dupBody["existingSessionId"])
	}

	// 4. System collection "Chưa phân loại" cannot be deleted
	delSys := doJSONRequest(t, client, http.MethodDelete, ts.URL+"/api/collections/"+storage.SystemCollectionID, token, nil)
	_ = delSys.Body.Close()
	if delSys.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden deleting system collection, got %d", delSys.StatusCode)
	}

	// 6. Deleting custom collection moves saved session safely to "Chưa phân loại"
	delCustom := doJSONRequest(t, client, http.MethodDelete, ts.URL+"/api/collections/"+customCol.ID, token, nil)
	_ = delCustom.Body.Close()
	if delCustom.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 deleting custom collection, got %d", delCustom.StatusCode)
	}
	checkSess := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/sessions/"+pasteOut.Session.ID, token, nil)
	defer checkSess.Body.Close()
	var afterDel storage.SessionDetail
	_ = json.NewDecoder(checkSess.Body).Decode(&afterDel)
	if afterDel.Session.CollectionID != storage.SystemCollectionID {
		t.Fatalf("expected session moved to %q after collection delete, got %q", storage.SystemCollectionID, afterDel.Session.CollectionID)
	}

	// 7. Export JSON backup & Import into a fresh instance
	expResp := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/export", token, nil)
	defer expResp.Body.Close()
	if expResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/export status = %d", expResp.StatusCode)
	}
	backupBytes, err := io.ReadAll(expResp.Body)
	if err != nil {
		t.Fatalf("read export body: %v", err)
	}

	ts2, _, _ := setupTestEnv(t, token)
	impReq, _ := http.NewRequest(http.MethodPost, ts2.URL+"/api/import", bytes.NewReader(backupBytes))
	impReq.Header.Set("Content-Type", "application/json")
	impReq.Header.Set("X-Prompt-Studio-Token", token)
	impResp, err := ts2.Client().Do(impReq)
	if err != nil {
		t.Fatalf("POST /api/import failed: %v", err)
	}
	defer impResp.Body.Close()
	if impResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/import status = %d", impResp.StatusCode)
	}

	restoredResp := doJSONRequest(t, ts2.Client(), http.MethodGet, ts2.URL+"/api/sessions/"+pasteOut.Session.ID, token, nil)
	defer restoredResp.Body.Close()
	if restoredResp.StatusCode != http.StatusOK {
		t.Fatalf("expected imported session to exist in target DB, got %d", restoredResp.StatusCode)
	}
}

type slowCancelProvider struct{}

func (p *slowCancelProvider) Ping(ctx context.Context) error { return nil }
func (p *slowCancelProvider) Stream(ctx context.Context, _ ai.Request, onDelta func(string)) (ai.Usage, error) {
	onDelta("## Vai trò\nBạn là chuyên gia kiểm thử phần mềm.\n\n## Nhiệm vụ\nĐang viết dở prompt kiểm thử tải cho cổng thanh toán nội bộ...")
	<-ctx.Done()
	return ai.Usage{}, ctx.Err()
}

func TestCancelStreamPersistsIncompleteVersion(t *testing.T) {
	const token = "cancel-stream-token"
	ts, svc, _ := setupTestEnv(t, token)
	svc.SetProviderFactory(func(cfg ai.ProviderConfig) (ai.Provider, error) {
		return &slowCancelProvider{}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	body, _ := json.Marshal(map[string]any{
		"request": "Viết kế hoạch kiểm thử tải cho cổng thanh toán",
		"target":  "chat",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Prompt-Studio-Token", token)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /api/sessions failed: %v", err)
	}
	defer resp.Body.Close()

	// Read until we see the first delta, then cancel client context (simulating user clicking Stop)
	reader := bufio.NewReader(resp.Body)
	for {
		line, rErr := reader.ReadString('\n')
		if strings.Contains(line, "Đang viết dở prompt") {
			cancel()
			break
		}
		if rErr != nil {
			t.Fatalf("stream ended before partial delta arrived: %v", rErr)
		}
	}

	// Wait briefly for server handler goroutine to commit partial version
	var sessions []storage.Session
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		list, _ := svc.Repo().ListSessions(storage.SessionFilter{})
		if len(list) > 0 {
			detail, dErr := svc.Repo().GetSessionDetail(list[0].ID, false)
			if dErr == nil && len(detail.Versions) > 0 {
				sessions = list
				if !detail.Versions[0].Incomplete {
					t.Fatalf("expected partial version to be marked incomplete=true")
				}
				if !strings.Contains(detail.Versions[0].Content, "Đang viết dở prompt") {
					t.Fatalf("expected partial content preserved, got %q", detail.Versions[0].Content)
				}
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for incomplete version to be saved (sessions=%d)", len(sessions))
}

func TestSecurityMiddlewareAndZeroInlineStyles(t *testing.T) {
	const token = "sec-guard-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	// 1. DNS Rebinding Host check on GET -> 403 Forbidden
	badHostReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/settings", nil)
	badHostReq.Host = "evil-rebind.attacker.com"
	badHostResp, err := client.Do(badHostReq)
	if err != nil {
		t.Fatalf("bad host request failed: %v", err)
	}
	_ = badHostResp.Body.Close()
	if badHostResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-loopback Host header, got %d", badHostResp.StatusCode)
	}

	// 2. Cross-Origin check on POST -> 403 Forbidden
	badOriginReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/collections", strings.NewReader(`{"name":"X"}`))
	badOriginReq.Header.Set("Origin", "https://evil.example.com")
	badOriginReq.Header.Set("X-Prompt-Studio-Token", token)
	badOriginResp, err := client.Do(badOriginReq)
	if err != nil {
		t.Fatalf("bad origin request failed: %v", err)
	}
	_ = badOriginResp.Body.Close()
	if badOriginResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for external Origin header, got %d", badOriginResp.StatusCode)
	}

	// 3. Missing session token on write -> 401 Unauthorized
	noTokReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/collections", strings.NewReader(`{"name":"NoTok"}`))
	noTokResp, err := client.Do(noTokReq)
	if err != nil {
		t.Fatalf("no-token request failed: %v", err)
	}
	_ = noTokResp.Body.Close()
	if noTokResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without session token, got %d", noTokResp.StatusCode)
	}

	// 4. Starters endpoint returns 8 Vietnamese templates
	stResp := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/starters", token, nil)
	defer stResp.Body.Close()
	if stResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/starters status = %d", stResp.StatusCode)
	}
	var stBody struct {
		Starters []ai.StarterTemplate `json:"starters"`
	}
	_ = json.NewDecoder(stResp.Body).Decode(&stBody)
	if len(stBody.Starters) != 8 {
		t.Fatalf("expected 8 Vietnamese starter templates, got %d", len(stBody.Starters))
	}

	// 5. Static HTML pages return 200 and have ZERO inline style="..." attributes
	for _, route := range []string{"/", "/library", "/settings", "/showcase"} {
		pageReq, _ := http.NewRequest(http.MethodGet, ts.URL+route, nil)
		pageReq.Header.Set("X-Prompt-Studio-Token", token)
		pageResp, pErr := client.Do(pageReq)
		if pErr != nil {
			t.Fatalf("GET %s failed: %v", route, pErr)
		}
		htmlBytes, _ := io.ReadAll(pageResp.Body)
		_ = pageResp.Body.Close()
		if pageResp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", route, pageResp.StatusCode)
		}
		if strings.Contains(string(htmlBytes), `style="`) || strings.Contains(string(htmlBytes), `style='`) {
			t.Fatalf("route %s contains forbidden inline style attribute", route)
		}
	}
}
