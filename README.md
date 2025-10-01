<p align="center">
  <img src="gui/windows/assets/icons/icon.png" alt="Giffy icon" width="128" />
</p>

<h1 align="center">Giffy</h1>

<p align="center">
  Frame-by-frame analysis for scientific data in a grid layout
</p>

<p align="center">
  <a href="https://go.dev/"><img alt="Go" src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white"></a>
  <a href="https://fyne.io/"><img alt="Fyne" src="https://img.shields.io/badge/Fyne-2.6.3-18a999"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platform-macOS%20|%20Windows%20|%20Linux-2ea44f">
  <img alt="FFmpeg" src="https://img.shields.io/badge/FFmpeg-bundled%20via%20script-007808?logo=ffmpeg&logoColor=white">
</p>

---
## Features
- Multi‑platform support (MacOS/Windows/Linux)
- Frame-by-frame scrubbing or controlled FPS playback
- Batch resizing for large images
- MP4 and grid frame image export

## Quick start
Prerequisites
- Go 1.24+
- Fyne
``` bash
go install fyne.io/fyne/v2/cmd/fyne@latest
```
- FFmpeg:
  - For development runs, Giffy can bundle platform FFmpeg binaries under gui/windows/assets/ffmpeg via the script below

### FFmpeg Binaries
This repository includes a helper script to fetch prebuilt FFmpeg binaries. Please ensure binaries are placed into the assets/ffmpeg directory structure used by the application.

```bash
./ffmpeg.sh
```

- Place binaries under:
```text
gui/windows/assets/
└─ ffmpeg/
   ├─ darwin-amd64/
   │  └─ ffmpeg
   ├─ darwin-arm64/
   │  └─ ffmpeg
   ├─ linux-amd64/
   │  └─ ffmpeg
   └─ windows-amd64/
      └─ ffmpeg.exe
```

> [!IMPORTANT]
> The application specifically looks in these directories for the binary file. Please make sure they exist and include the correct binary for your platform

### Run (development)
- From the repo root:
``` bash
go run .
```

## Build and package
- MacOS (Apple Silicon):
``` bash
fyne package -os darwin -name "Giffy" -icon gui/windows/assets/icons/icon.png
```

- Windows:
``` bash
fyne package -os windows -name "Giffy" -icon gui/windows/assets/icons/icon.png
```

- Linux:
``` bash
fyne package -os linux -name "Giffy" -icon gui/windows/assets/icons/icon.png
```
