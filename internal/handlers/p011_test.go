package handlers_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/handlers"
	"prompt-studio/internal/storage"
	"prompt-studio/web"
)

// staleHighImpactProvider stubbornly keeps returning `"impact": "high"` even after the
// user refines an assumption, verifying `studio.Service` (`resolveConfirmedAssumption`)
// deterministically clears the high-impact flag on the confirmed assumption in v2.
type staleHighImpactProvider struct{}

func (staleHighImpactProvider) Ping(context.Context) error { return nil }

func (staleHighImpactProvider) Stream(_ context.Context, r ai.Request, onDelta func(string)) (ai.Usage, error) {
	isRefine := len(r.Messages) > 0 && strings.Contains(r.Messages[0].Content, "<current_prompt>")
	val := "Chỉ chạy kiểm tra dry-run trước khi xóa"
	if isRefine {
		val = "Đánh dấu xóa mềm (soft-delete) các bản ghi trùng"
	}
	out := "### Vai trò\nBạn là kỹ sư quản trị cơ sở dữ liệu phụ trách làm sạch dữ liệu an toàn.\n\n" +
		"### Nhiệm vụ\nThực hiện xử lý các bản ghi khách hàng bị trùng lặp theo chính sách: " + val + ".\n" +
		"===META===\n" +
		`{"intent":"Xóa dữ liệu trùng an toàn","assumptions":[` +
		`{"label":"An toàn dữ liệu","value":"` + val + `","alternatives":["Dry-run","Hard-delete"],"impact":"high","question":"Dữ liệu nào được phép xóa?"},` +
		`{"label":"Sao lưu","value":"Chưa xác nhận bảng backup","alternatives":["Dump SQL","Snapshot"],"impact":"high","question":"Lưu bản sao lưu ở đâu?"}` +
		`]}`
	onDelta(out)
	return ai.Usage{InputTokens: 60, OutputTokens: 80}, nil
}

// P011 (D-P011-1 & D-P011-2) & P012 (D-P012-1): The exact Payment API 500 duplicate-request
// starter must auto-fill explicit facts directly (zero `{{pham_vi_module}}`/`{{mo_ta_loi}}`
// variables), delegate repo inspection to the coding agent, and expose a high-impact
// clarification chip; meanwhile `POST /api/versions/{id}/compare` is unregistered (D-P012-1).
func TestAutoFillPayment500ZeroPlaceholdersAndCompareRemoved(t *testing.T) {
	const token = "p011-payment-autofill-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	reqBody := map[string]any{
		"request": "Tôi cần prompt để agent chẩn đoán nguyên nhân gốc rễ, viết regression test và sửa lỗi API thanh toán thỉnh thoảng trả 500 khi gửi trùng request.",
		"target":  "agent",
	}
	resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions", token, reqBody)
	events := parseSSEStream(t, resp.Body)
	resp.Body.Close()

	done, ok := findEvent(events, "done")
	if !ok {
		t.Fatalf("expected SSE done event, got %+v", events)
	}
	sid := done.Data["sessionId"].(string)
	vid := done.Data["versionId"].(string)

	detail := getDetail(t, client, ts.URL, token, sid)
	if len(detail.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(detail.Versions))
	}
	v1 := detail.Versions[0]
	if v1.TemplateVersion != ai.GenerateTemplateVersion {
		t.Fatalf("templateVersion = %q, want %q", v1.TemplateVersion, ai.GenerateTemplateVersion)
	}

	vars := ai.ExtractVariables(v1.Content)
	if len(vars) != 0 {
		t.Fatalf("expected 0 redundant placeholders for concrete agent bugfix request, got %v in:\n%s", vars, v1.Content)
	}
	lowerContent := strings.ToLower(v1.Content)
	if !strings.Contains(lowerContent, "api thanh toán") || !strings.Contains(lowerContent, "500") || !strings.Contains(lowerContent, "repository") {
		t.Fatalf("expected auto-filled module, HTTP 500 symptom, and repository inspection in prompt, got:\n%s", v1.Content)
	}

	if len(v1.Assumptions) == 0 || v1.Assumptions[0].Impact != "high" || v1.Assumptions[0].Question == "" {
		t.Fatalf("expected leading high-impact clarification assumption with question, got %+v", v1.Assumptions)
	}

	// D-P012-1: Compare endpoint and Compare UI elements are completely removed.
	cmpResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/versions/"+vid+"/compare", token, map[string]any{})
	cmpResp.Body.Close()
	if cmpResp.StatusCode != http.StatusNotFound && cmpResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 404/405 for removed /api/versions/{id}/compare endpoint, got %d", cmpResp.StatusCode)
	}
	for _, asset := range []string{"index.html", "app.js", "css/06-pages.css"} {
		b, err := fs.ReadFile(web.Assets, asset)
		if err != nil {
			t.Fatalf("read %s: %v", asset, err)
		}
		src := string(b)
		for _, forbidden := range []string{"compareView", "openCompareBtn", "compare-workspace", "/compare"} {
			if strings.Contains(src, forbidden) {
				t.Fatalf("%s still contains removed Compare artifact %q", asset, forbidden)
			}
		}
	}
}

