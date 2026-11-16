# Prompt Studio — Vanilla CSS Design System (`CSS_DESIGN_SYSTEM.md`)

> **Triết lý cốt lõi:** Semantic HTML + Design Tokens + Reusable BEM Components + CSS `@layer`.  
> Không phụ thuộc bất kỳ CSS framework nào, không CSS-in-JS, **không xây dựng bản sao utility-first kiểu Tailwind CSS**, không dùng inline `style="..."`.

Trang tham chiếu trực quan (Live Interactive Showcase): mở route **`GET /showcase`** ([web/showcase.html](web/showcase.html)) khi chạy server.

---

## 1. Kiến trúc Phân tầng CSS (`@layer`)

Toàn bộ giao diện được quản lý cascade tường minh tại [web/style.css](web/style.css) theo thứ tự ưu tiên tăng dần:

```css
@layer tokens, reset, base, layout, components, pages, themes, utilities;
```

| Thứ tự | Tầng (`@layer`) | File nguồn | Trách nhiệm |
|---|---|---|---|
| 1 | `tokens` | `web/css/01-tokens.css` | Toàn bộ CSS Custom Properties ngữ nghĩa trong `:root` (Warm Graphite Dark mặc định theo `design/tokens.css`) |
| 2 | `reset` | `web/css/02-reset.css` | Chuẩn hóa box-sizing, margin, kế thừa font trên form controls |
| 3 | `base` | `web/css/03-base.css` | Kiểu mặc định cho thẻ HTML (`body`, `h1–h4`, `a`, `code`, `:focus-visible`) |
| 4 | `layout` | `web/css/04-layout.css` | Hệ thống bố cục vĩ mô: `.container`, `.grid` 12 cột, `.stack`, `.cluster`, `.sidebar-layout` |
| 5 | `components` | `web/css/05-components.css` | 10 họ UI Component tái sử dụng chuẩn BEM (`.btn`, `.input`, `.card`, `.modal`…) |
| 6 | `pages` | `web/css/06-pages.css`, `web/css/showcase.css` | Thành phần miền nghiệp vụ v2 (khung `.ai-frame` 4 trạng thái, `.result-card.is-streaming`, `.api-block`, `.assumption-chip`, `.compare-grid`, `.library-split`) |
| 7 | `themes` | `web/css/07-themes.css` | Bảng token Warm Paper Light (`html[data-theme="light"]`) — **nguồn duy nhất**. "Theo hệ thống" được `applyThemePreference()` ([web/api.js](web/api.js)) quy đổi thành `light`/`dark`, nên không còn khối `data-theme="system"` trùng lặp |
| 8 | `utilities` | `web/css/08-utilities.css` | Helper ngữ nghĩa (`.is-hidden`, `.sr-only`, `.text-truncate`, `.text-mono`) + thang chữ `.text-xs/sm/md/lg/xl`, `.font-medium/semibold` + `prefers-reduced-motion` |

**Gốc rem:** `html` giữ `font-size: 100%` (16px). Chỉ `body` dùng `--text-base` (14px), nhờ đó mọi token `rem` khớp đúng giá trị px ghi trong tài liệu.

**JS dùng chung:** [web/ui.js](web/ui.js) cung cấp `UI.toast`, `UI.confirm`, `UI.prompt`, `UI.announce`, `UI.bindMenu`, `UI.icons`. Cấm dùng `window.alert/confirm/prompt`.

**Tối ưu tải trang:** Các trang độc lập chỉ nạp thêm CSS đặc thù của trang đó khi cần (ví dụ `/showcase` nạp thêm `/css/showcase.css`, trong khi `/` và `/settings` không tải file này).

---

## 2. Hệ thống Design Tokens (v2 Warm Graphite & Warm Paper)

