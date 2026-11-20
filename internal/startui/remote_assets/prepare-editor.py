#!/usr/bin/env python3
"""Trusted provisioning helper: fresh home, no project/data/credentials mounts."""
import json
import os
import pathlib
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import time
import zipfile

def register_continue(home, arch, server):
    # Other CLI installs have already created the default extension inventory.
    # A manually copied, verified Continue directory must also be registered;
    # otherwise VS Code trusts that inventory and silently omits Continue.
    root = home/'.vscode-server/extensions'
    name = 'continue.continue-2.0.0-linux-'+arch
    inventory = root/'extensions.json'
    entries = json.loads(inventory.read_text()) if inventory.exists() else []
    if not isinstance(entries, list):
        raise RuntimeError('Invalid fresh editor extension inventory')
    entries = [entry for entry in entries if entry.get('identifier', {}).get('id', '').lower() != 'continue.continue']
    entries.append({'identifier': {'id': 'continue.continue'}, 'version': '2.0.0',
                    'location': {'$mid': 1, 'scheme': 'file', 'path': str(root/name)},
                    'relativeLocation': name,
                    'metadata': {'targetPlatform': 'linux-'+arch, 'pinned': True, 'isMachineScoped': True}})
    with tempfile.NamedTemporaryFile(mode='w', dir=root, delete=False) as output:
        json.dump(entries, output);temporary=output.name
    os.replace(temporary, inventory)
    listed = subprocess.check_output([str(server/'bin/code-server'), '--list-extensions'], text=True, timeout=30)
    if 'continue.continue' not in listed.lower().splitlines():
        raise RuntimeError('VS Code does not recognize the verified Continue extension')
    print('Verified pinned Continue in the actual VS Code extension inventory.', flush=True)