// P011 (D-P011-2): Confirming a high-impact assumption (`source: "assumption"`) clears
// `impact` and `question` on ONLY the confirmed item (leaving other unconfirmed high-impact
// items intact), and Export/Import preserves `impact` and `question` across versions.
func TestAdaptiveClarificationConfirmClearsHighImpactAndSurvivesBackupRestore(t *testing.T) {
	const token = "p011-clarify-lifecycle-token"
	ts, svc, _ := setupTestEnv(t, token)
	useProvider(svc, staleHighImpactProvider{})
	client := ts.Client()

	genResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions", token, map[string]any{
		"request": "Viết prompt xóa toàn bộ dữ liệu khách hàng trùng lặp trong database production",
		"target":  "chat",
	})
	genEvents := parseSSEStream(t, genResp.Body)
	genResp.Body.Close()

	done, ok := findEvent(genEvents, "done")
	if !ok {
		t.Fatalf("expected done event, got %+v", genEvents)
	}
	sid := done.Data["sessionId"].(string)
	v1ID := done.Data["versionId"].(string)

	// Confirm the first assumption ("An toàn dữ liệu") via source="assumption".
	refineSession(t, client, ts.URL, token, sid, map[string]any{
		"fromVersionId": v1ID,
		"source":        "assumption",
		"instruction":   `Đổi giả định "An toàn dữ liệu" thành: Đánh dấu xóa mềm (soft-delete) các bản ghi trùng`,
	})

	d := getDetail(t, client, ts.URL, token, sid)
	if len(d.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(d.Versions))
	}
	v1, v2 := d.Versions[0], d.Versions[1]
	if len(v1.Assumptions) != 2 || v1.Assumptions[0].Impact != "high" || v1.Assumptions[1].Impact != "high" {
		t.Fatalf("v1 should retain both high-impact clarifications, got %+v", v1.Assumptions)
	}

	// In v2, "An toàn dữ liệu" was confirmed -> Impact & Question cleared;
	// "Sao lưu" was NOT confirmed -> remains Impact="high".
	var confirmedItem, remainingHigh *storage.AssumptionRecord
	for i := range v2.Assumptions {
		switch v2.Assumptions[i].Label {
		case "An toàn dữ liệu":
			confirmedItem = &v2.Assumptions[i]
		case "Sao lưu":
			remainingHigh = &v2.Assumptions[i]
		}
	}
	if confirmedItem == nil || confirmedItem.Impact != "" || confirmedItem.Question != "" {
		t.Fatalf("confirmed assumption in v2 must clear Impact & Question, got %+v", confirmedItem)
	}
	if remainingHigh == nil || remainingHigh.Impact != "high" || remainingHigh.Question == "" {
		t.Fatalf("unconfirmed high-impact assumption in v2 must remain high-impact, got %+v", remainingHigh)
	}

	// Export from server 1 and Import into fresh server 2 -> metadata survives intact.
	expResp := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/export", token, nil)
	var dump map[string]any
	if err := json.NewDecoder(expResp.Body).Decode(&dump); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	expResp.Body.Close()

	const token2 = "p011-restore-token"
	ts2, _, _ := setupTestEnv(t, token2)
	impResp := doJSONRequest(t, ts2.Client(), http.MethodPost, ts2.URL+"/api/import", token2, dump)
	impResp.Body.Close()
	if impResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/import status = %d", impResp.StatusCode)
	}

	restored := getDetail(t, ts2.Client(), ts2.URL, token2, sid)
	if restored.Versions[0].Assumptions[0].Impact != "high" || restored.Versions[0].Assumptions[0].Question == "" {
		t.Fatalf("restored v1 lost high-impact metadata: %+v", restored.Versions[0].Assumptions)
	}
	for _, a := range restored.Versions[1].Assumptions {
		if a.Label == "An toàn dữ liệu" && a.Impact != "" {
			t.Fatalf("restored v2 re-introduced cleared impact: %+v", a)
		}
	}
}

// P011 (D-P011-1): Explicit reusable-template requests on `agent` and runtime `api` prompts
// still emit `{{ten_bien}}` variables as required.
func TestReusableTemplateAndAPITargetEmitRuntimeVariables(t *testing.T) {
	const token = "p011-reusable-vars-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	for _, tc := range []struct {
		name    string
		target  string
		request string
	}{
		{
			name:    "reusable agent template",
			target:  "agent",
			request: "Tạo mẫu prompt template dùng nhiều lần để agent sửa lỗi cho module bất kỳ trong dự án.",
		},
		{
			name:    "api system+user split",
			target:  "api",
			request: "Tôi cần system prompt cho API trích xuất thông tin hóa đơn sang JSON.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions", token, map[string]any{
				"request": tc.request,
				"target":  tc.target,
			})
			events := parseSSEStream(t, resp.Body)
			resp.Body.Close()
			done, ok := findEvent(events, "done")
			if !ok {
				t.Fatalf("expected done event, got %+v", events)
			}
			d := getDetail(t, client, ts.URL, token, done.Data["sessionId"].(string))
			vars := ai.ExtractVariables(d.Versions[0].Content)
			if len(vars) == 0 {
				t.Fatalf("expected runtime {{variables}} for %s, got none in:\n%s", tc.name, d.Versions[0].Content)
			}
		})
	}
}

