"""Make chmod tolerant inside a venv on a Windows-hosted Docker bind mount."""

import errno
import os
import sys


_original_chmod = os.chmod
_venv_root = os.path.realpath(sys.prefix)


def _path_is_in_venv(path):
    try:
        return os.path.commonpath((_venv_root, os.path.realpath(os.fspath(path)))) == _venv_root
    except (TypeError, ValueError):
        return False


def _venv_chmod(path, mode, *, dir_fd=None, follow_symlinks=True):
    kwargs = {}
    if dir_fd is not None:
        kwargs["dir_fd"] = dir_fd
    if not follow_symlinks:
        kwargs["follow_symlinks"] = False
    try:
        return _original_chmod(path, mode, **kwargs)
    except OSError as error:
        if error.errno != errno.EPERM or not _path_is_in_venv(path):
            raise


os.chmod = _venv_chmod