### 2.1. Colors (`01-tokens.css` & `07-themes.css`)
- **Surfaces:** `--color-bg` (`#111214` dark / `#F7F7F5` light), `--color-surface` (`#1A1C1F` / `#FFFFFF`), `--color-surface-subtle` (`#22252A` / `#EFEFEC`), `--color-surface-elevated` (`#262A30` / `#FFFFFF`), `--color-backdrop`
- **Brand & Actions:** `--color-primary` (`#7C9BFF` dark / `#3B5BDB` light, kèm `-hover`, `-active`, `--color-on-primary`), `--color-secondary`, `--color-accent`, `--color-accent-soft`, `--color-accent-border`, `--color-accent-text`
- **AI Active State Tokens (Quy tắc nghiêm ngặt):** `--ai-1` (`#B48CFF` / `#7950F2`) và `--ai-2` (`#5EEAD4` / `#0CA678`) **chỉ được phép dùng bên trong hiệu ứng khi AI đang phân tích hoặc đang stream** (`.ai-frame.is-busy`, `.ai-dot`, `.ai-caret`, `.result-card.is-streaming`), tuyệt đối không dùng cho nút bấm hay trang trí tĩnh.
- **Domain Highlight Tokens:** `--color-var-text` & `--color-var-bg` (tô nổi bật biến `{{ten_bien}}`), `--color-role-system` & `--color-role-user` (nhãn khối `system` / `user` cho `target = "api"`).
- **Typography:** `--color-text-primary` (`#ECECEA` / `#16171A`), `--color-text-body`, `--color-text-secondary`, `--color-text-muted`
- **Borders & Dividers:** `--color-border`, `--color-border-strong`, `--color-divider`
- **Semantic Status:** `--color-success` (`-soft`), `--color-warning` (`-soft`), `--color-error` (`-hover`, `-soft`, `-border`), `--color-on-danger` (chữ trên nền danger, đạt WCAG AA cả hai theme), `--color-info` (`-soft`)
- **Interactive States:** `--state-hover-bg`, `--state-active-bg`, `--state-disabled-opacity`, `--focus-ring`, `--focus-ring-color`, `--focus-border-color`

### 2.2. Spacing Scale (Cơ sở 4px)
Chỉ sử dụng thang đo từ `--space-1` (`4px` / `0.25rem`) đến `--space-16` (`64px` / `4rem`). Đặc biệt `--space-11` (`44px`) là chiều cao tối thiểu vùng chạm (touch target) chuẩn Mobile-First.

### 2.3. Typography Scale
- **Font:** `--font-sans` (`Inter`, system-ui), `--font-mono` (`ui-monospace`, SFMono, Menlo, Consolas — cho khối API user template, biến, phím tắt).
- **Sizes (6 cấp):** `--text-xs` (`12px` caption/meta), `--text-sm` (`13px` label, nút nhỏ), `--text-base` (`14px` body), `--text-md` (`15px` ô nhập yêu cầu), `--text-lg` (`18px` tiêu đề mục), `--text-xl` (`22px` tiêu đề trang — cỡ lớn nhất). Không dùng chữ hero cỡ lớn.
- **Weights:** `--font-regular` (`400`), `--font-medium` (`500`), `--font-semibold` (`600`), `--font-bold` (`700`).
- **Line Heights:** `--leading-tight` (`1.25`), `--leading-snug` (`1.4`), `--leading-normal` (`1.55`), `--leading-relaxed` (`1.65`).

### 2.4. Layout & Visual Tokens
- **Containers:** `--container-sm` (`680px`), `--container-md` (`860px`), `--container-lg` (`1180px`), `--container-xl` (`1440px`), `--sidebar-width` (`264px`), `--header-height` (`56px`).
- **Radius (phẳng 0–4px):** `--radius-xs` (`2px`: badge, code, var-pill), `--radius-sm` = `--radius-md` = `--radius-lg` (`4px`: control, card), `--radius-xl` (`6px`: modal), `--radius-full` (chỉ dành cho chip giả định/starter, status-dot, spinner).
- **Control heights:** `--control-sm` (`28px`), `--control-md` (`32px`: toolbar, chip, input nhỏ, mục menu), `--control-lg` (`36px`: input form, nút mặc định trên desktop), `--control-touch` (`44px`: tối thiểu dưới 1024px). Không dùng chiều cao nào khác.
- **Shadows:** chỉ dùng cho lớp nổi (dropdown, popover, modal, toast). Card và panel phân tách bằng border 1px, không đổ bóng.
- **Focus:** một kiểu duy nhất, `--focus-ring`. Không dùng glow mờ (blur).
- **Motion:** `--duration-fast` (`120ms`), `--duration-normal` (`200ms`), `--duration-slow` (`320ms`), `--ease-standard`.
- **Z-Index:** `--z-base` (`1`), `--z-sticky` (`10`), `--z-header` (`20`), `--z-dropdown` (`30`), `--z-drawer` (`40`), `--z-modal` (`50`), `--z-popover` (`60`), `--z-tooltip` (`70`).

---

## 3. Layout System & Mobile-First Responsive Standards

