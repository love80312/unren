#!/usr/bin/env python
# Internal read-only RPA extractor used by UnRen-Go.
# It intentionally supports extraction/listing only. Archive index data is
# parsed with a restricted unpickler and all output paths are sandboxed.
from __future__ import print_function

import argparse
import io
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
    """Unpickler that accepts primitive RPA index data but no global objects."""

    def find_class(self, module, name):
        raise pickle.UnpicklingError(
            "global objects are not permitted in an RPA index: %s.%s" % (module, name)
        )

    # Python 2 calls find_global instead of find_class.
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
            if offset < 0 or length < 0 or offset > data_end or length > data_end - offset:
                raise ValueError("RPA data block points outside the archive data region")
            if len(block) == 3 and not isinstance(block[2], (text_type, binary_type)):
                raise ValueError("invalid prefix value in RPA index")
            if len(block) == 3 and len(block[2]) > length:
                raise ValueError("RPA prefix is longer than its data block")


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
        self.handle.seek(self.index_offset)
        compressed = self.handle.read()
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

    def read(self, filename):
        if filename not in self.indexes:
            raise ValueError("file does not exist in archive")
        blocks = self.indexes[filename]
        if not blocks:
            raise ValueError("archive entry has no data blocks")
        offset, length, prefix = blocks[0]
        self.handle.seek(offset)
        data = self.handle.read(length - len(prefix))
        if len(data) != length - len(prefix):
            raise ValueError("truncated data block in RPA archive")
        return prefix + data

    def close(self):
        if self.handle is not None:
            self.handle.close()
            self.handle = None


def extract_archive(archive, output_dir, filenames):
    if not os.path.isdir(output_dir):
        os.makedirs(output_dir)
    errors = 0
    for archive_name in filenames:
        try:
            destination = safe_output_path(output_dir, archive_name)
            parent = os.path.dirname(destination)
            if not os.path.isdir(parent):
                os.makedirs(parent)
            # Re-resolve after directory creation to catch symlinked parents.
            destination = safe_output_path(output_dir, archive_name)
            contents = archive.read(archive_name)
            with open(destination, "wb") as output:
                output.write(contents)
        except Exception as exc:
            print("Could not extract file {0} from archive: {1}".format(archive_name, exc), file=sys.stderr)
            errors += 1
    return errors


def main(argv=None):
    parser = argparse.ArgumentParser(description="Safe read-only Ren'Py RPA extractor")
    parser.add_argument("archive", help="RPA archive to read")
    parser.add_argument("files", nargs="*", help="Optional archive file names")
    parser.add_argument("-x", "--extract", action="store_true", help="Extract files")
    parser.add_argument("-l", "--list", action="store_true", help="List archive files")
    parser.add_argument("-o", "--outfile", help="Output directory")
    parser.add_argument("-V", "--version", action="version", version="UnRen safe rpatool 1.0")
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
