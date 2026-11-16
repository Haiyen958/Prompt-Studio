// settings.js — Prompt Studio v2 Settings & Backup Controller
// Zero inline styles; strictly toggles semantic BEM classes.

(function () {
  const el = (id) => document.getElementById(id);

  let hasStoredKey = false;
  let replacingKey = false;
  let keyTestedOk = false;
  let currentAccent = "crimson";

  function syncAccentPicker(accent) {
    const clean = String(accent || "crimson").trim().toLowerCase();
    currentAccent = clean || "crimson";
    const picker = el("accentPicker");
    if (!picker) return;
    picker.querySelectorAll("[data-accent]").forEach((btn) => {
      const active = btn.getAttribute("data-accent") === currentAccent;
      btn.classList.toggle("is-selected", active);
      btn.setAttribute("aria-pressed", active ? "true" : "false");
    });
  }

  function show(node, visible) {
    if (!node) return;
    node.classList.toggle("is-hidden", !visible);
  }

  function showFeedback(kind, htmlMessage) {
    const banner = el("settingsFeedbackBanner");
    banner.className = `alert alert--${kind}`;
    banner.innerHTML = htmlMessage;
    show(banner, true);
  }

  function escapeHTML(str) {
    return String(str || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  // numberOr parses a numeric input, using fallback only for empty/non-numeric values.
  function numberOr(raw, fallback) {
    const s = String(raw ?? "").trim();
    const n = Number(s);
    return s !== "" && Number.isFinite(n) ? n : fallback;
  }

  const MODEL_HINT_DEFAULT =
    "Bấm “Tải danh sách model” để chọn từ các model provider đang cung cấp, hoặc gõ Model ID.";
  // Anthropic retires model IDs, so it has no default: the user picks from the live list.
  const MODEL_PLACEHOLDERS = {
    openai: "gpt-4o-mini",
    anthropic: "Chọn model — bấm “Tải danh sách model”",
    "openai-compatible": "llama3.1",
    mock: "mock-prompt-engineer-v2",
  };
  let savedProvider = "";
  let savedModel = "";

  function setModelHint(text) {
    el("modelHint").textContent = text || MODEL_HINT_DEFAULT;
  }

  function fillModelSuggestions(models) {
    const list = el("modelSuggestions");
    list.innerHTML = "";
    for (const m of models) {
      const opt = document.createElement("option");
      opt.value = m.id;
      if (m.name && m.name !== m.id) opt.label = m.name;
      list.appendChild(opt);
    }
  }

  // loadModels fetches the provider catalog for the values currently in the form (the
  // stored key is only used by the server for the saved provider + Base URL).
  async function loadModels({ quiet = false } = {}) {
    const btn = el("loadModelsBtn");
    btn.disabled = true;
    btn.textContent = "Đang tải…";
    try {
      const res = await API.request("POST", "/api/settings/models", {
        provider: el("providerSelect").value,
        baseUrl: el("baseUrlInput").value.trim(),
        apiKey: el("apiKeyInput").value.trim(),
        timeoutSec: Number(el("timeoutInput").value) || 15,
      });
      if (!res.ok) {
        fillModelSuggestions([]);
        setModelHint(quiet ? "" : `Không tải được danh sách model: ${res.error || "lỗi không xác định"}`);
        return;
      }
      const models = res.models || [];
      fillModelSuggestions(models);
      const current = el("modelInput").value.trim();
      if (!current && models.length > 0) el("modelInput").focus();
      setModelHint(
        models.length === 0
          ? "Provider không trả về model nào — hãy gõ Model ID."
          : `Đã tải ${models.length} model. Bấm vào ô Model ID để chọn${
              current && !models.some((m) => m.id === current) ? " (model hiện tại không có trong danh sách)" : ""
            }.`
      );
    } catch (err) {
      if (!quiet) setModelHint(`Không tải được danh sách model: ${err.message}`);
    } finally {
      btn.disabled = false;
      btn.textContent = "Tải danh sách model";
    }
  }

  function syncBaseUrlPresets() {
    const presetsWrap = el("baseUrlPresets");
    if (!presetsWrap) return;
    const isCompat = el("providerSelect").value === "openai-compatible";
    show(presetsWrap, isCompat);
    if (!isCompat) return;
    const norm = el("baseUrlInput").value.trim().replace(/\/+$/, "").toLowerCase();
    presetsWrap.querySelectorAll("[data-preset-url]").forEach((chip) => {
      const target = (chip.getAttribute("data-preset-url") || "").replace(/\/+$/, "").toLowerCase();
      const active = Boolean(norm && norm === target);
      chip.classList.toggle("is-selected", active);
      chip.setAttribute("aria-pressed", active ? "true" : "false");
    });
  }

  function syncProviderUI() {
    const prov = el("providerSelect").value;
    const isCompat = prov === "openai-compatible";
    el("modelInput").placeholder = MODEL_PLACEHOLDERS[prov] || "";
    el("loadModelsBtn").disabled = false;
    el("baseUrlInput").disabled = !isCompat;
    if (!isCompat && prov !== "openai-compatible") {
      el("baseUrlInput").placeholder =
        prov === "anthropic" ? "https://api.anthropic.com/v1" : "https://api.openai.com/v1";
    } else {
      el("baseUrlInput").placeholder = "http://127.0.0.1:11434/v1";
    }
    syncBaseUrlPresets();
  }

  function syncKeyDisplay(keyHint) {
    if (hasStoredKey && !replacingKey) {
      el("keyHintText").textContent = keyHint || "••••••••••";
      show(el("storedKeyBar"), true);
      show(el("keyInputGroup"), false);
      show(el("cancelReplaceKeyBtn"), false);
    } else {
      show(el("storedKeyBar"), false);
      show(el("keyInputGroup"), true);
      show(el("cancelReplaceKeyBtn"), hasStoredKey && replacingKey);
    }
  }

  async function loadSettings() {
    try {
      const s = await API.request("GET", "/api/settings");
      currentAccent = s.accent || "crimson";
      applyThemePreference(s.theme || "dark", currentAccent);
      syncAccentPicker(currentAccent);

      el("providerSelect").value = s.provider || "openai";
      el("baseUrlInput").value = s.baseUrl || "";
      el("modelInput").value = s.model || "";
      savedProvider = s.provider || "openai";
      savedModel = s.model || "";
      el("temperatureInput").value = s.temperature ?? 0.4;
      el("maxTokensInput").value = s.maxTokens ?? 2000;
      el("timeoutInput").value = s.timeoutSec ?? 60;
      el("promptLanguageSelect").value = s.promptLanguage || "match";
      el("themeSelect").value = s.theme || "dark";

      hasStoredKey = Boolean(s.hasKey);
      replacingKey = false;
      keyTestedOk = false;
      el("apiKeyInput").value = "";

      syncProviderUI();
      syncKeyDisplay(s.keyHint);
      // Offer the live list right away when a key is already configured (silent on failure).
      if (s.hasKey && savedProvider !== "mock") loadModels({ quiet: true });
    } catch (err) {
      showFeedback("error", `<span>Không thể tải cấu hình: ${escapeHTML(err.message)}</span>`);
    }
  }

  async function handleTestConnection() {
    const btn = el("testConnectionBtn");
    btn.disabled = true;
    btn.textContent = "Đang kiểm tra…";

    try {
      const res = await API.request("POST", "/api/settings/test", {
        provider: el("providerSelect").value,
        baseUrl: el("baseUrlInput").value.trim(),
        model: el("modelInput").value.trim(),
        apiKey: el("apiKeyInput").value.trim(),
        timeoutSec: Number(el("timeoutInput").value) || 15,
      });

      if (res.ok) {
        keyTestedOk = true;
        showFeedback(
          "success",
          `<span><strong>Kết nối thành công.</strong> ${res.latencyMs ?? 0} ms.</span>`
        );
      } else {
        keyTestedOk = false;
        showFeedback(
          "warning",
          `<span><strong>Kiểm tra kết nối thất bại:</strong> ${escapeHTML(res.error || "Lỗi không xác định")}. Bạn vẫn có thể bấm Lưu nếu muốn cấu hình trước.</span>`
        );
      }
    } catch (err) {
      keyTestedOk = false;
      showFeedback("error", `<span>${escapeHTML(err.message)}</span>`);
    } finally {
      btn.disabled = false;
      btn.textContent = "Test Connection";
    }
  }

  async function saveGeneralPreferences() {
    try {
      const updated = await API.request("PUT", "/api/settings", {
        theme: el("themeSelect").value,
        accent: currentAccent,
        promptLanguage: el("promptLanguageSelect").value,
      });
      currentAccent = updated.accent || currentAccent;
      applyThemePreference(updated.theme || "dark", currentAccent);
      syncAccentPicker(currentAccent);
    } catch (err) {
      showFeedback("error", `<span>Lỗi khi lưu tùy chọn giao diện: ${escapeHTML(err.message)}</span>`);
    }
  }

  async function saveAllSettings(e) {
    if (e) e.preventDefault();
    const payload = {
      provider: el("providerSelect").value,
      baseUrl: el("baseUrlInput").value.trim(),
      model: el("modelInput").value.trim(),
      // 0 is a valid temperature: only fall back when the field is empty or not a number.
      temperature: numberOr(el("temperatureInput").value, 0.4),
      maxTokens: Number(el("maxTokensInput").value) || 2000,
      timeoutSec: Number(el("timeoutInput").value) || 60,
      promptLanguage: el("promptLanguageSelect").value,
      theme: el("themeSelect").value,
      accent: currentAccent,
      tested: keyTestedOk,
    };

    const typedKey = el("apiKeyInput").value.trim();
    if (typedKey) {
      payload.apiKey = typedKey;
    }

    // A model belongs to one provider: a new provider (and Anthropic always) needs a chosen model.
    if (!payload.model && payload.provider !== "mock" && (payload.provider !== savedProvider || payload.provider === "anthropic")) {
      showFeedback("warning", "<span><strong>Hãy chọn model</strong> cho provider này — bấm “Tải danh sách model”.</span>");
      el("modelInput").focus();
      return;
    }

    try {
      const updated = await API.request("PUT", "/api/settings", payload);
      currentAccent = updated.accent || currentAccent;
      applyThemePreference(updated.theme || "dark", currentAccent);
      syncAccentPicker(currentAccent);
      hasStoredKey = Boolean(updated.hasKey);
      replacingKey = false;
      el("apiKeyInput").value = "";
      savedProvider = updated.provider || payload.provider;
      savedModel = updated.model || payload.model;
      syncKeyDisplay(updated.keyHint);

      if (updated.warning) {
        showFeedback("warning", `<span>${escapeHTML(updated.warning)}</span>`);
      } else {
        showFeedback("success", `<span><strong>Đã lưu cấu hình thành công.</strong></span>`);
      }
    } catch (err) {
      showFeedback("error", `<span>Lỗi khi lưu: ${escapeHTML(err.message)}</span>`);
    }
  }

  async function handleDeleteKey() {
    const ok = await UI.confirm({
      title: "Xóa API key?",
      message: "Key đã lưu trên máy này sẽ bị xóa. Studio sẽ không tạo được prompt cho tới khi bạn nhập key mới.",
      confirmLabel: "Xóa key",
      danger: true,
    });
    if (!ok) return;
    try {
      await API.request("DELETE", "/api/settings/key");
      hasStoredKey = false;
      replacingKey = false;
      keyTestedOk = false;
      el("apiKeyInput").value = "";
      syncKeyDisplay("");
      showFeedback("info", `<span>Đã xóa API key khỏi máy.</span>`);
    } catch (err) {
      showFeedback("error", `<span>${escapeHTML(err.message)}</span>`);
    }
  }

  function handleExportData() {
    window.location.href = "/api/export";
  }

  async function handleImportFile(file) {
    if (!file) return;
    try {
      const raw = await file.text();
      const parsed = JSON.parse(raw);
      const res = await API.request("POST", "/api/import", parsed);
      await loadSettings();
      showFeedback(
        "success",
        `<span><strong>Khôi phục dữ liệu thành công:</strong> ${res.sessions || 0} phiên và ${res.collections || 0} collection.</span>`
      );
    } catch (err) {
      showFeedback("error", `<span>Không thể nhập dữ liệu: ${escapeHTML(err.message)}</span>`);
    } finally {
      el("importFileInput").value = "";
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    loadSettings();

    el("providerSelect").addEventListener("change", () => {
      keyTestedOk = false;
      // The previous provider's model and catalog do not apply to the new one
      // (switching back to the saved provider restores its saved model).
      el("modelInput").value = el("providerSelect").value === savedProvider ? savedModel : "";
      fillModelSuggestions([]);
      setModelHint("");
      syncProviderUI();
    });

    el("baseUrlInput").addEventListener("input", () => {
      keyTestedOk = false;
      syncBaseUrlPresets();
    });

    if (el("baseUrlPresets")) {
      el("baseUrlPresets")
        .querySelectorAll("[data-preset-url]")
        .forEach((chip) => {
          chip.addEventListener("click", () => {
            const url = chip.getAttribute("data-preset-url") || "";
            const suggestedModel = chip.getAttribute("data-preset-model") || "";
            el("baseUrlInput").value = url;
            if (!el("modelInput").value.trim() && suggestedModel) {
              el("modelInput").value = suggestedModel;
            }
            keyTestedOk = false;
            fillModelSuggestions([]);
            setModelHint("");
            syncBaseUrlPresets();
            el("modelInput").focus();
          });
        });
    }

    el("apiKeyInput").addEventListener("input", () => {
      keyTestedOk = false;
    });

    el("loadModelsBtn").addEventListener("click", () => loadModels());

    el("replaceKeyBtn").addEventListener("click", () => {
      replacingKey = true;
      keyTestedOk = false;
      syncKeyDisplay("");
      el("apiKeyInput").focus();
    });

    el("cancelReplaceKeyBtn").addEventListener("click", () => {
      replacingKey = false;
      el("apiKeyInput").value = "";
      loadSettings();
    });

    el("deleteKeyBtn").addEventListener("click", handleDeleteKey);
    el("testConnectionBtn").addEventListener("click", handleTestConnection);
    el("aiSettingsForm").addEventListener("submit", saveAllSettings);

    // Auto-save general preferences (theme, accent, promptLanguage) immediately when changed
    el("themeSelect").addEventListener("change", () => {
      applyThemePreference(el("themeSelect").value, currentAccent);
      saveGeneralPreferences();
    });
    el("promptLanguageSelect").addEventListener("change", () => {
      saveGeneralPreferences();
    });
    if (el("accentPicker")) {
      el("accentPicker")
        .querySelectorAll("[data-accent]")
        .forEach((btn) => {
          btn.addEventListener("click", () => {
            const nextAccent = btn.getAttribute("data-accent") || "crimson";
            syncAccentPicker(nextAccent);
            applyThemePreference(undefined, nextAccent);
            saveGeneralPreferences();
          });
        });
    }

    el("exportDataBtn").addEventListener("click", handleExportData);
    el("importDataBtn").addEventListener("click", () => el("importFileInput").click());
    el("importFileInput").addEventListener("change", (e) => {
      const f = e.target.files && e.target.files[0];
      if (f) handleImportFile(f);
    });
  });
})();
