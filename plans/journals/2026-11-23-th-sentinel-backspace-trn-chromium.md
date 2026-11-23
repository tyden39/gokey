---
title: Thử sentinel Backspace trên Chromium
date: 2026-11-23
summary: Thêm fallback dấu chấm và hai Backspace để thử thay aa thành â trên Chromium.
---

# Thử sentinel Backspace trên Chromium

## What happened

Chromium không cung cấp surrounding-text snapshot an toàn nên GoKey phải fallback khỏi `delete_surrounding_text`.

## Decision

Fallback riêng cho app dùng strategy `surrounding-delete-commit` sẽ commit dấu `.`, gửi số Backspace cần thay cộng thêm một Backspace để xóa sentinel, rồi commit chuỗi Telex mới. Ví dụ `aa` phát `a`, `.`, Backspace, Backspace, `â`.

## Verification

`go test ./...` và `go build -o gokey .` chạy thành công với Go 1.25.0. Dev GoKey đã được khởi động lại để thử trực tiếp.

## Next steps

Gõ `aa` trong Chromium và kiểm tra log cùng kết quả hiển thị trước khi quyết định giữ hay bỏ workaround.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
