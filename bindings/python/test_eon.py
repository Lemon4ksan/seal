# Copyright (c) 2026 Lemon4ksan All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import os
import subprocess
import sys
import tempfile
try:
    from typing import override
except ImportError:
    def override(func):
        return func
import unittest

sys.path.insert(0, os.path.dirname(__file__))
try:
    from . import eon
except ImportError:
    import eon


class TestEonPythonBindings(unittest.TestCase):
    @override
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.src_dir = os.path.join(self.tmpdir.name, "src")
        os.makedirs(self.src_dir, exist_ok=True)

        self.f1_path = os.path.join(self.src_dir, "hello.txt")
        self.f1_content = b"Hello from EON python bindings test!\n" * 10
        with open(self.f1_path, "wb") as f:
            f.write(self.f1_content)

        self.f2_path = os.path.join(self.src_dir, "nested", "data.bin")
        os.makedirs(os.path.dirname(self.f2_path), exist_ok=True)
        self.f2_content = bytes(range(256)) * 4
        with open(self.f2_path, "wb") as f:
            f.write(self.f2_content)

        # Locate seal CLI binary
        root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
        seal_candidates = [
            os.path.join(root_dir, "seal.exe"),
            os.path.join(root_dir, "bin", "seal.exe"),
            os.path.join(root_dir, "bin", "aeon.exe"),
        ]
        self.cli_exe = None
        for cand in seal_candidates:
            if os.path.exists(cand):
                self.cli_exe = cand
                break
        if not self.cli_exe:
            raise FileNotFoundError("Could not find seal.exe executable")

        self.archive_path = os.path.join(self.tmpdir.name, "test.eon")
        res = subprocess.run(
            [self.cli_exe, "create", "-o", self.archive_path, self.src_dir],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 0, f"seal create failed: {res.stderr}")

        # Build encrypted archive using CLI
        self.enc_archive_path = os.path.join(self.tmpdir.name, "test_enc.eon")
        self.password = "PythonSecret2026!"
        res = subprocess.run(
            [
                self.cli_exe,
                "create",
                "-p",
                self.password,
                "-o",
                self.enc_archive_path,
                self.src_dir,
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(
            res.returncode, 0, f"aeon create (encrypted) failed: {res.stderr}"
        )

    @override
    def tearDown(self):
        self.tmpdir.cleanup()

    def test_version(self):
        v = eon.version()
        self.assertTrue(len(v) > 0)
        print(f"libeon version: {v}")

    def test_open_and_read(self):
        with eon.open(self.archive_path) as arc:
            files = [item for item in arc if not item.is_dir]
            self.assertEqual(len(files), 2)
            names = arc.namelist()
            print("Extents in archive:", names)
            self.assertTrue(any("hello.txt" in n for n in names))
            self.assertTrue(any("data.bin" in n for n in names))

            # Read by name
            for name in names:
                if "hello.txt" in name:
                    data = arc.read(name)
                    self.assertEqual(data, self.f1_content)
                elif "data.bin" in name:
                    data = arc.read(name)
                    self.assertEqual(data, self.f2_content)

    def test_encrypted_container(self):
        # 1. Opening without password should fail
        with self.assertRaises(eon.EonError):
            _ = eon.open(self.enc_archive_path)

        # 2. Opening with wrong password should fail
        with self.assertRaises(eon.EonError):
            _ = eon.open(self.enc_archive_path, password="WrongPassword!")

        # 3. Opening with correct password should succeed
        with eon.open(self.enc_archive_path, password=self.password) as arc:
            files = [item for item in arc if not item.is_dir]
            self.assertEqual(len(files), 2)
            for name in arc.namelist():
                if "hello.txt" in name:
                    data = arc.read(name)
                    self.assertEqual(data, self.f1_content)


if __name__ == "__main__":
    _ = unittest.main()
