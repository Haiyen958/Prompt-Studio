# Prompt Studio (v2)

Ứng dụng web cục bộ đóng gói thành một binary Go duy nhất (`127.0.0.1:7788`), giúp biến yêu cầu bằng ngôn ngữ tự nhiên thành prompt có cấu trúc rõ ràng, quản lý cây phiên bản bất biến (`prompt_versions`), chỉnh sửa giả định một chạm (**Editable Assumptions**), so sánh trực tiếp song song (**Side-by-Side Compare**), lưu trữ Thư viện & Bộ sưu tập hợp nhất với tìm kiếm tiếng Việt không dấu (SQLite FTS5), và giao diện tiếng Việt tinh gọn.

## Tính năng cốt lõi (v2)

- **Stream tức thì + Giả định có thể đổi (`===META===`):** Không hỏi vặn trước khi chạy; stream Markdown trực tiếp ngay từ ký tự đầu tiên kèm tối đa 4 thẻ giả định có gợi ý thay thế 1 chạm.
- **3 Đích sử dụng chuyên biệt (`chat`, `api`, `agent`):**
  - Với `target = "api"`, tự động tách 2 khối `system` và `user` qua `===USER===`, tô nổi bật biến `{{ten_bien}}` và hỗ trợ **Copy JSON messages** hoặc **Copy tất cả**.
- **So sánh trực tiếp song song (`POST /api/versions/{id}/compare`):** Stream đồng thời câu trả lời từ *Yêu cầu gốc* (`raw`) và *Prompt đã tạo* (`prompt`) với cùng mô hình và bộ biến thử nghiệm.
- **Lịch sử & Thư viện hợp nhất (`sessions` + `prompt_versions`):**
  - Mọi phiên bản đều bất biến (`append-only`, có `parent_id`), tự động lưu bản dở dang (`incomplete = 1`) khi bấm Dừng hoặc hết thời gian chờ.
  - Lưu 1 chạm vào Thư viện (mặc định bộ sưu tập hệ thống `"Chưa phân loại"`), dán prompt có sẵn kèm kiểm tra trùng lặp chuẩn hóa khoảng trắng (`409 Conflict`), tìm kiếm tiếng Việt bỏ dấu (`unicode61 remove_diacritics 2`), và Xuất/Nhập JSON sao lưu.
- **Bảo mật Localhost nghiêm ngặt:** Chỉ lắng nghe trên `127.0.0.1`, kiểm tra `Host` & `Origin` trên mọi request, xác thực phiên bằng cookie `HttpOnly` đổi từ `?token=...` lúc khởi động, và lưu API key riêng biệt trong `~/.prompt-studio/secrets.json` (`0600`) — không bao giờ lưu trong SQLite, không ghi log, không trả về frontend.

## Khởi chạy nhanh

```bash
# Biên dịch single binary vào ./bin/prompt-studio
go build -o ./bin/prompt-studio ./cmd/prompt-studio

# Chạy server local tại http://127.0.0.1:7788 (in kèm URL có ?token=...)
./bin/prompt-studio

# Hoặc chạy chế độ giả lập offline (không cần API key thật)
./bin/prompt-studio --mock
```

Các cờ dòng lệnh hỗ trợ:
- `--addr`: Địa chỉ lắng nghe cục bộ (mặc định `127.0.0.1:7788`).
- `--data-dir`: Thư mục lưu `prompt-studio.db` và `secrets.json` (mặc định `~/.prompt-studio`). Tự động sao lưu (`prompt-studio.v1.bak.db`) và nâng cấp nếu phát hiện CSDL v1 cũ.
- `--mock`: Sử dụng provider giả lập có sẵn để dùng thử và kiểm thử ngoại tuyến.

## Kiểm thử & Đánh giá chất lượng (Evals)

```bash
# Chạy toàn bộ Unit Test, Migration Test & E2E HTTP/SSE Test
go test -v -count=1 ./...

# Chạy bộ 20 kịch bản đánh giá Meta-Prompt + Blind A/B Judge
go run ./evals
```

## Tài liệu Kiến trúc & Hệ thống Thiết kế CSS

- **Kế hoạch sản phẩm & kỹ thuật v2:** [docs/PLAN.md](docs/PLAN.md)
- **Tài liệu Thiết kế giao diện gốc:** [design/README.md](design/README.md)
- **Hệ thống Vanilla CSS `@layer` (Zero Framework, Zero Inline Styles):** [CSS_DESIGN_SYSTEM.md](CSS_DESIGN_SYSTEM.md) — xem trực tiếp tại `http://127.0.0.1:7788/showcase`
- **Nhật ký thay đổi:** [docs/CHANGELOG.md](docs/CHANGELOG.md)
