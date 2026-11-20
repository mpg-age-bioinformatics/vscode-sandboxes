#!/usr/bin/env python3
"""Keep R's x86-64 dependency mappings compatible with 16 KiB build hosts.

QEMU cannot change its host kernel page size. Older distro shared libraries can
have 4 KiB LOAD layouts that fail to map on Apple silicon's 16 KiB sandbox kernel.
patchelf adds a 16 KiB-aligned metadata segment; machine code and the library ABI
stay unchanged. Only affected libraries in R's dependency graph are rewritten.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tempfile

PAGE_SIZE = 16384


def inspect_elf(filename):
    data = Path(filename).read_bytes()
    if data[:6] != b'\x7fELF\x02\x01':
        return None
    kind, machine = struct.unpack_from('<HH', data, 16)
    if machine != 62 or kind not in (2, 3):  # x86-64 executables/shared objects
        return None
    offset = struct.unpack_from('<Q', data, 32)[0]
    size, count = struct.unpack_from('<HH', data, 54)
    if size != 56 or offset + size * count > len(data):
        raise ValueError(f'Invalid ELF program headers: {filename}')
    headers = [struct.unpack_from('<IIQQQQQQ', data, offset + i * size) for i in range(count)]
    loads = [p for p in headers if p[0] == 1]
    entries = {}
    for p in headers:
        if p[0] == 2:  # PT_DYNAMIC
            for position in range(p[2], p[2] + p[5], 16):
                tag, value = struct.unpack_from('<qQ', data, position)
                if tag == 0:
                    break
                entries.setdefault(tag, []).append(value)
    strings = b''
    if 5 in entries:
        address = entries[5][0]
        owner = next(p for p in loads if p[3] <= address < p[3] + p[5])
        start = owner[2] + address - owner[3]
        strings = data[start:start + entries[10][0]]

    def string(value):
        return strings[value:strings.index(b'\0', value)].decode('utf-8')

    needed = [string(value) for value in entries.get(1, [])]
    soname = [string(value) for value in entries.get(14, [])]
    rpath = [string(value) for tag in (15, 29) for value in entries.get(tag, [])]
    # Assert that the rewrite leaves executable bytes and their virtual
    # addresses unchanged; patchelf is used for loader metadata only.
    executable = {}
    section_offset = struct.unpack_from('<Q', data, 40)[0]
    section_size, section_count, names_index = struct.unpack_from('<HHH', data, 58)
    if section_count:
        if section_size != 64 or section_offset + section_size * section_count > len(data):
            raise ValueError(f'Invalid ELF section headers: {filename}')
        sections = [struct.unpack_from('<IIQQQQIIQQ', data, section_offset + i * section_size) for i in range(section_count)]
        names_header = sections[names_index]
        names = data[names_header[4]:names_header[4] + names_header[5]]
        for section in sections:
            if section[2] & 4:
                name = names[section[0]:names.index(b'\0', section[0])].decode('utf-8')
                executable[name] = (section[3], section[5], hashlib.sha256(data[section[4]:section[4] + section[5]]).hexdigest())
    return dict(kind=kind, loads=loads, needed=needed, soname=soname, rpath=rpath,
                rpath_tag='RPATH' if 15 in entries and 29 not in entries else 'RUNPATH',
                executable=executable, sha256=hashlib.sha256(data).hexdigest())


def needs_alignment(info):
    return bool(info and info['kind'] == 3 and info['loads']
                and max(p[7] for p in info['loads']) < PAGE_SIZE
                and any((p[3] - p[2]) % PAGE_SIZE for p in info['loads']))


def dependency_paths(roots, library_cache):
    pending = list(roots)
    visited = {}
    r_libraries = [str(root.parent.parent.parent / 'lib') for root in roots if root.name == 'R']
    while pending:
        filename = Path(pending.pop()).resolve()
        if filename in visited:
            continue
        info = inspect_elf(filename)
        visited[filename] = info
        if info is None:
            continue
        search = [filename.parent] + [Path(p) for value in info['rpath'] for p in value.replace('${ORIGIN}', str(filename.parent)).replace('$ORIGIN', str(filename.parent)).split(':') if p]
        search += [Path(p) for p in r_libraries]
        for name in info['needed']:
            candidates = [directory / name for directory in search] + library_cache.get(name, [])
            dependency = next((path for path in candidates if path.is_file()), None)
            if dependency is None:
                raise RuntimeError(f'Cannot resolve {name} needed by {filename}')
            pending.append(dependency)
    return visited


def align_library(filename, before):
    stat = filename.stat()
    fd, temporary = tempfile.mkstemp(prefix='.vscode-elf-', dir=filename.parent)
    os.close(fd)
    try:
        shutil.copy2(filename, temporary)
        os.chown(temporary, stat.st_uid, stat.st_gid)
        old_rpath = subprocess.check_output(['patchelf', '--print-rpath', str(filename)], text=True).strip()
        arguments = ['patchelf', '--page-size', str(PAGE_SIZE)]
        if before['rpath_tag'] == 'RPATH':
            arguments.append('--force-rpath')
        subprocess.run(arguments + ['--set-rpath', old_rpath + (':' if old_rpath else '') + '$ORIGIN', temporary], check=True)
        after = inspect_elf(temporary)
        if after is None or needs_alignment(after) or before['needed'] != after['needed'] or before['soname'] != after['soname'] or before['executable'] != after['executable']:
            raise RuntimeError(f'ELF alignment changed code or ABI: {filename}')
        # Atomic replacement keeps running tools' mapped libraries intact.
        os.replace(temporary, filename)
        return dict(path=str(filename), originalSHA256=before['sha256'],
                    alignedSHA256=after['sha256'], soname=after['soname'])
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def main():
    roots = []
    for home in (Path('/usr/local/lib/R'), Path('/usr/lib/R')):
        if (home / 'bin/exec/R').is_file():
            roots.append(home / 'bin/exec/R')
            roots.extend((home / 'lib').glob('*.so'))
            roots.extend((home / 'modules').glob('*.so'))
            for package in ('stats', 'utils', 'grDevices', 'graphics', 'methods', 'tools', 'parallel'):
                roots.extend((home / 'library' / package / 'libs').glob('*.so'))
    if not roots:
        raise RuntimeError('Cannot locate the R runtime and its shared modules')
    cache = {}
    listing = subprocess.check_output(['ldconfig', '-p'], text=True)
    for line in listing.splitlines():
        match = re.match(r'\s*(\S+)\s+\([^)]*x86-64[^)]*\)\s+=>\s+(\S+)', line)
        if match:
            cache.setdefault(match[1], []).append(Path(match[2]))
    dependencies = dependency_paths(roots, cache)
    changed = [align_library(filename, info) for filename, info in sorted(dependencies.items()) if needs_alignment(info)]
    destination = Path('/usr/local/share/vscode-sandboxes/r-elf-alignment.json')
    destination.parent.mkdir(parents=True, exist_ok=True)
    previous = json.loads(destination.read_text()) if destination.exists() else {}
    version = subprocess.check_output(['patchelf', '--version'], text=True).strip()
    destination.write_text(json.dumps(dict(pageSize=PAGE_SIZE, tool=version,
                                          libraries=previous.get('libraries', []) + changed), indent=2) + '\n')
    print(f'R ELF alignment: repaired {len(changed)} of {len(dependencies)} runtime dependencies')


if __name__ == '__main__':
    main()
