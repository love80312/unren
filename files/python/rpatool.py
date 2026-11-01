#!/usr/bin/env python
# Internal read-only RPA extractor used by UnRen-Go.
# It intentionally supports extraction/listing only. Archive index data is
# parsed with a restricted unpickler and all output paths are sandboxed.
from __future__ import print_function

import argparse
import ctypes
import io
import os
try:
    import pickle5 as pickle
except ImportError:
    import pickle
import re
import sys
import tempfile
import zlib

try:
    text_type = unicode
    integer_types = (int, long)
except NameError:
    text_type = str
    integer_types = (int,)

try:
    binary_type = bytes
except NameError:
    binary_type = str

MAX_INDEX_BYTES = 512 * 1024 * 1024
# A DEFLATE stream can be slightly larger than its input when data is
# incompressible. Allow a small framing margin while still bounding memory.
MAX_INDEX_COMPRESSED_BYTES = MAX_INDEX_BYTES + (MAX_INDEX_BYTES // 100) + 65536
COPY_CHUNK_BYTES = 8 * 1024 * 1024


def _safe_empty_bytes(*args):
    # Protocol-2 pickle represents an empty bytes value as bytes().
    if args:
        raise pickle.UnpicklingError("invalid bytes constructor in RPA index")
    return b""


def _safe_codec_encode(value, encoding="latin1", errors="strict"):
    # Protocol-2 pickle represents non-empty bytes via _codecs.encode.
    # Restrict the codec to latin1: this is the representation used for
    # Python 2 byte strings and cannot import or invoke arbitrary codecs.
    if not isinstance(value, text_type) or str(encoding).lower() not in ("latin1", "latin-1"):
        raise pickle.UnpicklingError("unsupported byte-string encoding in RPA index")
    if errors != "strict":
        raise pickle.UnpicklingError("unsupported byte-string error mode in RPA index")
    return value.encode("latin1", "strict")


def _restricted_global(module, name):
    if module in ("__builtin__", "builtins") and name == "bytes":
        return _safe_empty_bytes
    if module == "_codecs" and name == "encode":
        return _safe_codec_encode
    raise pickle.UnpicklingError(
        "global objects are not permitted in an RPA index: %s.%s" % (module, name)
    )


class RestrictedUnpickler(pickle.Unpickler):
    """Unpickler for primitive RPA index data and two constrained byte encoders."""

    def find_class(self, module, name):
        return _restricted_global(module, name)

    # Python 2 calls find_global instead of find_class.
    def find_global(self, module, name):
        return _restricted_global(module, name)


def safe_unpickle(data):
    stream = io.BytesIO(data)
    if sys.version_info[0] >= 3:
        return RestrictedUnpickler(stream, encoding="latin1").load()
    return RestrictedUnpickler(stream).load()


def safe_zlib_decompress(data, limit=MAX_INDEX_BYTES):
    """Decompress one complete RPA index stream within bounded input/output sizes."""
    if len(data) > MAX_INDEX_COMPRESSED_BYTES:
        raise ValueError("compressed RPA index exceeds the safety limit")

    decoder = zlib.decompressobj()
    chunks = []
    # Python 2.7 has no decompressobj.eof attribute. Append a small sentinel
    # so unused_data proves that the single zlib stream reached its end. This
    # avoids calling zlib.decompress(data), which has no output-size limit.
    legacy_end_marker = b"\x00UNREN_ZLIB_END\x00" if not hasattr(decoder, "eof") else b""
    pending = data + legacy_end_marker
    total = 0
    while pending:
        piece = decoder.decompress(pending, limit - total + 1)
        total += len(piece)
        if total > limit:
            raise ValueError("decompressed RPA index exceeds the safety limit")
        chunks.append(piece)
        next_pending = decoder.unconsumed_tail
        if next_pending == pending and not piece:
            raise ValueError("invalid or stalled compressed RPA index")
        pending = next_pending

    tail = decoder.flush()
    total += len(tail)
    if total > limit:
        raise ValueError("decompressed RPA index exceeds the safety limit")
    chunks.append(tail)

    # An RPA index is exactly one zlib stream. Reject trailing bytes and
    # concatenated streams instead of silently accepting an ambiguous index.
    unused_data = getattr(decoder, "unused_data", b"")
    if legacy_end_marker:
        # On Python 2.7 the marker must be the only unused input. A truncated
        # stream consumes it (or raises), while trailing bytes remain before it.
        if unused_data != legacy_end_marker:
            if unused_data and unused_data.endswith(legacy_end_marker):
                raise ValueError("trailing data after compressed RPA index")
            raise ValueError("truncated or invalid compressed RPA index")
    else:
        if unused_data:
            raise ValueError("trailing data after compressed RPA index")
        if not decoder.eof:
            raise ValueError("truncated compressed RPA index")

    return b"".join(chunks)


def to_text(value):
    if isinstance(value, text_type):
        return value
    if isinstance(value, binary_type):
        if sys.version_info[0] >= 3:
            try:
                return value.decode("utf-8")
            except UnicodeDecodeError:
                return value.decode("latin1")
        return value.decode("utf-8")
    raise ValueError("archive path is not a string")


def safe_output_path(root, archive_name):
    """Resolve an archive path and reject traversal, drives and symlinks."""
    name = to_text(archive_name).replace("\\", "/")
    if not name or "\x00" in name:
        raise ValueError("empty or invalid archive path")
    if name.startswith("/") or name.startswith("//") or re.match(r"^[A-Za-z]:", name):
        raise ValueError("absolute paths are not allowed")

    parts = []
    for part in name.split("/"):
        if part in ("", "."):
            continue
        if part == ".." or part.rstrip(" .") in ("", ".", ".."):
            raise ValueError("parent-directory traversal is not allowed")
        # Windows strips trailing spaces/dots and rejects control characters;
        # reject them on every host to avoid cross-platform path aliases.
        if part != part.rstrip(" .") or any(ord(char) < 32 for char in part):
            raise ValueError("invalid trailing space, dot, or control character in archive path")
        if any(char in part for char in (":", "<", ">", '"', "|", "?", "*")):
            raise ValueError("invalid character in archive path component")
        device_name = part.rstrip(" .").split(".", 1)[0].rstrip(" .").upper()
        if device_name in ("CON", "PRN", "AUX", "NUL") or re.match(r"^(COM|LPT)[1-9]$", device_name):
            raise ValueError("reserved device name in archive path")
        parts.append(part)
    if not parts:
        raise ValueError("archive path does not name a file")

    root_real = os.path.realpath(root)
    candidate = os.path.join(root_real, *parts)
    parent_real = os.path.realpath(os.path.dirname(candidate))
    relative_parent = os.path.relpath(parent_real, root_real)
    if (relative_parent == os.pardir or
            relative_parent.startswith(os.pardir + os.sep) or
            os.path.isabs(relative_parent)):
        raise ValueError("resolved output path escapes the extraction directory")

    if os.path.islink(candidate):
        raise ValueError("refusing to overwrite a symbolic link")
    if os.path.exists(candidate):
        info = os.stat(candidate)
        if not os.path.isfile(candidate):
            raise ValueError("output path is not a regular file")
        if getattr(info, "st_nlink", 1) > 1:
            raise ValueError("refusing to overwrite a file with multiple hard links")
    return candidate


def validate_index(indexes, archive_size, data_end):
    if not isinstance(indexes, dict):
        raise ValueError("RPA index must be a dictionary")

    for filename, blocks in indexes.items():
        to_text(filename)
        if not isinstance(blocks, (list, tuple)) or not blocks:
            raise ValueError("invalid block list for archive entry")
        for block in blocks:
            if not isinstance(block, (list, tuple)) or len(block) not in (2, 3):
                raise ValueError("invalid block record in RPA index")
            offset, length = block[0], block[1]
            if (not isinstance(offset, integer_types) or isinstance(offset, bool) or
                    not isinstance(length, integer_types) or isinstance(length, bool)):
                raise ValueError("RPA offsets and lengths must be integers")
            prefix = block[2] if len(block) == 3 else b""
            if len(block) == 3 and not isinstance(prefix, (text_type, binary_type)):
                raise ValueError("invalid prefix value in RPA index")
            # The indexed length is the number of physical data bytes. The
            # optional third field is a separate virtual prefix prepended
            # before those bytes, as in Ren'Py's loader.read_index().
            if (offset < 0 or length < 0 or offset > data_end or
                    length > data_end - offset):
                raise ValueError("RPA data block points outside the archive data region")


def replace_file(source, destination):
    """Replace an output file after a complete extraction has succeeded."""
    if hasattr(os, "replace"):
        os.replace(source, destination)
        return
    if os.name == "nt":
        # Python 2.7 has no os.replace; MoveFileExW supports replacement.
        def as_unicode(path):
            if isinstance(path, text_type):
                return path
            return path.decode(sys.getfilesystemencoding() or "mbcs")
        move_file = ctypes.windll.kernel32.MoveFileExW
        move_file.argtypes = (ctypes.c_wchar_p, ctypes.c_wchar_p, ctypes.c_uint)
        move_file.restype = ctypes.c_int
        flags = 0x00000001 | 0x00000008  # REPLACE_EXISTING | WRITE_THROUGH
        if not move_file(as_unicode(source), as_unicode(destination), flags):
            raise ctypes.WinError()
        return
    os.rename(source, destination)


class RenPyArchive(object):
    def __init__(self, filename):
        self.filename = filename
        self.handle = open(filename, "rb")
        try:
            self.size = os.fstat(self.handle.fileno()).st_size
            self.version, self.index_offset, self.key = self.read_header()
            self.indexes = self.read_index()
        except Exception:
            self.handle.close()
            raise

    def read_header(self):
        self.handle.seek(0)
        header = self.handle.readline()
        fields = header.split()
        if len(fields) < 2:
            raise ValueError("invalid RPA header")
        magic = fields[0]
        if not isinstance(magic, text_type):
            magic = magic.decode("ascii")

        if magic == "RPA-2.0":
            version = 2
            key = 0
        elif magic == "RPA-3.0":
            version = 3
            key = 0
        elif magic == "RPA-3.2":
            version = 3.2
            key = 0
        else:
            raise ValueError("unsupported Ren'Py archive version: %s" % magic)

        try:
            index_offset = int(fields[1], 16)
            key_fields = fields[2:] if version == 3 else (fields[3:] if version == 3.2 else [])
            for value in key_fields:
                key ^= int(value, 16)
        except (ValueError, IndexError):
            raise ValueError("invalid RPA header metadata")

        if index_offset < len(header) or index_offset >= self.size:
            raise ValueError("RPA index offset is outside the archive")
        return version, index_offset, key

    def read_index(self):
        compressed_size = self.size - self.index_offset
        if compressed_size > MAX_INDEX_COMPRESSED_BYTES:
            raise ValueError("compressed RPA index exceeds the safety limit")
        self.handle.seek(self.index_offset)
        compressed = self.handle.read(MAX_INDEX_COMPRESSED_BYTES + 1)
        if len(compressed) != compressed_size:
            raise ValueError("unable to read the complete RPA index")
        try:
            raw_index = safe_zlib_decompress(compressed)
            indexes = safe_unpickle(raw_index)
        except Exception as exc:
            raise ValueError("unable to safely read RPA index: %s" % exc)

        if not isinstance(indexes, dict):
            raise ValueError("RPA index must be a dictionary")

        normalized = {}
        for filename, blocks in indexes.items():
            to_text(filename)
            if not isinstance(blocks, (list, tuple)) or not blocks:
                raise ValueError("invalid block list for archive entry")
            converted = []
            for block in blocks:
                if not isinstance(block, (list, tuple)) or len(block) not in (2, 3):
                    raise ValueError("invalid block record in RPA index")
                offset = block[0]
                length = block[1]
                if (not isinstance(offset, integer_types) or isinstance(offset, bool) or
                        not isinstance(length, integer_types) or isinstance(length, bool)):
                    raise ValueError("RPA offsets and lengths must be integers")
                prefix = block[2] if len(block) == 3 else (b"" if sys.version_info[0] >= 3 else "")
                if self.version in (3, 3.2):
                    offset ^= self.key
                    length ^= self.key
                if isinstance(prefix, text_type) and sys.version_info[0] >= 3:
                    prefix = prefix.encode("latin1")
                converted.append((offset, length, prefix))
            normalized[filename] = converted

        validate_index(normalized, self.size, self.index_offset)
        return normalized

    def list(self):
        return list(self.indexes.keys())

    def write_to(self, filename, output, chunk_size=COPY_CHUNK_BYTES):
        """Write every data block for an entry in index order, with bounded RAM."""
        if filename not in self.indexes:
            raise ValueError("file does not exist in archive")
        if chunk_size <= 0:
            raise ValueError("chunk_size must be positive")

        blocks = self.indexes[filename]
        if not blocks:
            raise ValueError("archive entry has no data blocks")

        for offset, length, prefix in blocks:
            if prefix:
                output.write(prefix)
            remaining = length
            self.handle.seek(offset)
            while remaining:
                data = self.handle.read(min(chunk_size, remaining))
                if not data:
                    raise ValueError("truncated data block in RPA archive")
                output.write(data)
                remaining -= len(data)

    def read(self, filename):
        """Return a complete entry; all blocks are joined in archive index order."""
        blocks = self.indexes.get(filename)
        if not blocks:
            raise ValueError("file does not exist in archive or has no data blocks")

        # Preserve the fast, low-overhead path for the common one-block case.
        if len(blocks) == 1:
            offset, length, prefix = blocks[0]
            self.handle.seek(offset)
            data = self.handle.read(length)
            if len(data) != length:
                raise ValueError("truncated data block in RPA archive")
            return prefix + data

        result = io.BytesIO()
        self.write_to(filename, result)
        return result.getvalue()

    def close(self):
        if self.handle is not None:
            self.handle.close()
            self.handle = None


def extract_archive(archive, output_dir, filenames):
    if not os.path.isdir(output_dir):
        os.makedirs(output_dir)
    errors = 0
    for archive_name in filenames:
        temp_path = None
        try:
            destination = safe_output_path(output_dir, archive_name)
            parent = os.path.dirname(destination)
            if not os.path.isdir(parent):
                os.makedirs(parent)
            # Re-resolve after directory creation to catch symlinked parents.
            destination = safe_output_path(output_dir, archive_name)

            fd, temp_path = tempfile.mkstemp(prefix=".unren-", suffix=".tmp", dir=parent)
            with os.fdopen(fd, "wb") as output:
                archive.write_to(archive_name, output)
                output.flush()
            # Never leave a partially extracted destination if a later block fails.
            replace_file(temp_path, destination)
            temp_path = None
        except Exception as exc:
            print("Could not extract file {0} from archive: {1}".format(archive_name, exc), file=sys.stderr)
            errors += 1
        finally:
            if temp_path and os.path.exists(temp_path):
                try:
                    os.remove(temp_path)
                except OSError:
                    pass
    return errors


def main(argv=None):
    parser = argparse.ArgumentParser(description="Safe read-only Ren'Py RPA extractor")
    parser.add_argument("archive", help="RPA archive to read")
    parser.add_argument("files", nargs="*", help="Optional archive file names")
    parser.add_argument("-x", "--extract", action="store_true", help="Extract files")
    parser.add_argument("-l", "--list", action="store_true", help="List archive files")
    parser.add_argument("-o", "--outfile", help="Output directory")
    parser.add_argument("-V", "--version", action="version", version="UnRen safe rpatool 1.1")
    args = parser.parse_args(argv)

    if not args.extract and not args.list:
        print("No operation given :(\nUse --help for usage details.", file=sys.stderr)
        return 2

    archive = None
    try:
        archive = RenPyArchive(args.archive)
        if args.list:
            for name in sorted(archive.list()):
                print(to_text(name))
            return 0
        output_dir = args.outfile or "."
        filenames = args.files or archive.list()
        errors = extract_archive(archive, output_dir, filenames)
        return 1 if errors else 0
    except Exception as exc:
        print("RPA extraction failed: %s" % exc, file=sys.stderr)
        return 1
    finally:
        if archive is not None:
            archive.close()


if __name__ == "__main__":
    sys.exit(main())