// P011 (D-P011-2 & D-P011-3): Executes embedded `web/ui.js` + `web/app.js` in a pure-Node
// VM sandbox against real IDs from `web/index.html` to verify Target Hint detection,
// manual-override suppression, high-impact assumption chips/popovers, and XSS escaping.
func TestFrontendClarificationAndTargetHintJSWithNode(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping JS VM verification")
	}

	readWeb := func(name string) string {
		t.Helper()
		b, rErr := fs.ReadFile(web.Assets, name)
		if rErr != nil {
			t.Fatalf("read %s: %v", name, rErr)
		}
		return string(b)
	}

	uiJS := readWeb("ui.js")
	appJS := readWeb("app.js")
	indexHTML := readWeb("index.html")

	harness := `
const assert = require("node:assert/strict");
const vm = require("node:vm");

const indexHTML = ` + toJSString(indexHTML) + `;
const uiJS = ` + toJSString(uiJS) + `;
const appJS = ` + toJSString(appJS) + `;

class ClassList {
  constructor(initial = "") {
    this.set = new Set(initial.split(/\s+/).filter(Boolean));
  }
  add(...cls) { cls.forEach((c) => c && this.set.add(c)); }
  remove(...cls) { cls.forEach((c) => this.set.delete(c)); }
  contains(c) { return this.set.has(c); }
  toggle(c, force) {
    const on = force !== undefined ? Boolean(force) : !this.set.has(c);
    if (on) this.set.add(c); else this.set.delete(c);
    return on;
  }
  toString() { return [...this.set].join(" "); }
}

class FakeElement {
  constructor(tag = "div", id = "", cls = "") {
    this.tagName = tag.toUpperCase();
    this.id = id;
    this.classList = new ClassList(cls);
    this.attrs = {};
    this.listeners = {};
    this.children = [];
    this.parentNode = null;
    this._text = "";
    this._html = "";
    this.value = "";
    this.disabled = false;
  }
  get className() { return this.classList.toString(); }
  set className(v) { this.classList = new ClassList(String(v || "")); }
  get textContent() { return this._text; }
  set textContent(v) { this._text = String(v ?? ""); this._html = this._text; }
  get innerHTML() { return this._html; }
  set innerHTML(v) {
    this._html = String(v ?? "");
    if (this._html === "") this.children = [];
  }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k] ?? null; }
  removeAttribute(k) { delete this.attrs[k]; }
  appendChild(child) {
    child.parentNode = this;
    this.children.push(child);
    return child;
  }
  remove() {
    if (this.parentNode) {
      this.parentNode.children = this.parentNode.children.filter((c) => c !== this);
    }
  }
  addEventListener(type, fn) {
    (this.listeners[type] ||= []).push(fn);
  }
  removeEventListener(type, fn) {
    this.listeners[type] = (this.listeners[type] || []).filter((f) => f !== fn);
  }
  dispatch(type, extra = {}) {
    const evt = {
      type,
      target: this,
      preventDefault() {},
      stopPropagation() {},
      ...extra,
    };
    for (const fn of this.listeners[type] || []) fn(evt);
  }
  click() { this.dispatch("click"); }
  focus() { FakeDoc.activeElement = this; }
  select() {}
  setSelectionRange() {}
  contains(node) {
    let cur = node;
    while (cur) { if (cur === this) return true; cur = cur.parentNode; }
    return false;
  }
  closest(sel) {
    if (sel.startsWith(".")) {
      const want = sel.slice(1);
      let cur = this;
      while (cur) { if (cur.classList && cur.classList.contains(want)) return cur; cur = cur.parentNode; }
    }
    return null;
  }
  querySelectorAll(sel) {
    const out = [];
    const visit = (n) => {
      for (const c of n.children || []) {
        if (sel === "[data-instruction]" && c.attrs["data-instruction"]) out.push(c);
        else if (sel === '[role="menuitem"]' && c.attrs["role"] === "menuitem") out.push(c);
        else if (sel === "input" && c.tagName === "INPUT") out.push(c);
        else if (sel.startsWith(".") && c.classList.contains(sel.slice(1))) out.push(c);
        visit(c);
      }
    };
    visit(this);
    return out;
  }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
}

const byId = new Map();
const tagIdClassRe = /<([a-zA-Z0-9]+)\b([^>]*)>/g;
let m;
while ((m = tagIdClassRe.exec(indexHTML)) !== null) {
  const tag = m[1];
  const attrs = m[2];
  const idMatch = /\bid="([^"]+)"/.exec(attrs);
  if (!idMatch) continue;
  const clsMatch = /\bclass="([^"]+)"/.exec(attrs);
  const elem = new FakeElement(tag, idMatch[1], clsMatch ? clsMatch[1] : "");
  if (idMatch[1] === "targetSelect") elem.value = "chat";
  byId.set(idMatch[1], elem);
}

const docBody = new FakeElement("body", "body", "");
const docRoot = new FakeElement("html", "html", "");
const FakeDoc = {
  body: docBody,
  documentElement: docRoot,
  activeElement: null,
  listeners: {},
  getElementById(id) { return byId.get(id) || null; },
  createElement(tag) {
    const e = new FakeElement(tag);
    const origSetAttr = e.setAttribute.bind(e);
    e.setAttribute = (k, v) => { origSetAttr(k, v); if (k === "id") byId.set(v, e); };
    return e;
  },
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); },
  dispatch(type, evt = {}) { for (const fn of this.listeners[type] || []) fn(evt); },
};

let sessionMockDetail = null;
const sandbox = {
  window: null,
  document: FakeDoc,
  navigator: { clipboard: { writeText: async () => {} } },
  location: { pathname: "/", search: "" },
  history: { pushState() {} },
  URLSearchParams,
  Blob: class {},
  URL: { createObjectURL: () => "blob:1", revokeObjectURL() {} },
  requestAnimationFrame: (cb) => cb(),
  setTimeout,
  clearTimeout,
  applyThemePreference() {},
  API: {
    async request(method, path) {
      if (path === "/api/settings") return { provider: "mock", model: "mock", theme: "dark", defaultTarget: "chat", mockMode: true };
      if (path === "/api/starters") return [];
      if (path === "/api/collections") return { collections: [], totalSaved: 0 };
      if (path.startsWith("/api/sessions?")) return { sessions: [], hasMore: false, nextOffset: 0 };
      if (path.startsWith("/api/sessions/") && sessionMockDetail) return sessionMockDetail;
      return {};
    },
  },
  console,
};
sandbox.window = sandbox;
sandbox.window.location = sandbox.location;
sandbox.window.history = sandbox.history;
sandbox.window.addEventListener = () => {};
sandbox.document.body.appendChild = (el) => {
  if (el.id) byId.set(el.id, el);
  docBody.children.push(el);
  return el;
};

vm.createContext(sandbox);
vm.runInContext(uiJS, sandbox);
vm.runInContext(appJS, sandbox);

(async () => {
  FakeDoc.dispatch("DOMContentLoaded");
  await new Promise((r) => setTimeout(r, 20));

  const reqInput = byId.get("requestInput");
  const targetSelect = byId.get("targetSelect");
  const hintBtn = byId.get("targetHintBtn");

  // 1. Payment API 500 request mentioning both "agent" and "API thanh toán":
  // must suggest "agent" ("Agent lập trình"), NEVER "api"!
  targetSelect.value = "chat";
  reqInput.value = "Tôi cần prompt để agent chẩn đoán nguyên nhân gốc rễ, viết regression test và sửa lỗi API thanh toán thỉnh thoảng trả 500 khi gửi trùng request.";
  reqInput.dispatch("input");
  assert.equal(hintBtn.classList.contains("is-hidden"), false, "targetHintBtn should show for agent request when target=chat");
  assert.match(hintBtn.textContent, /Agent lập trình/, "must suggest Agent lập trình, not API");

  // Clicking targetHintBtn switches targetSelect to "agent", hides hint, and announces via liveRegion.
  hintBtn.click();
  assert.equal(targetSelect.value, "agent");
  assert.equal(hintBtn.classList.contains("is-hidden"), true);
  assert.match(byId.get("liveRegion").textContent, /Agent lập trình/);

  // 2. Clearing input resets manual override; typing API system prompt suggests "api".
  reqInput.value = "";
  reqInput.dispatch("input");
  targetSelect.value = "chat";
  reqInput.value = "Tôi cần system prompt cho API trích xuất hóa đơn GTGT sang JSON chuẩn.";
  reqInput.dispatch("input");
  assert.equal(hintBtn.classList.contains("is-hidden"), false);
  assert.match(hintBtn.textContent, /System prompt API/);

  // Manual dropdown change suppresses hint until draft is cleared.
  targetSelect.value = "chat";
  targetSelect.dispatch("change");
  assert.equal(hintBtn.classList.contains("is-hidden"), true);

  // 2b. Vietnamese Unicode diacritic boundary check ("kiểm thử" + "repository" without "agent" word).
  reqInput.value = "";
  reqInput.dispatch("input");
  reqInput.value = "Viết kiểm thử unit test cho repository hiện tại khi đầu vào rỗng.";
  reqInput.dispatch("input");
  assert.equal(hintBtn.classList.contains("is-hidden"), false, "Unicode Vietnamese phrase 'kiểm thử' + 'repository' must trigger agent hint");
  assert.match(hintBtn.textContent, /Agent lập trình/);

  // 3. High-impact assumption rendering, popover question, and XSS escaping.
  sessionMockDetail = {
    session: { id: "s1", title: "Test", origin: "generated", request: "Xóa trùng DB", target: "chat" },
    versions: [{
      id: "v1",
      sessionId: "s1",
      seq: 1,
      source: "generate",
      target: "chat",
      content: "### Vai trò\nChuyên gia DB.",
      assumptions: [
        {
          label: "An toàn <img src=x onerror=alert(1)>",
          value: "Dry-run <script>evil()</script>",
          alternatives: ["Soft-delete", "Transaction"],
          impact: "high",
          question: "Dữ liệu nào được phép xóa?",
        },
        {
          label: "Giọng điệu",
          value: "Chuyên nghiệp",
          alternatives: ["Ngắn gọn"],
        },
      ],
    }],
  };
  sandbox.location.search = "?session=s1";
  // Trigger session load via init() path or history click: load history with s1 and click it.
  sandbox.API.request = async (method, path) => {
    if (path.startsWith("/api/sessions?")) return { sessions: [{ id: "s1", title: "Test", openedAt: Date.now() }], hasMore: false };
    if (path === "/api/sessions/s1") return sessionMockDetail;
    return {};
  };
  byId.get("historySearchInput").dispatch("input");
  await new Promise((r) => setTimeout(r, 220));
  const histRowBtn = byId.get("historyList").querySelector(".nav-item__link");
  assert.ok(histRowBtn, "expected history row button");
  histRowBtn.click();
  await new Promise((r) => setTimeout(r, 20));

  const assumpSec = byId.get("assumptionsSection");
  assert.equal(assumpSec.classList.contains("assumptions-section--needs-review"), true);
  assert.equal(byId.get("assumptionsTitleMain").textContent, "Cần xác nhận 1 điểm quan trọng");

  const chips = byId.get("assumptionsList").querySelectorAll(".assumption-chip");
  assert.equal(chips.length, 2);
  assert.equal(chips[0].classList.contains("assumption-chip--high"), true);
  assert.equal(chips[1].classList.contains("assumption-chip--high"), false);
  assert.equal(chips[0].innerHTML.includes("<script>"), false, "must escape XSS in value");
  assert.equal(chips[0].innerHTML.includes("<img"), false, "must escape XSS in label");
  assert.equal(chips[0].innerHTML.includes("Cần rõ"), true);

  // Open popover on high-impact chip -> displays question & current safe default.
  chips[0].click();
  const openedPop = byId.get("assumptionsList").querySelector(".assumption-popover--high");
  assert.ok(openedPop, "expected .assumption-popover--high to open on click");
  const qEl = openedPop.querySelector(".assumption-popover__question");
  const curEl = openedPop.querySelector(".assumption-popover__current");
  assert.equal(qEl.textContent, "Dữ liệu nào được phép xóa?");
  assert.match(curEl.textContent, /Đang áp dụng: Dry-run/);

  // Pressing Escape closes the assumption popover and restores focus to the trigger chip.
  FakeDoc.dispatch("keydown", { key: "Escape" });
  assert.equal(byId.get("assumptionsList").querySelector(".assumption-popover"), null);
  const reRenderedChips = byId.get("assumptionsList").querySelectorAll(".assumption-chip");
  assert.equal(FakeDoc.activeElement, reRenderedChips[0]);

  // 4. Library inline rename: split card button + header inline form + Escape cancel + PATCH save.
  let patchedTitle = "";
  sandbox.API.request = async (method, path, body) => {
    if (path === "/api/collections") return { collections: [{ id: "col_uncategorized", name: "Chưa phân loại", isSystem: true, count: 1 }], totalSaved: 1 };
    if (path.startsWith("/api/sessions?saved=1")) {
      return { sessions: [{ id: "s1", title: sessionMockDetail.session.title, origin: "generated", savedAt: Date.now(), openedAt: Date.now() }], hasMore: false };
    }
    if (path.startsWith("/api/sessions?")) return { sessions: [{ id: "s1", title: sessionMockDetail.session.title, openedAt: Date.now() }], hasMore: false };
    if (method === "PATCH" && path === "/api/sessions/s1") {
      patchedTitle = body.title;
      sessionMockDetail.session.title = body.title;
      return sessionMockDetail.session;
    }
    if (path === "/api/sessions/s1") return sessionMockDetail;
    return {};
  };

  byId.get("navLibraryBtn").click();
  await new Promise((r) => setTimeout(r, 30));

  const libCard = byId.get("libraryItemsList").children[0];
  assert.ok(libCard && libCard.tagName === "DIV", "library-item must be a split DIV container (no nested button)");
  const cardActions = libCard.querySelector(".library-item__actions");
  assert.ok(cardActions && cardActions.children[0], "expected quick rename button on library item card");

  // Click quick rename button on card -> opens inline rename form with current title selected.
  cardActions.children[0].click();
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(byId.get("libTitleRow").classList.contains("is-hidden"), true);
  assert.equal(byId.get("libInlineRenameForm").classList.contains("is-hidden"), false);
  assert.equal(byId.get("libInlineRenameInput").value, "Test");

  // Pressing Escape cancels inline rename and restores focus to libInlineRenameBtn.
  FakeDoc.dispatch("keydown", { key: "Escape" });
  assert.equal(byId.get("libInlineRenameForm").classList.contains("is-hidden"), true);
  assert.equal(FakeDoc.activeElement, byId.get("libInlineRenameBtn"));

  // Reopen via header "Đổi tên" button and submit new title.
  byId.get("libInlineRenameBtn").click();
  byId.get("libInlineRenameInput").value = "  Prompt kiểm thử thanh toán mới  ";
  byId.get("libInlineRenameForm").dispatch("submit");
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(patchedTitle, "Prompt kiểm thử thanh toán mới");
  assert.equal(byId.get("libDetailTitle").textContent, "Prompt kiểm thử thanh toán mới");
  assert.equal(byId.get("libInlineRenameForm").classList.contains("is-hidden"), true);
  process.exit(0);
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
`

	cmd := exec.Command(nodeBin, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("Node JS VM frontend verification failed: %v\nOutput:\n%s", runErr, string(out))
	}
}

