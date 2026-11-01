# Restricted fallback extractor for RPA-3.0 archives.
# This is used only when the primary extractor cannot process an archive.
from __future__ import print_function

import ctypes
import io
import mmap
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
# Allow DEFLATE framing overhead for incompressible indices, but keep the
# compressed tail bounded before it is copied from the memory map.
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
    pending = data
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

    if hasattr(decoder, "eof"):
        if not decoder.eof:
            raise ValueError("truncated compressed RPA index")
    else:
        # Python 2.7 has no decompressobj.eof. The streaming pass has already
        # bounded output; validate completion using the legacy zlib API.
        try:
            verified = zlib.decompress(data)
        except zlib.error:
            raise ValueError("truncated or invalid compressed RPA index")
        if len(verified) != total:
            raise ValueError("incomplete compressed RPA index")

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
        if any(char in part for char in (":", "<", ">", '"', "|", "?", "*")):
            raise ValueError("invalid character in archive path component")
        device_name = part.rstrip(" .").split(".", 1)[0].upper()
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


def replace_file(source, destination):
    """Replace an output file after a complete extraction has succeeded."""
    if hasattr(os, "replace"):
        os.replace(source, destination)
        return
    if os.name == "nt":
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


class RPA(object):
    def __init__(self, filename):
        self.filename = filename
        self.file = open(filename, "rb")
        try:
            self.mm = mmap.mmap(self.file.fileno(), 0, access=mmap.ACCESS_READ)
            self.index_offset, self.key = self.load_index_header()
            self.index = self.load_index()
        except Exception:
            if hasattr(self, "mm"):
                try:
                    self.mm.close()
                except Exception:
                    pass
            self.file.close()
            raise

    def load_index_header(self):
        self.mm.seek(0)
        header = self.mm.readline()
        fields = header.split()
        if len(fields) < 3 or fields[0] != b"RPA-3.0":
            raise ValueError("fallback extractor supports RPA-3.0 only")
        try:
            index_offset = int(fields[1], 16)
            key = 0
            for value in fields[2:]:
                key ^= int(value, 16)
        except ValueError:
            raise ValueError("invalid RPA-3.0 header metadata")
        if index_offset < len(header) or index_offset >= len(self.mm):
            raise ValueError("RPA index offset is outside the archive")
        return index_offset, key

    def load_index(self):
        compressed_size = len(self.mm) - self.index_offset
        if compressed_size > MAX_INDEX_COMPRESSED_BYTES:
            raise ValueError("compressed RPA index exceeds the safety limit")
        try:
            raw = safe_zlib_decompress(self.mm[self.index_offset:])
            index = safe_unpickle(raw)
        except Exception as exc:
            raise ValueError("unable to safely read RPA index: %s" % exc)
        if not isinstance(index, dict):
            raise ValueError("RPA index must be a dictionary")

        normalized = {}
        for name, blocks in index.items():
            to_text(name)
            if not isinstance(blocks, (list, tuple)) or not blocks:
                raise ValueError("invalid block list in RPA index")
            checked = []
            for block in blocks:
                if not isinstance(block, (list, tuple)) or len(block) not in (2, 3):
                    raise ValueError("invalid block record in RPA index")
                if (not isinstance(block[0], integer_types) or isinstance(block[0], bool) or
                        not isinstance(block[1], integer_types) or isinstance(block[1], bool)):
                    raise ValueError("RPA offsets and lengths must be integers")
                offset, length = block[0] ^ self.key, block[1] ^ self.key
                prefix = block[2] if len(block) == 3 else (b"" if sys.version_info[0] >= 3 else "")
                if len(block) == 3 and not isinstance(prefix, (text_type, binary_type)):
                    raise ValueError("invalid prefix value in RPA index")
                if isinstance(prefix, text_type) and sys.version_info[0] >= 3:
                    prefix = prefix.encode("latin1")
                # The indexed length is physical data length. The optional
                # prefix is a separate virtual segment prepended before it.
                if (offset < 0 or length < 0 or offset > self.index_offset or
                        length > self.index_offset - offset):
                    raise ValueError("RPA data block points outside the archive data region")
                checked.append((offset, length, prefix))
            normalized[name] = checked
        return normalized

    def list(self):
        return list(self.index.keys())

    def write_to(self, archive_name, output, chunk_size=COPY_CHUNK_BYTES):
        """Stream every block to output without caching archive payloads in RAM."""
        blocks = self.index.get(archive_name)
        if not blocks:
            raise ValueError("archive entry does not exist or has no data blocks")
        if chunk_size <= 0:
            raise ValueError("chunk_size must be positive")

        for offset, length, prefix in blocks:
            if prefix:
                output.write(prefix)
            remaining = length
            self.mm.seek(offset)
            while remaining:
                data = self.mm.read(min(chunk_size, remaining))
                if not data:
                    raise ValueError("truncated data block in RPA archive")
                output.write(data)
                remaining -= len(data)

    def close(self):
        try:
            self.mm.close()
        finally:
            self.file.close()


def extract(filename):
    archive = RPA(filename)
    try:
        root = os.getcwd()
        for archive_name in archive.list():
            temp_path = None
            try:
                destination = safe_output_path(root, archive_name)
                parent = os.path.dirname(destination)
                if not os.path.isdir(parent):
                    os.makedirs(parent)
                destination = safe_output_path(root, archive_name)

                fd, temp_path = tempfile.mkstemp(prefix=".unren-", suffix=".tmp", dir=parent)
                with os.fdopen(fd, "wb") as output:
                    archive.write_to(archive_name, output)
                    output.flush()
                replace_file(temp_path, destination)
                temp_path = None
            finally:
                if temp_path and os.path.exists(temp_path):
                    try:
                        os.remove(temp_path)
                    except OSError:
                        pass
    finally:
        archive.close()


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: rpa.py ARCHIVE", file=sys.stderr)
        sys.exit(2)
    try:
        extract(sys.argv[1])
    except Exception as exc:
        print("RPA fallback extraction failed: %s" % exc, file=sys.stderr)
        sys.exit(1)
