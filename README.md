# screenshot-win

screenshot-win is a lightweight Windows screenshot utility written in Go. It supports ordinary region captures and scrolling captures that automatically match and stitch consecutive frames into one PNG image.

## Features

- Select a region on the Windows desktop with the mouse
- Capture and stitch scrolling content in either vertical direction
- Save screenshots as PNG or copy them to the clipboard
- Pin a captured image above other windows
- Add rectangles, arrows, and text annotations
- Adjust annotation colors and line widths
- Run from the Windows notification area
- Configure the global capture shortcut and scrolling matcher from a native Windows settings dialog
- Capture multi-monitor desktops using physical pixel coordinates
- Collect diagnostic data for rejected scrolling frames

## Requirements

### Runtime

- Windows 10 version 2004 or later is recommended for the best preview behavior

### Build environment

- Go 1.22 or later
- A Linux shell
- `zip` and `sha256sum` are optional and are only needed for creating a release archive

The Windows implementation uses the Win32 API through Go's standard library and does not use CGO. As a result, MinGW and Wine are not required to cross-compile the executable on Linux.

## Build Windows on Linux

From the repository root, run:

```bash
mkdir -p dist
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -buildvcs=false -trimpath \
  -ldflags="-s -w -H=windowsgui" \
  -o dist/screenshot-win.exe ./cmd/screenshot-win
```

The resulting Windows executable is:

```text
dist/screenshot-win.exe
```

The linker options used above have the following effects:

- `-s -w` removes symbol and debug tables to reduce the executable size.
- `-H=windowsgui` builds a GUI application, preventing a console window from opening when screenshot-win is launched from Explorer.
- `-trimpath` removes local build paths from the binary.
- `-buildvcs=false` makes builds work even when Git metadata is unavailable.

To create the same kind of archive and checksum used by the release workflow:

```bash
zip -j -9 dist/screenshot-win-windows-amd64.zip dist/screenshot-win.exe
cd dist
sha256sum screenshot-win-windows-amd64.zip > screenshot-win-windows-amd64.zip.sha256
```

## Test and verify

Run the platform-independent tests and static checks on Linux before building:

```bash
go test ./...
go vet ./...
```

You can verify the cross-compiled file type with:

```bash
file dist/screenshot-win.exe
```

It should be reported as a 64-bit PE executable for Microsoft Windows. The Windows executable cannot be run natively on Linux; copy it to a Windows machine for the final GUI and capture test.

## Usage

Double-click `screenshot-win.exe` to start screenshot-win in the Windows notification area. Press `Alt+Shift+A` from any application to begin a capture. You can also left-click the tray icon, or right-click it to start a capture, open settings, or exit the application.

### Settings

Settings are stored beside the executable in `screenshot-win.toml`. Relative diagnostic paths are resolved from that directory. If the executable directory is not writable, saving reports an error and keeps the unsaved values in the dialog. A missing file uses built-in defaults; an invalid file produces a warning and the application continues with defaults.

Screenshot and clipboard-pin shortcuts accept any combination of `Ctrl`, `Alt`, and `Shift` plus one key (for example `Ctrl+A`, `Alt+A`, `Shift+A`, or `Ctrl+Alt+Shift+A`), or a standalone `F1`–`F11`. F12 is reserved by Windows and is rejected. Bare letters/digits and modifier-only shortcuts are not accepted. The two actions must use different shortcuts; conflicts with other applications are reported when applying settings. Shortcuts are temporarily released while a shortcut field has focus so you can record an existing binding.

**Pin clipboard** can be bound to a shortcut in Settings. Its optional shortcut defaults to disabled (`pin_hotkey = ''`); use **Clear** beside either shortcut in Settings to disable that binding. The tray context menu contains Settings and Exit; left-clicking the tray icon starts a capture. It pins a clipboard image, preferring PNG and then Windows DIB formats. If no supported image can be read, plain Unicode text is rendered as a white image with black text, preserving newlines and wrapping long lines. Text uses Microsoft YaHei UI at 18 pixels with 16-pixel margins and up to 640 pixels of content width. Rich text formatting is discarded. Empty content and oversized or malformed data produce an error without changing the clipboard.

Pins appear near the mouse pointer and use the existing drag, zoom, and close controls. While a capture or clipboard-pin operation is running, additional triggers are ignored. Applying settings takes effect immediately; older settings files keep clipboard pinning disabled until configured.

The generated file has this shape:

```toml
[general]
hotkey = 'Alt+Shift+A'
pin_hotkey = ''
language = 'en'
candidate_mode = 'none'

[long_capture]
mode = 'legacy'
interval_ms = 100
max_scroll_ratio = 0.5
max_mean_difference = 8.0
minimum_confidence = 0.25
stationary_threshold = 0.5

[diagnostics]
enabled = false
directory = 'diagnostics'
limit = 50
```

### Automatic candidate rectangles

In Settings → General, choose a candidate mode:

- `none` (default): manual selection.
- `windows ui interface`: reserved, not implemented; currently behaves like none.
- `minimal Rectangle`: freeze the desktop on entry and detect rectangular regions using pure Go image processing.

Move the mouse to preview the smallest detected rectangle containing it. Candidates must be at least 100×80 physical pixels. While detection runs, or if no candidate contains the pointer, the preview uses the entire current monitor, including the taskbar. Click to confirm, or hold and drag to select manually; manual selections have no minimum size. Esc or right-click cancels. Detection and the final screenshot use the same frozen frame. After selection, use the toolbar to save, copy, annotate, pin, or start a scrolling capture.

During scrolling capture, scroll slowly. The default `legacy` mode captures downward scrolling; select `bidirectional` in Settings to capture in both directions. Use the capture toolbar to save, copy, edit, pin, or cancel. Settings changes apply to the next capture.

### Rectangle detector debugging

The only command-line mode is a one-shot detector diagnostic:

```powershell
.\\screenshot-win.exe --debug.candidate_mode.minimalrectangle 100 150 1200 700 page.png
```

The four numbers specify x, y, width, and height in physical virtual-desktop pixels. Negative coordinates are supported; the region must be inside the virtual desktop. The output path is required and is relative to the current working directory unless absolute.

This captures the specified area once and outlines every detected rectangle of at least 100×80 pixels in blue. It uses the same detector as interactive selection, without pointer filtering or monitor fallback. With no candidates, it saves the unmarked screenshot. The PNG retains the requested dimensions; an existing output file is overwritten. The command prints the rectangle count and output path, then exits. Capture, argument, and file errors produce a nonzero exit code.

Normal launch takes no arguments and starts the tray host. The former coordinate scrolling-capture command and all previous command-line options have been removed. Configure scrolling and diagnostics through Settings or `screenshot-win.toml`.

## Project layout

```text
app/                 Capture workflows and application state
capture/             Windows screen and clipboard integration
cmd/screenshot-win/      Application entry point and tray host
editor/              Screenshot annotation model and rendering
selector/            Region selection, toolbars, previews, and pinned images
third_party/         Vendored toolbar icon assets and licenses
matcher.go           Frame matching logic
stitch.go            Scrolling image builder
bidirectional.go      Bidirectional matcher, page anchors, and image builder
```

## Releases

The repository includes a manually triggered GitHub Actions workflow that tests the project, cross-compiles the Windows amd64 executable on Ubuntu, packages it, generates a SHA-256 checksum, and creates a GitHub release.
