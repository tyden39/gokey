---
title: Remove preedit and word restore modes
date: 2026-11-23
summary: GoKey now uses only direct Telex composition and the Ctrl+Shift Vietnamese toggle.
---

# Remove preedit and word restore modes

## What happened

Removed the optional preedit path, its Ctrl+Shift+Space shortcut, and the one-shot Space-then-Backspace word restore behavior from `main.go`. Removed the restore-only engine API and tests, while preserving cursor-word editing through replay verification in `ResumeWord`.

Updated the README, project guidance, architecture, roadmap, deployment notes, and codebase summary so direct composition is the only documented mode.

## Decision

GoKey has one live composition path: Bamboo computes the Telex result, then `apply` sends synthetic Backspaces and commits the changed suffix. Ctrl+Shift remains the only GoKey mode shortcut.

## Verification

`git diff --check` and `bash -n run.sh` passed. Go tests and formatting could not run because the Go toolchain is not installed in the current PATH.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
