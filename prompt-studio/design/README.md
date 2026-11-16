# Phác thảo giao diện

HTML tĩnh, mở trực tiếp bằng trình duyệt. Màu được viết thẳng trong file; khi code, dùng biến trong `tokens.css`.
Dữ liệu trong các màn hình (tên collection, số liệu token, nội dung hai cột So sánh, `[model-id]`) chỉ là mẫu.

| File | Màn hình | Giai đoạn |
| --- | --- | --- |
| `studio.html` | Studio có kết quả: chip giả định, thanh công cụ, thông báo đã lưu, ô tinh chỉnh | 3, 5, 6 |
| `compare.html` | So sánh yêu cầu gốc và prompt | 4 |
| `library.html` | Thư viện: collection, danh sách phiên đã lưu, chi tiết | 6 |
| `add-prompt.html` | Hộp dán prompt vào thư viện | 6 |
| `settings.html` | Settings và banner "Chưa kết nối AI" | 1 |
| `input-effects.html` | 4 trạng thái hiệu ứng của khung nhập, có sẵn CSS | 3 |
| `studio-empty.html` | Studio trống với chip mẫu khởi đầu | 7 |
| `result-api.html` | Kết quả 2 khối system/user cho nơi dùng API | 7 |

Trong `studio.html`, dropdown "Dùng cho" thuộc giai đoạn 7; trước đó hàng điều khiển chỉ có nút Tạo prompt.
Hiệu ứng "đang phân tích" của khung nhập: thêm class `busy` vào phần tử `.ai-frame`.
