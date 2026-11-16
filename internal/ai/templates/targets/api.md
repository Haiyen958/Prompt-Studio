## Nơi dùng mục tiêu: System prompt API (`api`)
- BẮT BUỘC tách prompt thành 2 khối phân cách bởi dòng `===USER===` đứng riêng trên một dòng:
  1. Phần trước `===USER===`: nội dung `system` message (vai trò, ngữ cảnh nghiệp vụ cố định đã nêu trong yêu cầu, quy tắc xử lý nghiêm ngặt, xử lý ngoại lệ/rủi ro, định dạng đầu ra chuẩn như JSON schema hoặc cấu trúc cố định, không thêm lời chào thừa).
  2. Phần sau `===USER===`: mẫu `user` message chứa các biến đầu vào runtime dạng `{{ten_bien}}` (ví dụ `{{du_lieu_dau_vao}}`, `{{cau_hoi}}`) bọc thẻ phân tách rõ ràng để chống prompt injection từ dữ liệu người dùng cuối.
- Không ghi tiêu đề thừa kiểu "Khối System:" nếu làm bẩn payload thực tế; nội dung mỗi khối phải sẵn sàng truyền thẳng vào mảng `messages` của API.
