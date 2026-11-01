> **README 維護備註：** 本文件由 ChatGPT 協助整理，日期：2026-11-01。

# UnRen-Go

Cross-platform Ren'Py game utility, ported from [UnRen 1.0.11d](https://f95zone.to/threads/unren-bat-v1-0-11d-rpa-extractor-rpyc-decompiler-console-developer-menu-enabler.3083/).

This is the personal `gofix_2` branch. It includes additional archive-safety checks, more reliable RPYC decompilation verification, and safer source cleanup and retry handling.

## Features

- **RPA Extraction**: Extract Ren'Py archives in RPA-2.0, RPA-3.0, and RPA-3.2 formats.
- **RPYC Decompilation**: Decompile compiled `.rpyc` scripts into `.rpy` source files using the Python runtime bundled with the game.
- **Python 2 / Python 3 Support**: Includes unrpyc v1.3.2 for Python 2.7 and unrpyc v2.0.2 for Python 3.9 or later.
- **Game Detection**: Detect Ren'Py game directories and identify the bundled Python runtime.
- **Developer Console/Menu**: Enable the developer console (SHIFT+O) and developer menu (SHIFT+D).
- **Quick Save/Load**: Enable F5/F9 quick save and load hotkeys.
- **Skip Unseen Content**: Force-enable skipping of unseen text.
- **Extended Rollback**: Enable rollback using the mouse wheel.
- **Controlled Source Cleanup**: Remove eligible RPA and RPYC source files only after their respective processing steps succeed.
- **Failed Decompilation Recovery**: Remove incomplete output files after failed decompilation attempts so the original RPYC files can be retried.

## Build and download

### Requirements

- Go 1.25.6 or later for building from source.
- Bash for running the cross-platform build script.
- A Ren'Py game containing its bundled Python runtime for archive extraction and RPYC decompilation.

### Build from source

Build for the current host platform:

```bash
go build -o unren-go .
```

Build all supported platforms:

```bash
bash ./build.sh
```

The build script generates binaries for the following targets:

| Operating System | Architecture | Output |
|---|---|---|
| Windows | x64 | `unren-go-windows-x64.exe` |
| Windows | x86 | `unren-go-windows-x86.exe` |
| Windows | ARM64 | `unren-go-windows-arm64.exe` |
| Linux | x64 | `unren-go-linux-x64` |
| Linux | x86 | `unren-go-linux-x86` |
| Linux | ARM64 | `unren-go-linux-arm64` |
| Linux | ARM | `unren-go-linux-arm` |
| macOS | x64 | `unren-go-macos-x64` |
| macOS | ARM64 | `unren-go-macos-arm64` |

Generated binaries are placed in the `dist/` directory.

### Download GitHub Actions artifacts

The personal build workflow runs when code is pushed to the `gofix_2` branch or when manually started.

1. Open the repository's **Actions** tab.
2. Select a successful **Build (Actions Artifact)** run for the `gofix_2` branch.
3. Download the `UnRen-Go-gofix_2-<commit-sha>` artifact.
4. Extract the downloaded ZIP.
5. Select the binary matching your operating system and architecture.

The workflow performs Go tests, Python archive-safety tests, and cross-platform builds before uploading the generated binaries.

It does not create GitHub Releases, tags, or release assets. Actions artifacts are retained for 30 days.

Note: The workflow performs cross-compilation on Ubuntu. Successful compilation does not replace runtime testing on each target operating system.

## Usage

### Interactive mode

Run without arguments to launch the interactive menu:

```bash
./unren-go
```

The program detects the game in the current directory by default.

You can also specify the game directory:

```bash
./unren-go /path/to/game
```

If the game is not detected, the recovery menu lets you browse to the correct game directory.

The interactive menu provides the following operations:

1. Extract RPA archives.
2. Decompile RPYC scripts.
3. Enable the developer console and menu.
4. Enable quick save/load.
5. Enable skipping unseen content.
6. Enable extended rollback.
7. Apply patch options 3–6.
8. Extract, decompile, and apply all patch options.

