# screenshot-win

screenshot-win is a lightweight Windows screenshot utility written in Go. It supports ordinary region captures and scrolling captures that automatically match and stitch consecutive frames into one PNG image.

## Features

- Select a region on the Windows desktop with the mouse
- Capture and stitch scrolling content
- Save screenshots as PNG or copy them to the clipboard
- Pin a captured image above other windows
- Add rectangles, arrows, and text annotations
- Adjust annotation colors and line widths
- Run from the Windows notification area
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

Double-click `screenshot-win.exe` to start screenshot-win in the Windows notification area. Press `Alt+Shift+A` from any application to begin a capture. You can also left-click the tray icon, or right-click it to start a capture or exit the application.

To start one interactive capture without keeping the tray application running:

```powershell
.\screenshot-win.exe --once
```

Drag to select a region. Press `Esc` or right-click to cancel. After selecting a region, use the toolbar to save, copy, annotate, pin, or begin a scrolling capture.

During a scrolling capture, scroll downward slowly. Use the capture toolbar to save, copy, edit, pin, or cancel the result. `Esc` and `Ctrl+C` also stop a coordinate-based capture.

### Capture a fixed region

screenshot-win also accepts a region in virtual-desktop coordinates:

```powershell
.\screenshot-win.exe <x> <y> <width> <height> [result.png]
```

For example:

```powershell
.\screenshot-win.exe 100 150 1200 700 page.png
```

This starts a scrolling capture for the specified region and saves the stitched image when the capture is stopped.

### Options

| Option | Default | Description |
| --- | --- | --- |
| `--tray` | Enabled when no coordinates are supplied | Run in the Windows notification area |
| `--once` | Disabled | Run one interactive capture without the tray host |
| `--interval <duration>` | `100ms` | Set the delay between captured frames |
| `--max-scroll-ratio <value>` | Internal matcher default | Limit the maximum scroll offset as a fraction of frame height |
| `--max-mean-diff <value>` | Internal matcher default | Set the maximum accepted mean pixel difference |
| `--min-confidence <value>` | Internal matcher default | Set the minimum difference between the best and second-best matches |
| `--stationary-threshold <value>` | Internal matcher default | Set the difference threshold used to treat a frame as stationary |
| `--diagnostics <directory>` | Disabled | Write matching events and rejected frames for troubleshooting |
| `--diagnostic-limit <count>` | `50` | Limit the number of rejected frame pairs saved |

Go duration syntax is accepted by `--interval`, for example `50ms`, `250ms`, or `1s`.

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
```

## Releases

The repository includes a manually triggered GitHub Actions workflow that tests the project, cross-compiles the Windows amd64 executable on Ubuntu, packages it, generates a SHA-256 checksum, and creates a GitHub release.