func toJSString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// P013 (D-P013-2): Renaming a saved prompt in the Library validates non-empty and max-200-rune
// titles (`400 Bad Request`) and immediately updates the accent-insensitive FTS5 search index.
func TestLibraryPromptRenameValidationAndFTSSearch(t *testing.T) {
	const token = "p013-rename-library-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	sid, vid := generateSession(t, client, ts.URL, token)
	saveResp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/sessions/"+sid+"/save", token, map[string]any{
		"versionId": vid,
		"title":     "Tên ban đầu",
	})
	saveResp.Body.Close()
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /save status = %d", saveResp.StatusCode)
	}

	// 1. Blank/whitespace title -> 400 Bad Request.
	blankResp := doJSONRequest(t, client, http.MethodPatch, ts.URL+"/api/sessions/"+sid, token, map[string]any{
		"title": "   ",
	})
	blankResp.Body.Close()
	if blankResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank title, got %d", blankResp.StatusCode)
	}

	// 2. Title > 200 runes -> 400 Bad Request.
	longResp := doJSONRequest(t, client, http.MethodPatch, ts.URL+"/api/sessions/"+sid, token, map[string]any{
		"title": strings.Repeat("ă", 201),
	})
	longResp.Body.Close()
	if longResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for >200-rune title, got %d", longResp.StatusCode)
	}

	// 3. Valid rename -> 200 OK + FTS5 accent-insensitive lookup hits new title immediately.
	newTitle := "Kiểm định cổng thanh toán quốc tế"
	okResp := doJSONRequest(t, client, http.MethodPatch, ts.URL+"/api/sessions/"+sid, token, map[string]any{
		"title": "  " + newTitle + "  ",
	})
	var updated storage.Session
	if err := json.NewDecoder(okResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode PATCH response: %v", err)
	}
	okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK || updated.Title != newTitle {
		t.Fatalf("PATCH status=%d title=%q, want 200 %q", okResp.StatusCode, updated.Title, newTitle)
	}

	searchResp := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/sessions?saved=1&q=kiem+dinh+cong+thanh+toan", token, nil)
	var listOut struct {
		Sessions []storage.Session `json:"sessions"`
	}
	_ = json.NewDecoder(searchResp.Body).Decode(&listOut)
	searchResp.Body.Close()
	if len(listOut.Sessions) != 1 || listOut.Sessions[0].ID != sid || listOut.Sessions[0].Title != newTitle {
		t.Fatalf("FTS search after rename failed: %+v", listOut.Sessions)
	}
}