### 3.1. Breakpoints Chuẩn (`min-width`)
Thiết kế luôn bắt đầu từ **Mobile (`< 640px`)** và mở rộng dần lên màn hình lớn:
- `640px` (`--bp-sm`): Điện thoại ngang / Phablet (`.grid-cols-2` kích hoạt).
- `768px` (`--bp-md`): Tablet dọc (`.grid-cols-3`, `.col-span-1..12` và `.compare-grid` 2 cột kích hoạt).
- `1024px` (`--bp-lg`): Laptop / Tablet ngang (`.sidebar-layout` gắn cố định bên trái, `.library-split` 2 cột Master-Detail kích hoạt).
- `1280px` (`--bp-xl`): Desktop rộng.

### 3.2. Class Bố cục Dùng chung (`04-layout.css`)
- **Giới hạn khung nội dung:** `.container`, `.container--sm`, `.container--md`, `.container--lg`, `.container--xl`, `.container--fluid`.
- **Bố cục trang & phân vùng:** `.page-layout`, `.page-header`, `.section` (`--sm`, `--lg`), `.sidebar-layout` (`__sidebar`, `__backdrop`, `__main`).
- **Bố cục dọc (thay thế `margin-bottom` thủ công):** `.stack` (`--xs`, `--sm`, `--md`, `--lg`, `--xl`).
- **Bố cục ngang tự xuống dòng:** `.cluster` (`--xs`, `--sm`, `--md`, `--lg`, `--between`, `--end`, `--start`).
- **Lưới CSS Grid:**
  - `.grid` (12 cột `repeat(12, minmax(0, 1fr))`) đi kèm `.col-span-1` → `.col-span-12`, `.col-span-full`.
  - `.grid-cols-2`, `.grid-cols-3`, `.grid-cols-4`.
  - `.grid-auto-fit` (co giãn tự nhiên bằng `repeat(auto-fit, minmax(min(100%, var(--min-col-size)), 1fr))` không cần media query).

---

## 4. Quy ước Đặt tên Class (BEM) & Thành phần Tái sử dụng

### 4.1. Quy tắc Đặt tên
- **Block:** `.btn`, `.card`, `.field`, `.modal`, `.tabs`, `.badge`, `.table`, `.toolbar`, `.popover`, `.empty-state`, `.ai-frame`, `.result-card`
- **Element (`__`):** `.card__header`, `.card__title`, `.result-card__body`, `.api-block__role`, `.assumption-chip__label`
- **Modifier (`--`):** `.btn--primary`, `.btn--secondary`, `.btn--danger`, `.ai-frame--compact`
- **State Hooks (JS toggle):** `.is-active`, `.is-selected`, `.is-open`, `.is-focused`, `.is-busy`, `.is-streaming`, `.is-over`, `.is-hidden`
- **Cấm tuyệt đối:** Không dùng `#id` trong CSS; không dùng `!important`; không lồng selector quá 2 cấp; không viết inline `style="..."` trong HTML hoặc JS.

### 4.2. Danh mục 10 Họ Component Nền tảng (`05-components.css`) & Domain Components (`06-pages.css`)
1. **Button (5 biến thể):** `.btn` + `--primary | --secondary (= mặc định) | --ghost | --danger | --danger-outline`, kích thước `--sm | --lg | --icon | --block`. **Mỗi ngữ cảnh (card, pane, modal footer) chỉ một `--primary`.** Trạng thái bật/tắt (ví dụ "Đã lưu") dùng `aria-pressed="true"`, không tạo biến thể mới. Nút chỉ có icon phải có `aria-label` (và `data-tooltip` khi nghĩa không hiển nhiên). Icon là SVG inline 16px, không dùng ký tự Unicode/emoji.
2. **Form Controls:** `.field` (`__label`, `__hint`, `__error`), `.input`, `.textarea` (`--lg`), `.select` (`--sm`, `--lg`, `--error`), `.toggle-control`.
3. **Card, Result Card & 4-State AI Frame:** `.card` (`--subtle`, `--elevated`, `--compact`, `--interactive`), `.ai-frame` (`idle` / `.is-focused` viền nhấn + ring 1px / `.is-busy` xoay viền conic `--ai-1`/`--ai-2`, **không có quầng glow**), `.result-card` (`.is-streaming` quét tia sáng đáy + `.ai-caret`), `.api-block` (`system` / `user`), `.var-pill`.
4. **Modal, Dropdown & Toast:** `.modal` (`__backdrop`, `__dialog` `--sm`, `__header`, `__body`, `__footer`; hộp thoại xác nhận/nhập do `UI.confirm`/`UI.prompt` tạo), `.dropdown` (`__menu[role=menu]`, `__item[role=menuitem]` `--danger`, `__hint`, `__divider`; điều khiển bằng `UI.bindMenu`: ↑↓ Home End Esc), `.toast-region` + `.toast` (`--error`, `__text`; có hành động Hoàn tác).
5. **Tabs & Navigation:** `.tabs` (`__list`, `__tab`, `__panel`), `.segmented` (`--full`, `__btn`), `.nav-list`, `.nav-group-label`, `.nav-item` (là `<button>`; hàng có nút phụ dùng `.nav-item--split` + `.nav-item__link`). **Mọi phần tử bấm được trong danh sách phải là `<button>`/`<a>`, không dùng `<div>` gắn `click`.**
6. **Badge, Assumption Chip, Starter Chip & Alert:** `.badge` (bo 2px), `.starter-chip`, `.assumption-chip` + `.assumption-popover` (dạng pill có chủ ý: giả định là thứ *sửa được*, không phải lệnh), `.chip`, `.status-dot`, `.alert`.
7. **Table & Pagination:** `.table-wrap` (`overflow-x: auto`), `.table` (`--compact`, `--striped`), `.pagination` (`__item`).
8. **Toolbar & Filter Bar:** `.toolbar` (`--bordered`, `--sticky-mobile`, `__group`, `__spacer`), `.filter-bar`.
9. **Tooltip & Popover:** `[data-tooltip="..."]` (thuần CSS hỗ trợ hover & focus bàn phím), `.popover` (`__header`, `__footer`).
10. **Loading & Empty State:** `.spinner` (`--sm`, `--lg`), `.skeleton` (`--title`, `--text`, `--card`), `.ai-shimmer`, `.empty-state`.

