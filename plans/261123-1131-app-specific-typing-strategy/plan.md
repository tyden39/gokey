---
title: "App-specific typing strategy"
description: "Detect the active Hyprland app and use an atomic surrounding-text injection strategy for configured applications such as Chromium."
status: in-progress
priority: P1
effort: "4h"
branch: main
tags: [feature, wayland, input-method]
blockedBy: []
blocks: []
created: 2026-11-23
---

# App-specific typing strategy

## Overview

GoKey will cache the active Hyprland application's identity when input-method
focus activates, load an app-to-strategy mapping from the user's config, and
use atomic `DeleteSurroundingText + CommitString` for Chromium when the current
surrounding-text snapshot is safe. Every uncertainty falls back to the existing
virtual-Backspace strategy so typing remains available.

## Outcome contract

- **Outcome:** Chromium uses `surrounding-delete-commit`; other apps retain
  `virtual-backspace-commit` unless configured otherwise.
- **Constraints:** no per-key Hyprland IPC or config reads; byte-correct UTF-8
  deletion; stale cursor state never drives surrounding deletion; typing fails
  open to the current strategy.
- **Non-goals:** changing Bamboo/Telex rules, browser-tab detection, plugins,
  VNI, or application identity as a security boundary.
- **Acceptance:** config contains `chromium`; app identity is cached; safe
  snapshots use one input-method commit; invalid snapshots demonstrably
  fallback; focused tests and docs cover the public behavior.

## Phases

| # | Phase | Status |
|---|---|---|
| 1 | [Active app and configuration](./phase-01-start.md) | Done |
| 2 | [Injection strategy](./phase-02-injection-strategy.md) | Done |
| 3 | [Verification and documentation](./phase-03-verification-and-docs.md) | In progress |

## Dependencies

- Accepted design: `plans/reports/brainstorm-261123-1112-app-specific-typing-strategy.md`
- Detection research: `plans/reports/research-261123-1100-hyprland-active-app-detection.md`

<!-- slug: app-specific-typing-strategy -->
