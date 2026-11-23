# gokey — Roadmap

Kept short and honest: only items with real evidence in the code or recent
plans go here. No aspirational feature list.

## Done

- **Core Telex composition** via bamboo-core — tones, diacritics, `đ`,
  uppercase, verified by `TestTelexBasic` in `engine/telex_test.go`.
- **Vietnamese on/off toggle** — Ctrl+Shift chord, implemented in
  `updateModifier` in `main.go`.
- **drun/launcher passthrough** — keys are only transformed when
  `a.active` is true (the focused client has granted input-method focus);
  everything else, including full alphabetic input, is forwarded raw so
  application launchers (wofi, fuzzel) and layer-shell overlays keep working
  normally.
- **App-specific injection strategies on Hyprland** — active-window identity
  is cached on input-method activation; Chromium uses guarded atomic
  surrounding-text replacement while unknown apps retain virtual Backspace.
- **Runtime app strategy configuration** —
  `~/.config/gokey/app-strategies.json` maps Hyprland `initialClass` values to
  either supported strategy and is loaded once at startup.

## Candidate future work

Nothing below is scheduled or committed to — these are known gaps or
weaknesses observed while building the features above, listed so they aren't
rediscovered from scratch.

- **Other input methods (e.g. VNI).** gokey is hardcoded to Bamboo's
  `"Telex"` input method (`bamboo.ParseInputMethod(bamboo.InputMethodDefinitions,
  "Telex")` in `engine.New()`). Supporting VNI or VIQR would mean making that
  a runtime choice.
- **General preferences configuration.** The current JSON only selects text
  injection by app. Vietnamese-on-by-default and keybindings remain compiled
  in; exposing those choices would require an intentional schema extension.

## Explicitly out of scope

- X11/XWayland support — XWayland apps don't speak `text-input-v3`, so
  there's nothing for gokey's Wayland protocols to attach to.
- A plugin/extension system — gokey is intentionally a single small binary,
  not an IME framework.
