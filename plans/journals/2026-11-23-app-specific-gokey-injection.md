---
title: App-specific GoKey injection
date: 2026-11-23
summary: Added cached Hyprland app detection and guarded atomic surrounding-text replacement for Chromium.
---

# App-specific GoKey injection

## What happened

GoKey previously used virtual Backspace plus `commit_string` for all live composition. Chromium on this Hyprland system has `initialClass=chromium`, so the implementation now caches active-window metadata on input-method activation and selects an injection strategy from `~/.config/gokey/app-strategies.json`.

## Decision

`virtual-backspace-commit` remains the fail-open default. Chromium uses `surrounding-delete-commit` only when the latest surrounding-text snapshot is valid, clean, selection-free, on the current serial, and contains the exact UTF-8 suffix directly before the byte cursor. Otherwise that key falls back to virtual Backspace. Config and Hyprland IPC are read outside the per-key path.

## Verification

`go test ./...`, `go vet ./...`, a Go 1.25 build, config JSON validation, and `git diff --check` passed. Hyprland reported a native-Wayland Chromium client with `initialClass=chromium`. The inline pre-landing review found no remaining issues after adding explicit ASCII coverage and removing a stale documentation reference.

## Next step

Run the documented Chromium page-field and omnibox smoke test with `GOKEY_DEBUG=1`. The automation session deliberately did not take over desktop focus or inject text into an open browser.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
