---
phase: 2
title: "Injection strategy"
status: pending
priority: P1
effort: "1.5h"
dependencies: [1]
---

# Phase 2: Injection strategy

## Goal

Add byte-correct atomic replacement for configured apps without weakening the
existing universal fallback.

## Files to create or modify

- Modify `main.go`
- Create `main_test.go`

## Tasks

- [x] Preserve the pre-transform word so the deleted rune suffix can be
      converted to its exact UTF-8 byte sequence.
- [x] Split injection into named `virtual-backspace-commit` and
      `surrounding-delete-commit` paths.
- [x] Require a valid, non-dirty surrounding snapshot, no selection, a valid
      cursor offset, and an exact suffix match before surrounding deletion.
- [x] Queue deletion and insertion, then call `Commit(serial)` once and mark the
      snapshot stale.
- [x] Fallback immediately to virtual Backspace on every failed precondition.
- [x] Keep word ending, cursor-word adoption, toggling, and Bamboo state
      unchanged.

## Verification

- Unit-test ASCII and multibyte suffix conversion, mismatch/refusal cases, and
  strategy selection.
- `go test ./...`

## Risk and rollback

The load-bearing assumption is that Chromium returns fresh surrounding text.
The observable failure signal is fallback debug logging; the predetermined
response is to retain the current strategy for that keystroke, never guess.
