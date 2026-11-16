# Prompt Studio

Ứng dụng web chạy local: nhập yêu cầu bằng ngôn ngữ tự nhiên, nhận prompt có cấu trúc kèm giả định sửa được.

## Đọc trước khi code

- `docs/PLAN.md`: đặc tả đầy đủ (luồng, meta-prompt, API, schema, lộ trình).
- `design/`: bản HTML tĩnh của từng màn hình và `tokens.css`. Mở bằng trình duyệt để xem. Đây là tham chiếu về bố cục, màu và câu chữ, không phải code sản phẩm.

## Stack

- Go 1.22+, chỉ thư viện chuẩn và `modernc.org/sqlite` (không CGO).
- Frontend HTML/CSS/JS thuần trong `web/`, nhúng bằng `embed`. Không framework, không bước build.
- Router: `net/http` ServeMux với pattern có method.

## Lệnh

- Chạy: `go run ./cmd/prompt-studio`
- Test: `go test ./...`
- Evals (tốn token, cần đã cấu hình provider): `go run ./evals`

## Quy tắc bắt buộc

- Chỉ lắng nghe `127.0.0.1`. Kiểm tra `Host` và `Origin` ở mọi request.
- API key không bao giờ trả về frontend, không ghi log, không lưu trong SQLite.
- Template meta-prompt dùng delimiter `[[ ]]`, không dùng `{{ }}`.
- Đầu ra LLM là văn bản thường + `===META===` + JSON. Không dùng JSON mode. Meta hỏng thì vẫn hiển thị prompt.
- Không UPDATE nội dung `prompt_versions`; mọi thay đổi là một dòng mới.
- Màu lấy từ biến trong `design/tokens.css`. `--ai-1`, `--ai-2` chỉ dùng cho hiệu ứng khi AI đang chạy.
- Giao diện tiếng Việt; tên biến, hàm, bảng, endpoint bằng tiếng Anh.
- Không thêm tính năng ngoài `docs/PLAN.md` (không chọn model theo phiên, không tag, không chat).

## Thứ tự làm

Theo mục 10 của `docs/PLAN.md`, từng giai đoạn một, mỗi giai đoạn kết thúc khi đạt tiêu chí "Xong khi". Bắt đầu từ giai đoạn 1 (Nền tảng + Settings). Viết provider giả trước để test handler và SSE mà không tốn token.

Mục 11 của `docs/PLAN.md` liệt kê các quyết định còn mở; gặp chỗ phụ thuộc vào chúng thì hỏi lại thay vì tự chọn.
