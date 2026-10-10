# Copyright (c) 2026 Lemon4ksan All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

"""
EON Container Format - Python Client Library
Powered by libeon native C ABI.
"""

import ctypes
import os
import sys
from collections.abc import Iterator
from dataclasses import dataclass
from datetime import datetime
from typing import final

# Find libeon DLL/SO/dylib
_LIB_NAME = (
    "libeon.dll"
    if sys.platform == "win32"
    else ("libeon.dylib" if sys.platform == "darwin" else "libeon.so")
)


def _load_lib():
    paths = [
        os.environ.get("EON_LIB_PATH", ""),
        os.path.join(os.path.dirname(__file__), "..", "..", "bin", _LIB_NAME),
        os.path.join(os.path.dirname(__file__), _LIB_NAME),
        _LIB_NAME,
    ]
    for p in paths:
        if p and os.path.exists(p):
            try:
                return ctypes.CDLL(p)
            except Exception:
                pass
    try:
        return ctypes.CDLL(_LIB_NAME)
    except Exception as e:
        raise RuntimeError(
            f"Could not load native EON library ({_LIB_NAME}). Set EON_LIB_PATH. Error: {e}"
        )


_lib = _load_lib()


# Setup C types
@final
class _EonFileStat(ctypes.Structure):
    _fields_ = [
        ("name", ctypes.c_char * 512),
        ("size", ctypes.c_uint64),
        ("namespace_id", ctypes.c_uint16),
        ("mod_time_unix", ctypes.c_int64),
        ("chunk_index", ctypes.c_uint32),
        ("is_dir", ctypes.c_uint8),
    ]


_lib.eon_version.restype = ctypes.c_char_p
_lib.eon_version.argtypes = []

_lib.eon_get_last_error.restype = ctypes.c_char_p
_lib.eon_get_last_error.argtypes = []

_lib.eon_open.restype = ctypes.c_uint64
_lib.eon_open.argtypes = [
    ctypes.c_char_p,
    ctypes.c_char_p,
    ctypes.POINTER(ctypes.c_int),
]

_lib.eon_close.restype = None
_lib.eon_close.argtypes = [ctypes.c_uint64]

_lib.eon_file_count.restype = ctypes.c_int64
_lib.eon_file_count.argtypes = [ctypes.c_uint64]

_lib.eon_stat_index.restype = ctypes.c_int
_lib.eon_stat_index.argtypes = [
    ctypes.c_uint64,
    ctypes.c_int64,
    ctypes.POINTER(_EonFileStat),
]

_lib.eon_stat_name.restype = ctypes.c_int
_lib.eon_stat_name.argtypes = [
    ctypes.c_uint64,
    ctypes.c_char_p,
    ctypes.POINTER(_EonFileStat),
]

_lib.eon_read_file.restype = ctypes.c_int64
_lib.eon_read_file.argtypes = [
    ctypes.c_uint64,
    ctypes.c_char_p,
    ctypes.c_void_p,
    ctypes.c_size_t,
]

_lib.eon_read_index.restype = ctypes.c_int64
_lib.eon_read_index.argtypes = [
    ctypes.c_uint64,
    ctypes.c_int64,
    ctypes.c_void_p,
    ctypes.c_size_t,
]


def _safe_fromtimestamp(ts: int) -> datetime:
    if ts <= 0:
        return datetime.fromtimestamp(86400)
    try:
        return datetime.fromtimestamp(ts)
    except Exception:
        return datetime.utcfromtimestamp(ts)


@dataclass
class EonItem:
    name: str
    size: int
    namespace_id: int
    mod_time: datetime
    is_dir: bool


class EonError(Exception):
    pass


class EonArchive:
    """Represents an open EON Container."""

    def __init__(self, path: str, password: str | None = None):
        self.path = path
        out_err = ctypes.c_int(0)
        c_path = path.encode("utf-8")
        c_pass = password.encode("utf-8") if password else None

        self._handle = _lib.eon_open(c_path, c_pass, ctypes.byref(out_err))
        if self._handle == 0:
            err_code = out_err.value
            last_err = _lib.eon_get_last_error()
            err_msg = last_err.decode("utf-8") if last_err else f"Error code {err_code}"
            raise EonError(
                f"Failed to open EON container '{path}': {err_msg} (code {err_code})"
            )

    def close(self):
        if self._handle != 0:
            _lib.eon_close(self._handle)
            self._handle = 0

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def __len__(self) -> int:
        if not self._handle:
            raise EonError("Archive is closed")
        count = _lib.eon_file_count(self._handle)
        if count < 0:
            raise EonError("Failed to get file count")
        return count

    def __iter__(self) -> Iterator[EonItem]:
        for i in range(len(self)):
            yield self.stat_index(i)

    def stat_index(self, index: int) -> EonItem:
        if not self._handle:
            raise EonError("Archive is closed")
        st = _EonFileStat()
        res = _lib.eon_stat_index(self._handle, index, ctypes.byref(st))
        if res != 0:
            raise EonError(f"Extent #{index} not found")
        return EonItem(
            name=st.name.decode("utf-8", errors="replace"),
            size=st.size,
            namespace_id=st.namespace_id,
            mod_time=_safe_fromtimestamp(st.mod_time_unix),
            is_dir=bool(st.is_dir),
        )

    def stat(self, name: str) -> EonItem:
        if not self._handle:
            raise EonError("Archive is closed")
        st = _EonFileStat()
        res = _lib.eon_stat_name(self._handle, name.encode("utf-8"), ctypes.byref(st))
        if res != 0:
            raise KeyError(f"File '{name}' not found in archive")
        return EonItem(
            name=st.name.decode("utf-8", errors="replace"),
            size=st.size,
            namespace_id=st.namespace_id,
            mod_time=_safe_fromtimestamp(st.mod_time_unix),
            is_dir=bool(st.is_dir),
        )

    def read(self, name: str) -> bytes:
        item = self.stat(name)
        if item.size == 0:
            return b""
        buf = ctypes.create_string_buffer(item.size)
        n = _lib.eon_read_file(self._handle, name.encode("utf-8"), buf, item.size)
        if n < 0:
            last_err = _lib.eon_get_last_error()
            err_msg = last_err.decode("utf-8") if last_err else "read error"
            raise EonError(f"Failed to read '{name}': {err_msg}")
        return buf.raw[:n]

    def read_index(self, index: int) -> bytes:
        item = self.stat_index(index)
        if item.size == 0:
            return b""
        buf = ctypes.create_string_buffer(item.size)
        n = _lib.eon_read_index(self._handle, index, buf, item.size)
        if n < 0:
            raise EonError(f"Failed to read extent #{index}")
        return buf.raw[:n]

    def namelist(self) -> list[str]:
        return [item.name for item in self]


def open(path: str, password: str | None = None) -> EonArchive:
    """Open an EON container archive."""
    return EonArchive(path, password=password)


def version() -> str:
    """Return libeon version string."""
    v = _lib.eon_version()
    return v.decode("utf-8") if v else "unknown"