def prepare(run, home):
    profile = json.loads((run/'profile.json').read_text())
    commit = profile.get('editorCommit', '')
    if not re.fullmatch('[a-f0-9]{40}', commit):
        raise RuntimeError('Missing exact VS Code build; restart from an updated launcher')
    arch = {'x86_64': 'x64', 'aarch64': 'arm64'}.get(platform.machine())
    if not arch:
        raise RuntimeError('Unsupported VS Code Server architecture')
    server = home/'.vscode-server/bin'/commit
    server.mkdir(parents=True)
    print('Downloading exact VS Code Server in a helper without project or data access…', flush=True)
    with tempfile.TemporaryDirectory() as temporary:
        archive = pathlib.Path(temporary)/'server.tar.gz'
        with urllib.request.urlopen(f'https://update.code.visualstudio.com/commit:{commit}/server-linux-{arch}/stable', timeout=120) as source, archive.open('wb') as output:
            while True:
                data = source.read(1024*1024)
                if not data:
                    break
                output.write(data)
        # Validate all members and link destinations before extraction (older Python
        # runtimes do not implement tarfile's data extraction filter).
        unpacked = pathlib.Path(temporary)/'unpacked'
        unpacked.mkdir()
        with tarfile.open(archive) as bundle:
            for member in bundle:
                path = pathlib.PurePosixPath(member.name)
                if path.is_absolute() or '..' in path.parts or not (member.isfile() or member.isdir() or member.issym() or member.islnk()):
                    raise RuntimeError('Unsafe VS Code archive entry')
                if member.issym() or member.islnk():
                    link = pathlib.PurePosixPath(member.linkname)
                    if link.is_absolute() or '..' in link.parts:
                        raise RuntimeError('Unsafe VS Code archive link')
            bundle.extractall(unpacked, **({'filter': 'data'} if hasattr(tarfile, 'data_filter') else {}))
        contents = list(unpacked.iterdir())
        if len(contents) != 1 or not contents[0].is_dir():
            raise RuntimeError('Unexpected VS Code archive layout')
        for item in contents[0].iterdir():
            shutil.move(str(item), str(server/item.name))
    modern = home/'.vscode-server/cli/servers'/('Stable-'+commit)
    modern.mkdir(parents=True)
    (modern/'server').symlink_to(server, target_is_directory=True)
    extensions = profile.get('editorExtensions', [])
    installed = set()
    for path in (server/'extensions').glob('*/package.json'):
        manifest = json.loads(path.read_text())
        installed.add((manifest.get('publisher','')+'.'+manifest.get('name','')).lower())
    visiting = set()
    def fetch(request):
        for attempt in range(4):
            try:
                return urllib.request.urlopen(request, timeout=120)
            except OSError:
                if attempt == 3:
                    raise
                time.sleep(2 ** attempt)

    def install(extension):
        if not re.fullmatch(r'[A-Za-z0-9-]+\.[A-Za-z0-9-]+', extension):
            raise RuntimeError('Invalid extension identifier')
        identity = extension.lower()
        if identity in installed:
            return
        if identity in visiting:
            raise RuntimeError('Cyclic extension dependency')
        visiting.add(identity)
        # Query/download in Python: some sites reject the server CLI's gallery
        # query while allowing the public API and publisher CDN. Install the actual
        # VSIX with the official CLI, retaining its compatibility checks.
        payload = {'filters':[{'criteria':[{'filterType':7,'value':extension}]}], 'flags':147}
        request = urllib.request.Request('https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery', data=json.dumps(payload).encode(), headers={'Content-Type':'application/json','Accept':'application/json;api-version=3.0-preview.1'})
        with fetch(request) as source:
            results = json.load(source)['results'][0]['extensions']
        if len(results) != 1:
            raise RuntimeError('Extension not found: '+extension)
        platform_name = 'linux-'+arch
        versions = [item for item in results[0]['versions'] if not any(prop.get('key') == 'Microsoft.VisualStudio.Code.PreRelease' and prop.get('value') == 'true' for prop in item.get('properties', []))]
        version = next((item for item in versions if item.get('targetPlatform') == platform_name), None)
        if version is None:
            version = next((item for item in versions if item.get('targetPlatform') in (None, 'universal', 'undefined')), None)
        if version is None:
            raise RuntimeError('No extension build for '+platform_name+': '+extension)
        url = next(item['source'] for item in version['files'] if item['assetType'] == 'Microsoft.VisualStudio.Services.VSIXPackage')
        if not url.startswith('https://'):
            raise RuntimeError('Marketplace returned a non-HTTPS asset')
        print('Preparing extension '+extension+' '+version['version']+'…', flush=True)
        with tempfile.TemporaryDirectory() as temporary:
            vsix = pathlib.Path(temporary)/'extension.vsix'
            with fetch(url) as source, vsix.open('wb') as output:
                shutil.copyfileobj(source, output, 1024*1024)
            with zipfile.ZipFile(vsix) as archive:
                info = archive.getinfo('extension/package.json')
                if info.file_size > 1024*1024:
                    raise RuntimeError('Oversized extension manifest')
                manifest = json.loads(archive.read(info))
            if (manifest['publisher']+'.'+manifest['name']).lower() != identity or manifest['version'] != version['version']:
                raise RuntimeError('Extension identity mismatch')
            for dependency in manifest.get('extensionDependencies', []):
                install(dependency)
            subprocess.run([str(server/'bin/code-server'), '--install-extension', str(vsix), '--do-not-include-pack-dependencies', '--extensions-dir', str(home/'.vscode-server/extensions')], check=True, timeout=240)
        visiting.remove(identity)
        installed.add(identity)
        # Packs may reference an extension that depends on the pack's parent
        # (Python/debugpy); install the parent before expanding its pack.
        for packed in manifest.get('extensionPack', []):
            install(packed)

    for extension in extensions:
        install(extension)
    print('Preparing pinned Continue extension before workspace startup…', flush=True)
    bundle = subprocess.check_output(['python3', str(run/'prepare-continue.py')], text=True).strip().splitlines()[-1]
    tar = subprocess.Popen(['tar', '-C', str(pathlib.Path(bundle).parent), '-cf', '-', pathlib.Path(bundle).name], stdout=subprocess.PIPE)
    try:
        subprocess.run(['node', str(run/'install-continue.js'), 'install'], stdin=tar.stdout, check=True, timeout=180)
    finally:
        tar.stdout.close()
    if tar.wait() != 0:
        raise RuntimeError('Continue bundle transfer failed')
    register_continue(home, arch, server)
    print('Editor and extensions provisioned; project bootstrap will obey the selected internet policy.', flush=True)


if __name__ == '__main__':
    prepare(pathlib.Path('/run/vscode-launcher'), pathlib.Path.home())
