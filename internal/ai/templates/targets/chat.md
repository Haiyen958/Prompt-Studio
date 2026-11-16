## Nơi dùng mục tiêu: Chat (`chat`)
- Thiết kế thành một prompt liền mạch để người dùng dán trực tiếp vào khung chat (ChatGPT, Claude, Gemini — môi trường không tự truy cập được kho mã nguồn cục bộ).
- Tích hợp trực tiếp các thông tin, bối cảnh, đối tượng và mục tiêu người dùng đã nêu trong `<user_request>` để dùng được ngay mà không sinh biến thừa.
- Chỉ đặt biến `{{ten_bien}}` (ở phần đầu hoặc cuối thật dễ thấy) cho dữ liệu người dùng thực sự cần dán thêm lúc chat (ví dụ `{{noi_dung_can_xu_ly}}`, `{{bien_ban_hop}}`) hoặc khi người dùng muốn tạo mẫu dùng nhiều lần. Nếu thiếu thông tin quan trọng mà Chat AI không tự kiểm chứng được, hãy chọn giả định hợp lý vào `assumptions` (đặt `"impact": "high"` kèm `"question"` nếu lựa chọn làm thay đổi hẳn kết quả hoặc độ an toàn).
