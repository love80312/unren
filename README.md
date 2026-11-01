# UnRen-Go

Cross-platform Ren'Py game utility, ported from [UnRen 1.0.11d](https://f95zone.to/threads/unren-bat-v1-0-11d-rpa-extractor-rpyc-decompiler-console-developer-menu-enabler.3083/).

## Features

- Extract `.rpa` archives (RPA-2.0, RPA-3.0, and RPA-3.2).
- Decompile `.rpyc` files into `.rpy` source files using the game's bundled Python interpreter.
- Enable the developer console (SHIFT+O) and developer menu (SHIFT+D).
- Enable quick save/load hotkeys (F5/F9).
- Enable skipping unseen content.
- Enable extended rollback.
- Optional source cleanup after successful extraction/decompilation.

## Build and download

### Build from source

```bash
go build -o unren-go .
```

The project targets Go 1.25.6. The `build.sh` script cross-compiles the supported Windows, Linux, and macOS targets.

### Download GitHub Actions artifacts

The personal build workflow runs when code is pushed to the `gofix_2` branch, or when started manually.

1. Open the repository's **Actions** tab.
2. Select the latest successful **Build (Actions Artifact)** run.
3. Download the `UnRen-Go-gofix_2-<commit-sha>` artifact.
4. Extract the ZIP and select the binary for your platform.

The workflow uploads Windows x64/x86/ARM64, Linux x64/x86/ARM64/ARM, and macOS x64/ARM64 binaries. It does not create GitHub Releases, tags, or release assets. Actions artifacts are retained for 30 days.

## Usage

Run without arguments to launch the interactive menu:

```bash
./unren-go
```

If the game files are not found, the recovery menu lets you browse to the game directory.

### Command-line mode

```bash
# Extract RPA and decompile RPYC.
./unren-go -e -d /path/to/game

# Run extraction, decompilation, and all patch options.
./unren-go --all /path/to/game

# Enable selected options.
./unren-go --console --skip /path/to/game

# Remove source archives/compiled scripts only after successful processing.
./unren-go --all --clean /path/to/game
```

| Flag | Short | Description |
|---|---|---|
| `--extract` | `-e` | Extract RPA archives. |
| `--decompile` | `-d` | Decompile RPYC files. |
| `--clean` | `-c` | Remove processed source files after successful processing. |
| `--all` | `-a` | Extract, decompile, and enable all patches. |
| `--console` | | Enable developer console/menu. |
| `--quicksave` | | Enable quick save/load. |
| `--skip` | | Enable skipping unseen content. |
| `--rollback` | | Enable extended rollback. |
| `--help` | `-h` | Show help and usage. |
| `--version` | `-v` | Show version information. |

`--clean` does not delete RPYC files that were skipped, failed, or not processed. It only removes RPYC sources that this run successfully decompiled and verified. RPA archives are removed only when all RPA extraction operations complete without errors.

## License

MIT License.

## Credits

- [Sam](https://f95zone.to/members/sam.7899/) from F95Zone Community.
