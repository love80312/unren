import importlib.util
import os
import pickle
import subprocess
import sys
import tempfile
import unittest
import zlib

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PYTHON_DIR = os.path.join(ROOT, "files", "python")


def load_module(name, filename):
    path = os.path.join(PYTHON_DIR, filename)
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


rpatool = load_module("unren_rpatool_safety_test", "rpatool.py")
rpatool_py2 = load_module("unren_rpatool_py2_safety_test", "rpatool_py2.py")
rpa_fallback = load_module("unren_rpa_fallback_safety_test", "rpa.py")


class DangerousPayload(object):
    def __reduce__(self):
        return (eval, ("1 + 1",))


def make_rpa30(path, filename, data, malicious_index=None):
    key = 0x12345678
    header_placeholder = b"RPA-3.0 0000000000000000 00000000\n"
    data_offset = len(header_placeholder)
    if malicious_index is None:
        index = {filename: [(data_offset ^ key, len(data) ^ key)]}
    else:
        index = malicious_index
    compressed = zlib.compress(pickle.dumps(index, protocol=2))
    index_offset = data_offset + len(data)
    header = ("RPA-3.0 %016x %08x\n" % (index_offset, key)).encode("ascii")
    if len(header) != len(header_placeholder):
        raise AssertionError("test RPA header length changed unexpectedly")
    with open(path, "wb") as archive:
        archive.write(header)
        archive.write(data)
        archive.write(compressed)


def make_rpa30_multiblock(path, filename, blocks):
    """Build an RPA-3.0 entry from (virtual_prefix, stored_bytes) blocks."""
    key = 0x12345678
    header_placeholder = b"RPA-3.0 0000000000000000 00000000\n"
    data_offset = len(header_placeholder)
    payload = bytearray()
    records = []
    for prefix, data in blocks:
        if not isinstance(prefix, bytes) or not isinstance(data, bytes):
            raise TypeError("test blocks must contain bytes")
        offset = data_offset + len(payload)
        # RPA 3.0's dlen is the physical payload length; prefix is separate.
        length = len(data)
        records.append((offset ^ key, length ^ key, prefix))
        payload.extend(data)
    compressed = zlib.compress(pickle.dumps({filename: records}, protocol=2))
    index_offset = data_offset + len(payload)
    header = ("RPA-3.0 %016x %08x\n" % (index_offset, key)).encode("ascii")
    if len(header) != len(header_placeholder):
        raise AssertionError("test RPA header length changed unexpectedly")
    with open(path, "wb") as archive:
        archive.write(header)
        archive.write(payload)
        archive.write(compressed)


def make_rpa30_two_tuple_blocks(path, filename, chunks):
    """Build a fragmented RPA-3.0 entry using ordinary (offset, dlen) tuples."""
    key = 0x12345678
    header_placeholder = b"RPA-3.0 0000000000000000 00000000\n"
    data_offset = len(header_placeholder)
    payload = bytearray()
    records = []
    for data in chunks:
        offset = data_offset + len(payload)
        records.append((offset ^ key, len(data) ^ key))
        payload.extend(data)
    compressed = zlib.compress(pickle.dumps({filename: records}, protocol=2))
    index_offset = data_offset + len(payload)
    header = ("RPA-3.0 %016x %08x\n" % (index_offset, key)).encode("ascii")
    with open(path, "wb") as archive:
        archive.write(header)
        archive.write(payload)
        archive.write(compressed)


def make_rpa_archive(path, version, filename, data):
    key = 0x13572468
    if version == 2:
        placeholder = b"RPA-2.0 0000000000000000\n"
    elif version == 3:
        placeholder = b"RPA-3.0 0000000000000000 00000000\n"
    elif version == 3.2:
        placeholder = b"RPA-3.2 0000000000000000 00000000 00000000\n"
    else:
        raise ValueError("unsupported test version")
    data_offset = len(placeholder)
    if version == 2:
        entry = (data_offset, len(data))
    else:
        entry = (data_offset ^ key, len(data) ^ key)
    compressed = zlib.compress(pickle.dumps({filename: [entry]}, protocol=2))
    index_offset = data_offset + len(data)
    if version == 2:
        header = ("RPA-2.0 %016x\n" % index_offset).encode("ascii")
    elif version == 3:
        header = ("RPA-3.0 %016x %08x\n" % (index_offset, key)).encode("ascii")
    else:
        header = ("RPA-3.2 %016x 00000000 %08x\n" % (index_offset, key)).encode("ascii")
    if len(header) != len(placeholder):
        raise AssertionError("test RPA header length changed unexpectedly")
    with open(path, "wb") as archive:
        archive.write(header)
        archive.write(data)
        archive.write(compressed)


