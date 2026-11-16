// ui.js — Shared non-blocking UI primitives (loaded before app.js / settings.js).
// Replaces window.alert/confirm/prompt with themed, keyboard-accessible equivalents.
// Zero inline styles; composes .toast, .modal, .dropdown classes from 05-components.css.

(function () {
  const ICONS = {
    close:
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>',
    chevronDown:
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6"/></svg>',
    alertCircle:
      '<svg class="assumption-chip__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 8v4M12 16h.01"/></svg>',
    edit:
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 20h4L19 9l-4-4L4 16z"/></svg>',
  };

  const FOCUSABLE =
    'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

  function ensureEl(id, className, attrs) {
    let node = document.getElementById(id);
    if (node) return node;
    node = document.createElement("div");
    node.id = id;
    node.className = className;
    for (const [k, v] of Object.entries(attrs || {})) node.setAttribute(k, v);
    document.body.appendChild(node);
    return node;
  }

  // --- Screen reader announcements (copy feedback, background results) ---
  function announce(message) {
    const live = ensureEl("liveRegion", "sr-only", { role: "status", "aria-live": "polite" });
    live.textContent = "";
    // Next frame so repeated identical messages are still announced.
    requestAnimationFrame(() => {
      live.textContent = message;
    });
  }

  // --- Toasts ---
  // toast("Đã xóa…", { actionLabel: "Hoàn tác", onAction, duration: 5000 })
  // toast(err.message, { variant: "error" })  → stays 8s, has a close button.
  function toast(message, opts = {}) {
    const region = ensureEl("toastRegion", "toast-region");
    const variant = opts.variant || "info";
    const duration = opts.duration != null ? opts.duration : variant === "error" ? 8000 : 5000;

    const node = document.createElement("div");
    node.className = "toast" + (variant === "error" ? " toast--error" : "");
    node.setAttribute("role", variant === "error" ? "alert" : "status");

    const text = document.createElement("span");
    text.className = "toast__text";
    text.textContent = message;
    node.appendChild(text);

    let timer = null;
    const close = () => {
      clearTimeout(timer);
      node.remove();
    };

    if (opts.actionLabel && typeof opts.onAction === "function") {
      const action = document.createElement("button");
      action.type = "button";
      action.className = "btn btn--secondary btn--sm";
      action.textContent = opts.actionLabel;
      action.addEventListener("click", () => {
        close();
        opts.onAction();
      });
      node.appendChild(action);
    }

    const closeBtn = document.createElement("button");
    closeBtn.type = "button";
    closeBtn.className = "btn btn--ghost btn--icon";
    closeBtn.setAttribute("aria-label", "Đóng thông báo");
    closeBtn.innerHTML = ICONS.close;
    closeBtn.addEventListener("click", close);
    node.appendChild(closeBtn);

    region.appendChild(node);
    if (duration > 0) timer = setTimeout(close, duration);
    return { close };
  }

  // --- Themed dialog (confirm / prompt) ---
  let dialogEls = null;

  function buildDialog() {
    const root = document.createElement("div");
    root.className = "modal";
    root.setAttribute("role", "dialog");
    root.setAttribute("aria-modal", "true");
    root.setAttribute("aria-labelledby", "uiDialogTitle");
    root.setAttribute("aria-describedby", "uiDialogMessage");
    root.innerHTML = `
      <div class="modal__backdrop"></div>
      <form class="modal__dialog modal__dialog--sm stack stack--md" novalidate>
        <h2 class="modal__title" id="uiDialogTitle"></h2>
        <p class="modal__body" id="uiDialogMessage"></p>
        <input type="text" class="input" maxlength="200" aria-labelledby="uiDialogTitle">
        <footer class="modal__footer">
          <button type="button" class="btn btn--secondary" data-role="cancel"></button>
          <button type="submit" class="btn" data-role="confirm"></button>
        </footer>
      </form>`;
    document.body.appendChild(root);
    return {
      root,
      form: root.querySelector("form"),
      backdrop: root.querySelector(".modal__backdrop"),
      title: root.querySelector(".modal__title"),
      message: root.querySelector(".modal__body"),
      input: root.querySelector("input"),
      cancel: root.querySelector('[data-role="cancel"]'),
      confirm: root.querySelector('[data-role="confirm"]'),
    };
  }

  // Resolves to: true (confirm) | string (prompt value) | null (cancelled).
  function openDialog({ title, message, value, confirmLabel, cancelLabel, danger }) {
    if (!dialogEls) dialogEls = buildDialog();
    const d = dialogEls;
    const isPrompt = typeof value === "string";
    const returnFocus = document.activeElement;

    d.title.textContent = title || "";
    d.message.textContent = message || "";
    d.message.classList.toggle("is-hidden", !message);
    d.input.classList.toggle("is-hidden", !isPrompt);
    d.input.value = isPrompt ? value : "";
    d.cancel.textContent = cancelLabel || "Hủy";
    d.confirm.textContent = confirmLabel || "Đồng ý";
    d.confirm.className = "btn " + (danger ? "btn--danger" : "btn--primary");
    d.root.classList.add("is-open");

    return new Promise((resolve) => {
      const finish = (result) => {
        d.root.classList.remove("is-open");
        d.form.removeEventListener("submit", onSubmit);
        d.cancel.removeEventListener("click", onCancel);
        d.backdrop.removeEventListener("click", onCancel);
        d.root.removeEventListener("keydown", onKey);
        if (returnFocus && typeof returnFocus.focus === "function") returnFocus.focus();
        resolve(result);
      };
      const onSubmit = (e) => {
        e.preventDefault();
        if (isPrompt) {
          const v = d.input.value.trim();
          if (!v) {
            d.input.focus();
            return;
          }
          finish(v);
        } else {
          finish(true);
        }
      };
      const onCancel = () => finish(null);
      const onKey = (e) => {
        if (e.key === "Escape") {
          // Keep the app-level Esc handler (stop streaming) from firing too.
          e.stopPropagation();
          finish(null);
        } else if (e.key === "Tab") {
          trapTab(e, d.form);
        }
      };

      d.form.addEventListener("submit", onSubmit);
      d.cancel.addEventListener("click", onCancel);
      d.backdrop.addEventListener("click", onCancel);
      d.root.addEventListener("keydown", onKey);

      if (isPrompt) {
        d.input.focus();
        d.input.select();
      } else {
        (danger ? d.cancel : d.confirm).focus();
      }
    });
  }

  function trapTab(e, container) {
    const items = [...container.querySelectorAll(FOCUSABLE)].filter(
      (n) => !n.closest(".is-hidden")
    );
    if (!items.length) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  function confirmDialog(opts) {
    return openDialog({ ...opts, value: undefined }).then((r) => r === true);
  }

  function promptDialog(opts) {
    return openDialog({ ...opts, value: opts.value || "" });
  }

  // --- Dropdown menu (button[aria-haspopup="menu"] + .dropdown__menu[role="menu"]) ---
  function bindMenu(toggle, menu) {
    if (!toggle || !menu) return;
    const wrap = toggle.closest(".dropdown");
    const items = () =>
      [...menu.querySelectorAll('[role="menuitem"]')].filter(
        (n) => !n.classList.contains("is-hidden") && !n.disabled
      );
    const isOpen = () => wrap.classList.contains("is-open");

    const close = (restoreFocus) => {
      if (!isOpen()) return;
      wrap.classList.remove("is-open");
      toggle.setAttribute("aria-expanded", "false");
      if (restoreFocus) toggle.focus();
    };
    const open = (focusIndex) => {
      wrap.classList.add("is-open");
      toggle.setAttribute("aria-expanded", "true");
      const list = items();
      if (list.length) list[focusIndex === -1 ? list.length - 1 : 0].focus();
    };

    toggle.setAttribute("aria-haspopup", "menu");
    toggle.setAttribute("aria-expanded", "false");
    toggle.addEventListener("click", (e) => {
      e.stopPropagation();
      if (isOpen()) close(false);
      else open(0);
    });
    toggle.addEventListener("keydown", (e) => {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        open(0);
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        open(-1);
      }
    });

    menu.addEventListener("keydown", (e) => {
      const list = items();
      const idx = list.indexOf(document.activeElement);
      if (e.key === "ArrowDown") {
        e.preventDefault();
        list[(idx + 1) % list.length].focus();
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        list[(idx - 1 + list.length) % list.length].focus();
      } else if (e.key === "Home") {
        e.preventDefault();
        list[0].focus();
      } else if (e.key === "End") {
        e.preventDefault();
        list[list.length - 1].focus();
      } else if (e.key === "Escape") {
        e.stopPropagation();
        close(true);
      } else if (e.key === "Tab") {
        close(false);
      }
    });
    // Item handlers run first (bound on the item); then the menu closes.
    menu.addEventListener("click", (e) => {
      if (e.target.closest('[role="menuitem"]')) close(false);
    });
    document.addEventListener("click", (e) => {
      if (!wrap.contains(e.target)) close(false);
    });
  }

  window.UI = {
    icons: ICONS,
    announce,
    toast,
    confirm: confirmDialog,
    prompt: promptDialog,
    bindMenu,
  };
})();