---

## 5. Checklist Bắt buộc cho AI Agent & Developer khi phát triển UI

Trước khi hoàn thành bất kỳ PR/thay đổi giao diện nào trong dự án, **bắt buộc kiểm tra và đánh dấu đạt toàn bộ 11 mục sau**:

- [ ] **1. Design Tokens 100%:** Không có mã màu hex/rgb, khoảng cách pixel lẻ, font-size hay border-radius hardcode ngoài `:root` (`01-tokens.css` / `07-themes.css`). Chỉ dùng `--ai-1` và `--ai-2` khi AI đang bận hoặc stream.
- [ ] **2. Zero Inline Styles:** Không tồn tại thuộc tính `style="..."` trong file `.html`, template string JS, hoặc gán `element.style.*` trực tiếp trong `.js`.
- [ ] **3. Ưu tiên Layout System:** Sử dụng `.container`, `.stack`, `.cluster`, `.grid` / `.grid-auto-fit` thay vì cộng trừ `margin-top` / `margin-bottom` thủ công.
- [ ] **4. Tái sử dụng Component có sẵn:** Kiểm tra `05-components.css`, `06-pages.css` và `/showcase` trước khi định nghĩa class mới; không tạo CSS trùng lặp.
- [ ] **5. Tuân thủ BEM & `@layer`:** Đặt class đúng chuẩn `.block__element--modifier` + `.is-*` state; đặt rule vào đúng tầng `@layer`; không dùng `#id` selector; không dùng `!important`; không lồng quá 2 cấp.
- [ ] **6. Không Utility-First kiểu Tailwind:** Không thêm các class nguyên tử như `.mt-2`, `.px-4`, `.flex`, `.items-center` vào `08-utilities.css`.
- [ ] **7. Cô lập CSS đặc thù trang:** CSS trong `06-pages.css` (hoặc file trang riêng) không được ghi đè làm hỏng hành vi toàn cục của `.btn`, `.card`, `.input` ở trang khác.
- [ ] **8. Mobile-First & Chống tràn ngang:** Viết CSS gốc cho mobile trước, chỉ dùng `@media (min-width: 640px | 768px | 1024px | 1280px)`; luôn có `min-width: 0` trên cột Grid/Flex và `.table-wrap` cho bảng rộng.
- [ ] **9. Đa giao diện (Dark & Light):** Kiểm tra độ tương phản chữ/viền trên cả `data-theme="dark"` và `data-theme="light"`. "Theo hệ thống" quy về một trong hai.
- [ ] **10. Accessibility (A11y):** Mọi phần tử tương tác đều là `<button>`/`<a>`/form control (focus và kích hoạt được bằng bàn phím), có `:focus-visible`, nhãn `label`/`aria-label` đầy đủ, vùng chạm mobile `>= 44px`, và tôn trọng `prefers-reduced-motion: reduce`.
- [ ] **11. Không hộp thoại native & không lỗi im lặng:** Không dùng `window.alert/confirm/prompt`. Mọi lời gọi API do người dùng kích hoạt đều có `try/catch` và báo lỗi bằng `UI.toast(…, { variant: "error" })`. Mọi class dùng trong HTML/JS đều phải được định nghĩa trong CSS.
