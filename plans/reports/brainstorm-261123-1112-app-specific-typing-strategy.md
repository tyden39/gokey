# App-specific typing strategy contract

Created: 2026-11-23 11:12 Asia/Ho_Chi_Minh

## Outcome

GoKey selects a text-injection strategy from the active Hyprland application's
`initialClass`. Chromium is explicitly present in one runtime configuration
file. The existing typing behavior is named and specified first so a second
Chromium strategy can be designed without changing Bamboo's Telex rules.

## Constraints

- Resolve and cache the active app on input-method `activate`; do not query
  Hyprland for every key.
- Use `initialClass` as the lookup key. The verified value for Chromium on this
  system is `chromium`.
- Keep Bamboo composition, tone placement, Backspace semantics, Vietnamese
  on/off, and cursor-word adoption unchanged unless explicitly requested.
- A missing/invalid config, unknown app, or unavailable strategy must fall back
  to the current strategy so typing is never blocked.
- Do not retain window titles or use app identity as a security boundary.

## Non-goals

- Defining the new Chromium injection algorithm in this step.
- Adding VNI, changing Bamboo, or creating a general plugin framework.
- Distinguishing browser tabs, pages, or individual text fields.

## Current strategy: `virtual-backspace-commit`

This is the exact live-typing behavior currently implemented by
`engine.Telex.ProcessChar` and `app.apply`:

1. A physical letter is mapped to a rune and passed to Bamboo in Vietnamese
   mode.
2. Bamboo returns the full next word. GoKey compares it with the word already
   displayed and finds their longest common prefix.
3. The strategy receives:
   - `deleteRunes`: trailing runes from the old word that must be removed;
   - `insert`: the new suffix that must be inserted.
4. For each rune to remove, GoKey sends a synthetic Backspace press and release
   through `zwp_virtual_keyboard_v1`.
5. If `insert` is non-empty, GoKey sends `CommitString(insert)` followed by
   `Commit(serial)` through `zwp_input_method_v2`.
6. The physical letter press and release are consumed because the transformed
   result has already been injected.

Example for `aw`:

```text
key a: old=""  next="a"  -> delete=0, insert="a"
key w: old="a" next="ă"  -> delete=1, insert="ă"
```

In-word Backspace also produces the same `(deleteRunes, insert)` pair and uses
this strategy. Space, punctuation, and Enter are not part of the strategy: they
end the composing word and are forwarded normally.

Cursor-word adoption is also outside this strategy. It already uses an atomic
`DeleteSurroundingText + CommitString` operation to replace an older committed
word.

## Recommended configuration boundary

Use one runtime JSON mapping rather than a flat special-app list:

```json
{
  "default": "virtual-backspace-commit",
  "apps": {
    "chromium": "virtual-backspace-commit"
  }
}
```

Proposed installed path:

```text
~/.config/gokey/app-strategies.json
```

Chromium initially points to the fully specified current strategy. After the
new behavior is defined and implemented, only its value changes, for example:

```json
"chromium": "surrounding-delete-commit"
```

No placeholder strategy should be registered before its behavior and fallback
rules are accepted.

## Acceptance criteria

- The current strategy has a stable name and exact delete/insert semantics.
- The configuration schema contains `chromium` using its verified
  `initialClass`.
- Strategy selection happens from the cached active app, with no per-key
  Hyprland query.
- Unknown apps and all detection/config errors retain current typing behavior.
- The future Chromium strategy can replace only injection without duplicating
  Bamboo composition or the complete `onKey` state machine.

## Trade-offs

### App-to-strategy mapping (recommended)

Assumes future app differences belong to text injection. It fails first if an
app actually needs different Telex composition rules. It keeps one engine and
allows more than one injection strategy without another config format.

### Flat special-app list

Assumes there will forever be exactly one special behavior. It fails as soon as
two apps need different strategies and leaves the meaning of list membership
implicit in code.

### Full per-app composition engine

Assumes apps need different Vietnamese transformation rules. It fails first on
state duplication and regression risk because each path must reproduce word,
Backspace, toggle, and cursor-edit behavior.

## Better approaches

An app-to-strategy mapping is better than the implied flat list. The current
repo already identifies Chromium's known weakness as a fake-Backspace versus
client-state race, while Bamboo still produces the correct Vietnamese word.
Selecting only the injection strategy addresses that boundary and avoids
duplicating the Telex engine.

## Unresolved question

Should the new Chromium strategy use the already-available surrounding-text
snapshot to perform `DeleteSurroundingText(deleteRunes, 0) + CommitString`
instead of synthetic Backspace, or do you intend a different edit sequence?