// P015 (D-P015-1, D-P015-2): Verifies all 6 curated accent presets persist via GET/PUT /api/settings,
// invalid accents return 400 invalid_settings (field="accent"), and Export/Import preserves & sanitizes accent.
func TestAccentPresetSettingsAndBackupRoundtrip(t *testing.T) {
	const token = "p015-accent-settings-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	// 1. Default accent is "crimson".
	getResp := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/settings", token, nil)
	var s handlers.SettingsResponse
	if err := json.NewDecoder(getResp.Body).Decode(&s); err != nil {
		t.Fatalf("decode GET /api/settings: %v", err)
	}
	getResp.Body.Close()
	if s.Accent != "crimson" {
		t.Fatalf("default accent = %q, want crimson", s.Accent)
	}

	// 2. Partial PUT /api/settings accepts all 6 valid accents (case-insensitive normalized).
	for _, acc := range []string{"indigo", "ocean", "emerald", "violet", "amber", "crimson", "OCEAN"} {
		putResp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/settings", token, map[string]any{
			"accent": acc,
		})
		var updated handlers.SettingsResponse
		if err := json.NewDecoder(putResp.Body).Decode(&updated); err != nil {
			t.Fatalf("decode PUT /api/settings (%s): %v", acc, err)
		}
		putResp.Body.Close()
		want := strings.ToLower(acc)
		if putResp.StatusCode != http.StatusOK || updated.Accent != want {
			t.Fatalf("PUT accent=%q -> status=%d accent=%q, want 200 %q", acc, putResp.StatusCode, updated.Accent, want)
		}
	}

	// 3. Invalid accent returns 400 invalid_settings with field="accent" and leaves stored accent ("ocean") intact.
	badResp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/settings", token, map[string]any{
		"accent": "neon-lime",
	})
	var badBody map[string]any
	_ = json.NewDecoder(badResp.Body).Decode(&badBody)
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest || badBody["code"] != "invalid_settings" || badBody["field"] != "accent" {
		t.Fatalf("expected 400 invalid_settings field=accent, got status=%d body=%+v", badResp.StatusCode, badBody)
	}

	// 4. Export includes "accent": "ocean", and Import sanitizes unknown accents while accepting valid ones.
	expResp := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/export", token, nil)
	var bundle storage.BackupBundle
	if err := json.NewDecoder(expResp.Body).Decode(&bundle); err != nil {
		t.Fatalf("decode /api/export: %v", err)
	}
	expResp.Body.Close()
	if bundle.Settings["accent"] != "ocean" {
		t.Fatalf("exported settings.accent = %q, want ocean", bundle.Settings["accent"])
	}

	// Import valid accent "emerald" -> updates to "emerald".
	bundle.Settings["accent"] = "emerald"
	impResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/import", token, bundle)
	impResp.Body.Close()
	if impResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/import status = %d", impResp.StatusCode)
	}

	// Import invalid accent "bogus" -> ignored by sanitizer, keeping "emerald".
	bundle.Settings["accent"] = "bogus"
	impBadResp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/import", token, bundle)
	impBadResp.Body.Close()

	afterResp := doJSONRequest(t, client, http.MethodGet, ts.URL+"/api/settings", token, nil)
	var after handlers.SettingsResponse
	_ = json.NewDecoder(afterResp.Body).Decode(&after)
	afterResp.Body.Close()
	if after.Accent != "emerald" {
		t.Fatalf("accent after sanitized import = %q, want emerald", after.Accent)
	}
}

