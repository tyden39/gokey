---
phase: 1
title: "Active app and configuration"
status: pending
priority: P1
effort: "1.5h"
dependencies: []
---

# Phase 1: Active app and configuration

## Goal

Provide a tested, fail-open source of cached Hyprland app identity and runtime
strategy configuration.

## Files to create or modify

- Create `internal/hyprland/active_window.go`
- Create `internal/hyprland/active_window_test.go`
- Create `internal/appconfig/config.go`
- Create `internal/appconfig/config_test.go`
- Modify `main.go`
- Create `~/.config/gokey/app-strategies.json`

## Tasks

- [x] Query `j/activewindow` through Hyprland's Unix socket with deadlines and
      guaranteed connection close.
- [x] Cache `address`, `class`, `initialClass`, `pid`, and `xwayland` on input
      method activation; clear on deactivation.
- [x] Load the JSON mapping once at startup and resolve only known strategies,
      falling back to `virtual-backspace-commit` for all errors.
- [x] Configure `chromium` to use `surrounding-delete-commit`.

## Verification

- `go test ./internal/hyprland ./internal/appconfig`
- Missing socket, malformed JSON, unknown strategies, and empty app identity all
  preserve the default strategy.

## Risk and rollback

Hyprland IPC failure must not affect typing. Rollback is removal of the cached
identity fields and the user config; default injection remains unchanged.
