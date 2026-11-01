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
        # This would run if an unrestricted unpickler were used.
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
        # The current rpatool parser uses metadata fields after the third token
        # as XOR key components for RPA-3.2.
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
            "../outside.txt",
            "folder/../../outside.txt",
            "/tmp/outside.txt",
            "\\\\server\\share\\outside.txt",
            "C:\\outside.txt",
            "C:relative.txt",
            "folder/.. /escape.txt",
            "NUL.txt",
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
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                universal_newlines=True,
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
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    universal_newlines=True,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                with open(os.path.join(output_dir, "script", "data.txt"), "rb") as stream:
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
                [sys.executable, fallback_script, archive_path],
                cwd=output_dir,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                universal_newlines=True,
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
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                universal_newlines=True,
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
                [sys.executable, fallback_script, archive_path],
                cwd=output_dir,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                universal_newlines=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(os.path.exists(escaped_path))


if __name__ == "__main__":
    unittest.main()