// P015 (D-P015-2, D-P015-3): Verifies in Node VM that api.js applies cached theme/accent on boot
// and settings.js updates data-accent + sends a decoupled partial PUT /api/settings on swatch click.
func TestSettingsAccentPickerAndBootAppearanceInNodeVM(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping JS VM verification")
	}

	apiBytes, err := fs.ReadFile(web.Assets, "api.js")
	if err != nil {
		t.Fatalf("read api.js: %v", err)
	}
	uiBytes, err := fs.ReadFile(web.Assets, "ui.js")
	if err != nil {
		t.Fatalf("read ui.js: %v", err)
	}
	settingsJSBytes, err := fs.ReadFile(web.Assets, "settings.js")
	if err != nil {
		t.Fatalf("read settings.js: %v", err)
	}
	settingsHTMLBytes, err := fs.ReadFile(web.Assets, "settings.html")
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}

	harness := `
const assert = require("node:assert/strict");
const vm = require("node:vm");

const settingsHTML = ` + toJSString(string(settingsHTMLBytes)) + `;
const apiJS = ` + toJSString(string(apiBytes)) + `;
const uiJS = ` + toJSString(string(uiBytes)) + `;
const settingsJS = ` + toJSString(string(settingsJSBytes)) + `;

class FakeClassList {
  constructor(initial = "") {
    this.set = new Set(initial.split(/\s+/).filter(Boolean));
  }
  add(...cls) { cls.forEach((c) => c && this.set.add(c)); }
  remove(...cls) { cls.forEach((c) => this.set.delete(c)); }
  toggle(c, force) {
    const on = force === undefined ? !this.set.has(c) : Boolean(force);
    if (on) this.set.add(c); else this.set.delete(c);
    return on;
  }
  contains(c) { return this.set.has(c); }
}

class FakeElement {
  constructor(tagName, id = "", cls = "", attrs = {}) {
    this.tagName = tagName.toUpperCase();
    this.id = id;
    this.classList = new FakeClassList(cls);
    this.attrs = { ...attrs };
    this.children = [];
    this.listeners = {};
    this.value = "";
    this.disabled = false;
    this.textContent = "";
    this.innerHTML = "";
  }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null; }
  appendChild(child) { this.children.push(child); return child; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  dispatch(type, evt = {}) {
    const e = { target: this, preventDefault() {}, stopPropagation() {}, ...evt };
    for (const fn of this.listeners[type] || []) fn(e);
  }
  click() { this.dispatch("click"); }
  focus() {}
  querySelectorAll(sel) {
    const out = [];
    const visit = (n) => {
      for (const c of n.children || []) {
        if (sel === "[data-accent]" && c.attrs["data-accent"]) out.push(c);
        else if (sel === "[data-preset-url]" && c.attrs["data-preset-url"]) out.push(c);
        visit(c);
      }
    };
    visit(this);
    return out;
  }
}

const byId = new Map();
const tagRe = /<([a-zA-Z0-9]+)\b([^>]*)>/g;
let m;
while ((m = tagRe.exec(settingsHTML)) !== null) {
  const tag = m[1];
  const rawAttrs = m[2];
  const idMatch = /\bid="([^"]+)"/.exec(rawAttrs);
  if (!idMatch) continue;
  const clsMatch = /\bclass="([^"]+)"/.exec(rawAttrs);
  const el = new FakeElement(tag, idMatch[1], clsMatch ? clsMatch[1] : "");
  byId.set(idMatch[1], el);
}

// Attach the 6 swatch buttons under #accentPicker from settingsHTML
const swatchRe = /<button\b([^>]*\bdata-accent="([^"]+)"[^>]*)>/g;
const pickerEl = byId.get("accentPicker");
let sm;
while ((sm = swatchRe.exec(settingsHTML)) !== null) {
  const raw = sm[1];
  const acc = sm[2];
  const clsMatch = /\bclass="([^"]+)"/.exec(raw);
  const pressedMatch = /\baria-pressed="([^"]+)"/.exec(raw);
  const btn = new FakeElement("button", "", clsMatch ? clsMatch[1] : "", {
    "data-accent": acc,
    "aria-pressed": pressedMatch ? pressedMatch[1] : "false",
  });
  pickerEl.appendChild(btn);
}

const storageMap = new Map([["ps_theme", "light"], ["ps_accent", "violet"]]);
const docRoot = new FakeElement("html", "", "", { "data-theme": "dark", "data-accent": "crimson" });
const docBody = new FakeElement("body");
const FakeDoc = {
  documentElement: docRoot,
  body: docBody,
  listeners: {},
  getElementById(id) { return byId.get(id) || null; },
  createElement(tag) { return new FakeElement(tag); },
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); },
  dispatch(type, evt = {}) { for (const fn of this.listeners[type] || []) fn(evt); },
};

let putPayloads = [];
const sandbox = {
  window: null,
  document: FakeDoc,
  localStorage: {
    getItem: (k) => (storageMap.has(k) ? storageMap.get(k) : null),
    setItem: (k, v) => storageMap.set(k, String(v)),
  },
  matchMedia: () => ({ matches: false }),
  fetch: async (path, opts = {}) => {
    const method = opts.method || "GET";
    const body = opts.body ? JSON.parse(opts.body) : undefined;
    let payload = { ok: true, models: [] };
    if (method === "GET" && path === "/api/settings") {
      payload = {
        provider: "openai",
        model: "gpt-4o-mini",
        theme: "dark",
        accent: "indigo",
        promptLanguage: "vi",
        hasKey: false,
      };
    } else if (method === "PUT" && path === "/api/settings") {
      putPayloads.push(body);
      payload = {
        provider: body.provider || "openai",
        model: body.model || "gpt-4o-mini",
        theme: body.theme || "dark",
        accent: body.accent || "indigo",
        promptLanguage: body.promptLanguage || "vi",
        hasKey: false,
      };
    }
    return {
      ok: true,
      status: 200,
      json: async () => payload,
    };
  },
  location: { href: "/settings" },
  setTimeout,
  clearTimeout,
  console,
};
sandbox.window = sandbox;

vm.createContext(sandbox);
vm.runInContext(apiJS, sandbox);

// 1. Synchronous boot in api.js applies cached ps_theme="light" and ps_accent="violet"!
assert.equal(docRoot.getAttribute("data-theme"), "light");
assert.equal(docRoot.getAttribute("data-accent"), "violet");

vm.runInContext(uiJS, sandbox);
vm.runInContext(settingsJS, sandbox);

(async () => {
  FakeDoc.dispatch("DOMContentLoaded");
  await new Promise((r) => setTimeout(r, 20));

  // 2. loadSettings() syncs server theme="dark" & accent="indigo" + marks indigo swatch selected.
  assert.equal(docRoot.getAttribute("data-theme"), "dark");
  assert.equal(docRoot.getAttribute("data-accent"), "indigo");
  const swatches = pickerEl.querySelectorAll("[data-accent]");
  assert.equal(swatches.length, 6);
  const indigoBtn = swatches.find((b) => b.getAttribute("data-accent") === "indigo");
  const oceanBtn = swatches.find((b) => b.getAttribute("data-accent") === "ocean");
  assert.equal(indigoBtn.getAttribute("aria-pressed"), "true");
  assert.equal(indigoBtn.classList.contains("is-selected"), true);

  // 3. Simulate mid-edit AI provider switch (Anthropic with empty modelInput), then click Ocean swatch:
  // decoupled partial PUT /api/settings must succeed immediately without triggering model warning!
  byId.get("providerSelect").value = "anthropic";
  byId.get("providerSelect").dispatch("change");
  assert.equal(byId.get("modelInput").value, "");

  oceanBtn.click();
  await new Promise((r) => setTimeout(r, 20));

  assert.equal(docRoot.getAttribute("data-accent"), "ocean");
  assert.equal(storageMap.get("ps_accent"), "ocean");
  assert.equal(oceanBtn.getAttribute("aria-pressed"), "true");
  assert.equal(indigoBtn.getAttribute("aria-pressed"), "false");
  assert.equal(putPayloads.length, 1);
  assert.deepEqual(putPayloads[0], {
    theme: "dark",
    accent: "ocean",
    promptLanguage: "vi",
  });

  process.exit(0);
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
`

	cmd := exec.Command(nodeBin, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("Node VM settings accent verification failed: %v\nOutput:\n%s", runErr, string(out))
	}
}