class PickleSafetyTests(unittest.TestCase):
    def test_primitive_rpa_index_is_allowed(self):
        payload = pickle.dumps({"script.rpy": [(10, 20)]}, protocol=2)
        for module in (rpatool, rpatool_py2, rpa_fallback):
            result = module.safe_unpickle(payload)
            self.assertEqual(result["script.rpy"][0], (10, 20))

    def test_global_object_payload_is_rejected(self):
        payload = pickle.dumps(DangerousPayload(), protocol=2)
        for module in (rpatool, rpatool_py2, rpa_fallback):
            with self.assertRaises(Exception):
                module.safe_unpickle(payload)

    def test_compressed_index_expansion_is_bounded(self):
        compressed = zlib.compress(b"A" * 1024)
        for module in (rpatool, rpatool_py2, rpa_fallback):
            with self.assertRaises(ValueError):
                module.safe_zlib_decompress(compressed, limit=128)


class OutputPathSafetyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = self.temp.name

    def tearDown(self):
        self.temp.cleanup()

    def test_normal_relative_path_is_accepted(self):
        for module in (rpatool, rpatool_py2, rpa_fallback):
            result = module.safe_output_path(self.root, "game/script.rpy")
            self.assertTrue(result.startswith(os.path.realpath(self.root) + os.sep))

    def test_parent_traversal_and_absolute_paths_are_rejected(self):
        bad_paths = (
            "../outside.txt", "folder/../../outside.txt", "/tmp/outside.txt",
            "\\\\server\\share\\outside.txt", "C:\\outside.txt", "C:relative.txt",
            "folder/.. /escape.txt", "NUL.txt",
        )
        for module in (rpatool, rpatool_py2, rpa_fallback):
            for name in bad_paths:
                with self.subTest(module=module.__name__, name=name):
                    with self.assertRaises(ValueError):
                        module.safe_output_path(self.root, name)

    def test_symlink_parent_cannot_escape_root(self):
        outside = tempfile.mkdtemp()
        try:
            link = os.path.join(self.root, "linked")
            os.symlink(outside, link)
            for module in (rpatool, rpatool_py2, rpa_fallback):
                with self.assertRaises(ValueError):
                    module.safe_output_path(self.root, "linked/escape.txt")
        finally:
            import shutil
            shutil.rmtree(outside, ignore_errors=True)

    def test_final_symlink_is_rejected(self):
        outside_file = os.path.join(self.root, "outside.txt")
        with open(outside_file, "w") as stream:
            stream.write("keep")
        link = os.path.join(self.root, "link.txt")
        os.symlink(outside_file, link)
        for module in (rpatool, rpatool_py2, rpa_fallback):
            with self.assertRaises(ValueError):
                module.safe_output_path(self.root, "link.txt")