Source cleanup is disabled in interactive mode for safety.

### Command-line mode

Run with flags to perform actions without opening the interactive menu:

```bash
# Extract RPA archives.
./unren-go -e /path/to/game

# Decompile RPYC scripts.
./unren-go -d /path/to/game

# Extract RPA archives and decompile RPYC scripts.
./unren-go -e -d /path/to/game

# Extract, decompile, and apply all patches.
./unren-go --all /path/to/game

# Enable selected patch options.
./unren-go --console --skip /path/to/game

# Perform all operations and remove eligible source files.
./unren-go --all --clean /path/to/game
```

### Command-line options

| Flag | Short | Description |
|---|---|---|
| `--extract` | `-e` | Extract RPA archives. |
| `--decompile` | `-d` | Decompile RPYC files. |
| `--all` | `-a` | Extract RPA archives, decompile RPYC scripts, and apply all patches. |
| `--clean` | `-c` | Remove eligible source archives/scripts after successful processing. |
| `--console` | | Enable the developer console and menu. |
| `--quicksave` | | Enable quick save/load. |
| `--skip` | | Force-enable skipping unseen content. |
| `--rollback` | | Enable extended rollback. |
| `--help` | `-h` | Show help and usage. |
| `--version` | `-v` | Show version information. |

`--clean` must be used together with `--extract`, `--decompile`, or `--all`. It does not start an extraction or decompilation operation by itself.

Automation mode returns a nonzero exit code when an operation fails, allowing scripts and CI workflows to detect failures.

## Archive extraction safety

The embedded RPA extraction tools include additional validation to reduce the risk of malformed or malicious archives affecting files outside the intended extraction directory.

### Index validation

- Uses a restricted unpickler instead of unrestricted pickle loading.
- Allows only the limited data and byte-encoding operations required by RPA indexes.
- Limits compressed and decompressed archive-index sizes.
- Rejects truncated or invalid zlib streams and trailing data.
- Validates archive entry offsets, lengths, and data-block boundaries.

The maximum decompressed archive index size is 512 MiB. Compressed input is also subject to a separate size limit.

### Output path validation

- Rejects absolute paths and parent-directory traversal.
- Rejects invalid path components, control characters, and reserved Windows device names.
- Prevents extraction paths from escaping the selected output directory through symbolic-link parent directories.
- Refuses to overwrite symbolic links, non-regular files, and files with multiple hard links.
- Writes extracted data to a temporary file before replacing the destination.

These checks are applied by both the primary extractor and the RPA-3.0 fallback extractor.

### Fallback extractor

The primary extractor is the modified `rpatool.py` implementation.

If the primary extraction process fails, UnRen-Go attempts to use the simpler `rpa.py` fallback extractor.

The fallback extractor supports RPA-3.0 archives only. It is not a general replacement for the primary extractor.

## RPYC decompilation

The program uses the Python runtime bundled with the Ren'Py game instead of requiring a separate system-wide Python installation.

The embedded decompiler versions are:

| Runtime | Bundled decompiler | Python requirement |
|---|---|---|
| Python 2 | unrpyc v1.3.2 | Python 2.7 |
| Python 3 | unrpyc v2.0.2 | Python 3.9 or later |

The program searches the game's `lib/` directory for a compatible Python executable and library directory.

The selected runtime is used to execute the corresponding embedded unrpyc implementation.

### Decompilation success verification

A successful process exit code alone is not considered sufficient.

For each RPYC file, the program requires all of the following:

1. The decompiler process exits successfully.
2. The decompiler outputs its explicit one-file success marker.
3. A new corresponding `.rpy` output exists.
4. The output is a regular file.

If any condition is not met, the RPYC file is treated as failed and is not eligible for source cleanup.

Existing `.rpy` files are detected before decompilation and skipped. The special `un.rpyc` file is also skipped.

## Source cleanup behavior

The `--clean` option applies different rules to RPA and RPYC source files.

### RPA cleanup

RPA archives are removed only when all RPA extraction operations complete without errors.

