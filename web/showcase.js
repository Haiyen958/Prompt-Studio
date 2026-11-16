// showcase.js — Điều khiển tương tác cho trang Showcase Hệ thống CSS (/showcase)

(function () {
  const root = document.documentElement;
  const themeBtn = document.getElementById("themeToggleBtn");

  function syncThemeBtn() {
    const isLight = root.getAttribute("data-theme") === "light";
    themeBtn.textContent = isLight ? "Đổi giao diện Tối" : "Đổi giao diện Sáng";
  }

  function syncShowcaseAccents() {
    const active = root.getAttribute("data-accent") || "crimson";
    document.querySelectorAll("[data-accent]").forEach((btn) => {
      const selected = btn.getAttribute("data-accent") === active;
      btn.classList.toggle("is-selected", selected);
      btn.setAttribute("aria-pressed", selected ? "true" : "false");
    });
  }

  themeBtn.addEventListener("click", () => {
    const next = root.getAttribute("data-theme") === "light" ? "dark" : "light";
    root.setAttribute("data-theme", next);
    syncThemeBtn();
  });

  document.querySelectorAll("[data-accent]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const next = btn.getAttribute("data-accent") || "crimson";
      root.setAttribute("data-accent", next);
      syncShowcaseAccents();
    });
  });

  syncThemeBtn();
  syncShowcaseAccents();

  // Xem thử 4 trạng thái khung AI v2 (Idle / Focused / Analyzing / Streaming)
  const demoFrame = document.getElementById("demoAiFrame");
  const demoStatus = document.getElementById("demoAiStatus");
  const demoResultCard = document.getElementById("demoResultCard");
  const demoStreamCaret = document.getElementById("demoStreamCaret");

  document.querySelectorAll("[data-demo-ai]").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.querySelectorAll("[data-demo-ai]").forEach((b) => b.classList.remove("is-active"));
      btn.classList.add("is-active");
      const mode = btn.dataset.demoAi;
      demoFrame.classList.toggle("is-focused", mode === "focused");
      demoFrame.classList.toggle("is-busy", mode === "analyzing");
      demoStatus.classList.toggle("is-hidden", mode !== "analyzing");
      demoResultCard.classList.toggle("is-streaming", mode === "streaming");
      demoStreamCaret.classList.toggle("is-hidden", mode !== "streaming");
    });
  });

  // Modal Dialog preview
  const modal = document.getElementById("demoModal");
  const setModal = (open) => modal.classList.toggle("is-open", open);
  document.getElementById("openDemoModalBtn").addEventListener("click", () => setModal(true));
  document.getElementById("closeDemoModalBtn").addEventListener("click", () => setModal(false));
  document.getElementById("cancelDemoModalBtn").addEventListener("click", () => setModal(false));
  document.getElementById("confirmDemoModalBtn").addEventListener("click", () => setModal(false));
  document.getElementById("demoModalBackdrop").addEventListener("click", () => setModal(false));

  // Dropdown preview
  const dropdown = document.getElementById("demoDropdown");
  document.getElementById("toggleDropdownBtn").addEventListener("click", (e) => {
    e.stopPropagation();
    dropdown.classList.toggle("is-open");
  });
  document.addEventListener("click", () => dropdown.classList.remove("is-open"));

  // Escape key closes overlays
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      setModal(false);
      dropdown.classList.remove("is-open");
    }
  });

  // Tabs preview
  document.querySelectorAll("[data-demo-tab]").forEach((tab) => {
    tab.addEventListener("click", () => {
      document.querySelectorAll("[data-demo-tab]").forEach((t) => t.classList.remove("is-active"));
      tab.classList.add("is-active");
      ["tab1", "tab2", "tab3"].forEach((id) => {
        document.getElementById(id).classList.toggle("is-hidden", id !== tab.dataset.demoTab);
      });
    });
  });

  // Selectable Chips preview
  document.querySelectorAll(".demo-chip").forEach((chip) => {
    chip.addEventListener("click", () => chip.classList.toggle("is-selected"));
  });
})();