class ArchiveExtractionTests(unittest.TestCase):
    def test_valid_rpa30_extracts(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "game.rpa")
            output_dir = os.path.join(temp, "out")
            content = b"hello from archive"
            make_rpa30(archive_path, "scripts/hello.txt", content)
            script = os.path.join(PYTHON_DIR, "rpatool.py")
            result = subprocess.run(
                [sys.executable, "-O", script, "-x", archive_path, "-o", output_dir],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            with open(os.path.join(output_dir, "scripts", "hello.txt"), "rb") as stream:
                self.assertEqual(stream.read(), content)

    def test_rpa20_and_rpa32_extract(self):
        script = os.path.join(PYTHON_DIR, "rpatool.py")
        for version in (2, 3.2):
            with self.subTest(version=version), tempfile.TemporaryDirectory() as temp:
                archive_path = os.path.join(temp, "game.rpa")
                output_dir = os.path.join(temp, "out")
                expected = ("archive version %s" % version).encode("ascii")
                make_rpa_archive(archive_path, version, "script/data.txt", expected)
                result = subprocess.run(
                    [sys.executable, "-O", script, "-x", archive_path, "-o", output_dir],
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                with open(os.path.join(output_dir, "script", "data.txt"), "rb") as stream:
                    self.assertEqual(stream.read(), expected)

    def test_primary_extractor_joins_every_rpa30_block_and_prefix(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "multi.rpa")
            output_dir = os.path.join(temp, "out")
            name = "scripts/multi.bin"
            blocks = [(b"", b"first-"), (b"<middle>", b"second-"), (b"<tail>", b"last")]
            expected = b"".join(prefix + data for prefix, data in blocks)
            make_rpa30_multiblock(archive_path, name, blocks)

            archive = rpatool.RenPyArchive(archive_path)
            try:
                self.assertEqual(archive.read(name), expected)
            finally:
                archive.close()

            script = os.path.join(PYTHON_DIR, "rpatool.py")
            result = subprocess.run(
                [sys.executable, "-O", script, "-x", archive_path, "-o", output_dir],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            with open(os.path.join(output_dir, "scripts", "multi.bin"), "rb") as stream:
                self.assertEqual(stream.read(), expected)

    def test_primary_extractor_joins_multiple_two_tuple_blocks(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "two-tuples.rpa")
            output_dir = os.path.join(temp, "out")
            name = "scripts/two-tuples.bin"
            chunks = [b"part-one", b"-part-two", b"-part-three"]
            expected = b"".join(chunks)
            make_rpa30_two_tuple_blocks(archive_path, name, chunks)

            script = os.path.join(PYTHON_DIR, "rpatool.py")
            result = subprocess.run(
                [sys.executable, "-O", script, "-x", archive_path, "-o", output_dir],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            with open(os.path.join(output_dir, "scripts", "two-tuples.bin"), "rb") as stream:
                self.assertEqual(stream.read(), expected)

    def test_fallback_extractor_joins_every_rpa30_block_and_prefix(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "multi.rpa")
            output_dir = os.path.join(temp, "out")
            os.makedirs(output_dir)
            name = "scripts/multi.bin"
            blocks = [(b"", b"first-"), (b"<middle>", b"second-"), (b"<tail>", b"last")]
            expected = b"".join(prefix + data for prefix, data in blocks)
            make_rpa30_multiblock(archive_path, name, blocks)

            fallback_script = os.path.join(PYTHON_DIR, "rpa.py")
            result = subprocess.run(
                [sys.executable, fallback_script, archive_path], cwd=output_dir,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            with open(os.path.join(output_dir, "scripts", "multi.bin"), "rb") as stream:
                self.assertEqual(stream.read(), expected)

    def test_rpa_prefix_at_archive_end_is_validated_by_stored_length(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "prefix-at-end.rpa")
            output_dir = os.path.join(temp, "out")
            name = "scripts/end.bin"
            blocks = [(b"start:", b"body"), (b"a-long-virtual-prefix:", b"end")]
            expected = b"".join(prefix + data for prefix, data in blocks)
            make_rpa30_multiblock(archive_path, name, blocks)

            script = os.path.join(PYTHON_DIR, "rpatool.py")
            result = subprocess.run(
                [sys.executable, "-O", script, "-x", archive_path, "-o", output_dir],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            with open(os.path.join(output_dir, "scripts", "end.bin"), "rb") as stream:
                self.assertEqual(stream.read(), expected)

    def test_fallback_extracts_valid_rpa30(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "game.rpa")
            output_dir = os.path.join(temp, "out")
            os.makedirs(output_dir)
            expected = b"fallback success"
            make_rpa_archive(archive_path, 3, "folder/fallback.txt", expected)
            fallback_script = os.path.join(PYTHON_DIR, "rpa.py")
            result = subprocess.run(
                [sys.executable, fallback_script, archive_path], cwd=output_dir,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            with open(os.path.join(output_dir, "folder", "fallback.txt"), "rb") as stream:
                self.assertEqual(stream.read(), expected)

    def test_traversal_entry_is_rejected_without_writing_outside(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "malicious.rpa")
            output_dir = os.path.join(temp, "out")
            escaped_path = os.path.join(temp, "escaped.txt")
            key = 0x12345678
            header_placeholder = b"RPA-3.0 0000000000000000 00000000\n"
            data = b"malicious"
            offset = len(header_placeholder)
            index = {"../escaped.txt": [(offset ^ key, len(data) ^ key)]}
            compressed = zlib.compress(pickle.dumps(index, protocol=2))
            index_offset = offset + len(data)
            header = ("RPA-3.0 %016x %08x\n" % (index_offset, key)).encode("ascii")
            with open(archive_path, "wb") as archive:
                archive.write(header)
                archive.write(data)
                archive.write(compressed)

            script = os.path.join(PYTHON_DIR, "rpatool.py")
            result = subprocess.run(
                [sys.executable, "-O", script, "-x", archive_path, "-o", output_dir],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(os.path.exists(escaped_path))

    def test_fallback_rejects_traversal_entry(self):
        with tempfile.TemporaryDirectory() as temp:
            archive_path = os.path.join(temp, "malicious.rpa")
            output_dir = os.path.join(temp, "out")
            os.makedirs(output_dir)
            escaped_path = os.path.join(temp, "escaped.txt")
            key = 0x12345678
            placeholder = b"RPA-3.0 0000000000000000 00000000\n"
            data = b"blocked"
            offset = len(placeholder)
            index = {"../escaped.txt": [(offset ^ key, len(data) ^ key)]}
            compressed = zlib.compress(pickle.dumps(index, protocol=2))
            index_offset = offset + len(data)
            header = ("RPA-3.0 %016x %08x\n" % (index_offset, key)).encode("ascii")
            with open(archive_path, "wb") as archive:
                archive.write(header)
                archive.write(data)
                archive.write(compressed)

            fallback_script = os.path.join(PYTHON_DIR, "rpa.py")
            result = subprocess.run(
                [sys.executable, fallback_script, archive_path], cwd=output_dir,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(os.path.exists(escaped_path))


if __name__ == "__main__":
    unittest.main()
