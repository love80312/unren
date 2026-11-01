<!-- README.md revised with ChatGPT assistance on 2026-11-01. -->

# UnRen-Go

Cross-platform Ren'Py game utility, ported from [UnRen 1.0.11d](https://f95zone.to/threads/unren-bat-v1-0-11d-rpa-extractor-rpyc-decompiler-console-developer-menu-enabler.3083/).

This is the personal `gofix` branch. It provides RPA extraction, RPYC decompilation, and Ren'Py configuration patches.

## Features

- **RPA Extraction**: Extract `.rpa` archives in RPA-2.0, RPA-3.0, and RPA-3.2 formats.
- **RPYC Decompilation**: Decompile compiled `.rpyc` scripts into `.rpy` source files with the Python runtime bundled with the game.
- **Developer Console/Menu**: Enable the developer console (SHIFT+O) and developer menu (SHIFT+D).
- **Quick Save/Load**: Enable F5/F9 quick save and load hotkeys.
- **Skip Unseen Content**: Force-enable skipping of unseen text.
- **Extended Rollback**: Enable rollback using the mouse wheel.

## Build and download

### Build from source

The project targets Go 1.25.6.

Build for the current host platform:

```bash
go build -o unren-go .
```

Build all supported platforms:

```bash
bash ./build.sh
```

The build script generates binaries for:

- Windows: x64, x86, ARM64
- Linux: x64, x86, ARM64, ARM
- macOS: x64, ARM64

### Download GitHub Actions artifacts

The personal build workflow is configured to run when code is pushed to the `gofix` branch or when manually started.

1. Open the repository's **Actions** tab.
2. Select a successful **Build (Actions Artifact)** run for the `gofix` branch.
3. Download the `UnRen-Go-gofix-<commit-sha>` artifact.
4. Extract the ZIP and select the binary for your platform.

The workflow uploads build artifacts only. It does not create GitHub Releases, tags, or release assets. Actions artifacts are retained for 30 days.

## Usage

### Interactive mode

Run without arguments to launch the interactive menu:

```bash
./unren-go
```

If the game is not detected in the current directory, the recovery menu lets you browse to the game directory.

### Command-line mode

Run with flags to perform actions without opening the menu:

```bash
# Extract RPA archives and decompile RPYC scripts.
./unren-go -e -d /path/to/game

# Extract, decompile, and apply all patches.
./unren-go --all /path/to/game

# Enable selected options.
./unren-go --console --skip /path/to/game
```

### Options

| Flag | Short | Description |
|---|---|---|
| `--extract` | `-e` | Extract RPA archives. |
| `--decompile` | `-d` | Decompile RPYC files. |
| `--clean` | `-c` | Delete source archives/scripts after processing. **See the warning below before using this option.** |
| `--all` | `-a` | Extract, decompile, and apply all patches. |
| `--console` | | Enable the developer console and menu. |
| `--quicksave` | | Enable quick save/load. |
| `--skip` | | Force-enable skipping unseen content. |
| `--rollback` | | Enable extended rollback. |
| `--help` | `-h` | Show help and usage. |
| `--version` | `-v` | Show version information. |

### Known limitation: `--clean` in this branch

**Avoid using `--clean` (including `--all --clean`) when the original `.rpyc` files must be preserved.**

In this branch, when the RPYC decompilation pass reports no failures, cleanup removes every `.rpyc` path found in the initial game scan. That list can include files skipped because a matching `.rpy` already existed, as well as the special `un.rpyc` file. These files may therefore be deleted even though they were not decompiled during the current run.

This behavior is documented for future reference and has not been changed in this branch. Back up the game directory and do not use `--clean` until the cleanup behavior is intentionally revised. RPA cleanup is separate: RPA archives are removed only when all RPA extraction operations complete without errors.

## License

MIT License.

## Credits

- [Sam](https://f95zone.to/members/sam.7899/) from the F95Zone Community.
