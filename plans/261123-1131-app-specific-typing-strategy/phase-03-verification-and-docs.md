---
phase: 3
title: "Verification and documentation"
status: pending
priority: P1
effort: "1h"
dependencies: [2]
---

# Phase 3: Verification and documentation

## Goal

Prove fallback safety, document configuration and deployment behavior, and
leave the accepted plan synchronized with evidence.

## Files to modify

- Modify `README.md`, `CLAUDE.md`, and owning files under `docs/`
- Modify `plans/261123-1131-app-specific-typing-strategy/*`

## Tasks

- [x] Run formatting, focused tests, full tests, vet, and build.
- [x] Review app detection, UTF-8 byte boundaries, stale snapshot handling,
      config failure behavior, and existing direct-composition regressions.
- [x] Document the config path, strategy names, restart requirement, Chromium
      default, fallback conditions, and Hyprland dependency.
- [ ] If a runnable binary is available, perform a manual Chromium smoke test
      with debug logging; otherwise report the exact environment blocker.

The binary builds and Hyprland reports a native-Wayland Chromium window with
`initialClass=chromium`. Live typing remains a user-controlled check: this
automation session did not change desktop focus or inject text into the user's
open browser.

## Verification

- `gofmt -w` on changed Go files
- `go test ./...`
- `go vet ./...`
- `go build -o gokey .`
- `git diff --check`
- Config JSON parses and contains `chromium: surrounding-delete-commit`.

## Rollback

Set Chromium back to `virtual-backspace-commit` or remove its mapping. The
binary will then use the pre-existing injection behavior without code removal.
