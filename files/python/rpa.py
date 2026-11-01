# Restricted fallback extractor for RPA-3.0 archives.
# This is used only when the primary extractor cannot process an archive.
from __future__ import print_function

import io
import mmap
import os
try:
    import pickle5 as pickle
except ImportError:
    import pickle
import re
import sys
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


class RestrictedUnpickler(pickle.Unpickler):
    def find_class(self, module, name):
        raise pickle.UnpicklingError(
            "global objects are not permitted in an RPA index: %s.%s" % (module, name)
        )

    def find_global(self, module, name):
        raise pickle.UnpicklingError(
            "global objects are not permitted in an RPA index: %s.%s" % (module, name)
        )


def safe_unpickle(data):
    stream = io.BytesIO(data)
    if sys.version_info[0] >= 3:
        return RestrictedUnpickler(stream, encoding="latin1").load()
    return RestrictedUnpickler(stream).load()

MAX_INDEX_BYTES = 512 * 1024 * 1024

def safe_zlib_decompress(data, limit=MAX_INDEX_BYTES):
    """Decompress an RPA index with an upper bound on expanded size."""
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


class RPA(object):
    def __init__(self, filename):
        self.filename = filename
        self.file = open(filename, "rb")
        try:
            self.mm = mmap.mmap(self.file.fileno(), 0, access=mmap.ACCESS_READ)
            self.index_offset, self.key = self.load_index_header()
            self.index = self.load_index()
            self.files = self.build_file_set()
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
                if offset < 0 or length < 0 or offset > self.index_offset or length > self.index_offset - offset:
                    raise ValueError("RPA data block points outside the archive data region")
                if len(block) == 3 and not isinstance(prefix, (text_type, binary_type)):
                    raise ValueError("invalid prefix value in RPA index")
                if len(prefix) > length:
                    raise ValueError("RPA prefix is longer than its data block")
                if isinstance(prefix, text_type) and sys.version_info[0] >= 3:
                    prefix = prefix.encode("latin1")
                checked.append((offset, length, prefix))
            normalized[name] = checked
        return normalized

    def build_file_set(self):
        result = {}
        for name, blocks in self.index.items():
            if not blocks:
                raise ValueError("archive entry has no data blocks")
            offset, length, prefix = blocks[0]
            if prefix:
                # The legacy fallback historically did not support prefixes.
                raise ValueError("fallback extractor cannot process prefixed RPA blocks")
            result[name] = self.mm[offset:offset + length]
        return result

    def close(self):
        try:
            self.mm.close()
        finally:
            self.file.close()


def extract(filename):
    archive = RPA(filename)
    try:
        root = os.getcwd()
        for archive_name, contents in archive.files.items():
            destination = safe_output_path(root, archive_name)
            parent = os.path.dirname(destination)
            if not os.path.isdir(parent):
                os.makedirs(parent)
            destination = safe_output_path(root, archive_name)
            with open(destination, "wb") as output:
                output.write(contents)
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