If an archive cannot be extracted, the RPA cleanup step is skipped.

### RPYC cleanup

RPYC cleanup is more selective:

- Only RPYC files successfully decompiled during the current run are eligible for removal.
- The corresponding `.rpy` output must have been newly created and verified as a regular file.
- Existing `.rpy` outputs are skipped and their RPYC sources are preserved.
- Failed or unprocessed RPYC files are preserved.
- The special `un.rpyc` file is never added to the cleanup list.
- If any RPYC decompilation fails, or the directory traversal reports an error, RPYC cleanup is skipped for that run.

If an eligible RPYC source cannot be deleted because of a filesystem error, the operation reports a failure.

### Failed output cleanup and retry

A failed decompilation may leave behind a partially written `.rpy` file. That file could otherwise cause the next attempt to be skipped because the output path already exists.

To avoid this problem:

1. The program checks that the corresponding `.rpy` output path does not exist before starting the decompiler.
2. It runs the decompiler and checks the process result, success marker, and output file.
3. If the attempt fails, it checks the output path again.
4. A regular file or symbolic link that appeared at that path is removed so the original `.rpyc` can be retried.
5. The original `.rpyc` file is retained.

For safety, directories and other special filesystem objects are not automatically removed.

If a failed attempt leaves a directory, named pipe, socket, or another special object at the `.rpy` output path, the program preserves it. Because that path still exists, a subsequent run may skip the corresponding RPYC file. Inspect and move or remove the unusual path manually before retrying.

## Temporary files

The embedded Python scripts are extracted into a private temporary directory created by UnRen-Go, rather than being written directly into the game directory.

The temporary directory is removed when the runner finishes.

The program also validates temporary-directory ownership before writing or cleaning up, preventing it from treating an arbitrary caller-supplied directory as its own temporary workspace.

## Tests and continuous integration

The GitHub Actions workflow is configured to run on pushes to `gofix_2` and through manual dispatch.

The workflow performs the following steps:

1. Checks out the repository.
2. Sets up the Go version specified in `go.mod` (Go 1.25.6).
3. Runs Go tests:

   ```bash
   go test ./...
   ```

4. Runs Python archive-safety tests:

   ```bash
   python3 -m unittest discover -s scripts -p 'test_*.py' -v
   ```

5. Builds all nine supported platform targets using `build.sh`.
6. Uploads the generated binaries as a GitHub Actions artifact.

The archive-safety tests cover restricted pickle loading, bounded decompression, malformed index rejection, archive entry validation, output path traversal, symbolic-link protection, and valid RPA extraction.

## Version

Current application version: `0.0.6`

Use the following command to display the version:

```bash
./unren-go --version
```

## Credits

- **Original UnRen utility:** [Sam](https://f95zone.to/members/sam.7899/) — the original UnRen 1.0.11d Windows batch script from the F95Zone community.
- **RPYC decompiler:** [unrpyc](https://github.com/CensoredUsername/unrpyc) by Yuri K. Schlesner, CensoredUsername, and Jackmcbarn. This branch includes unrpyc v1.3.2 for Python 2 and v2.0.2 for Python 3. The bundled source retains its MIT license notices.
- **RPA extraction:** [rpatool](https://github.com/Shizmob/rpatool) by Shizmob. This project uses a modified, read-only extraction implementation with additional archive-safety checks. The upstream project is licensed under WTFPL v2.
- **Ren'Py archive format:** Credit for the creation of the Ren'Py archive format and its reference implementation goes to Tom Rothamel (renpytom), as acknowledged by the upstream rpatool project.

## License and attribution

This repository does not currently contain a top-level `LICENSE` file. The licenses of bundled third-party components should not be interpreted as a single project-wide license.

- The bundled unrpyc source retains its MIT License and copyright notices.
- The upstream rpatool component is licensed under the WTFPL v2. This branch contains a modified extraction implementation.
- Refer to the respective upstream projects for their full license terms and attribution requirements.

Use these tools only with game files and archives that you are authorized to access, extract, or modify.
