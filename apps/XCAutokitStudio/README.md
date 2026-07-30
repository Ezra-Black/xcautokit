# XCAutokit Studio

Standalone macOS app that **always monitors** every booted iOS Simulator, records manual interactions, and replays them as Blocks → Building Blocks → Sets.

## Concepts

| Layer | Meaning |
|-------|---------|
| **Timeline** | Continuous stream per simulator (one column each) |
| **HID capture** | Mouse/keyboard in Simulator.app → tap / swipe / long-press / type steps with device coordinates |
| **Block** | Shift-click a range → save reusable steps |
| **Building block** | Ordered chain of blocks |
| **Set** | Ordered chain of building blocks (a callable function) |
| **Video** | Per-column `simctl io recordVideo` |

## macOS UI

Native shell: `NavigationSplitView` sidebar, window toolbar, trailing inspector, `Settings…` window, and Capture/Replay menus. Empty states use `ContentUnavailableView`; lists use standard inset style with selection and context menus.

## Run

```bash
# rebuild CLI helpers used by interrupt-aware replay
cd ../.. && go build -o xcautokit .

cd apps/XCAutokitStudio
# Prefer the .app wrapper (native full screen / Zoom). Plain `swift run` is a naked binary.
./scripts/run-app.sh
```

Open **XCAutokit Studio → Settings…** for interrupt policy and launch defaults.

Optional: `XCAUTOKIT_BIN=/path/to/xcautokit`

## Permissions

**Accessibility** is required for HID capture (System Settings → Privacy & Security → Accessibility → enable XCAutokit Studio / `swift run` binary).

## Restore before execute

Every Execute run **restarts to the first-step state** before replaying:

1. Press Home (SpringBoard)
2. If the first step was in an app → **terminate + launch** that bundle (cold start)
3. If the first step was on the home screen → stay on SpringBoard
4. Then run interrupt handling and the recorded steps

Foreground bundle IDs are stamped onto each recorded step while monitoring (`launchctl` / UIKitApplication). Blocks store a `restore` snapshot when saved.

## Interrupt-aware replay

Before each step, Studio runs:

```bash
xcautokit interrupt check --udid <UDID>
xcautokit interrupt dismiss --action decline|accept --udid <UDID>
```

Policy picker in the toolbar:

- Auto-decline permissions (default)
- Auto-accept permissions
- Pause on interrupt

## Storage

- Library: `~/.xcautokit/studio/library.json`
- Videos: `~/Pictures/xcautokit/studio/`
