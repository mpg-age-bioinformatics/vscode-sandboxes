#!/usr/bin/env python3
"""Exercise Continue distribution preparation with no saved IDE/user policies."""
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
from unittest.mock import patch
import zipfile

sys.dont_write_bytecode = True

root = Path(__file__).resolve().parents[1]
assets = root / "internal/launcherbundle/bundles/common/assets"
spec = importlib.util.spec_from_file_location("prepare_continue", assets / "prepare-continue.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def fixture(path, bad_path=False, backend_count=10):
    with zipfile.ZipFile(path, "w") as archive:
        archive.writestr("extension/package.json", json.dumps({"name":"continue","publisher":"Continue","version":"2.0.0"}))
        archive.writestr("extension/out/extension.js", "const tools=[" + ",".join('{'+module.BACKEND_FROM+'}' for _ in range(backend_count)) + ',{defaultToolPolicy: "disabled"}];let fetchSearchResults;'+module.SEARCH_FROM+'return "fixture-result";};')
        archive.writestr("extension/gui/assets/index.js", module.GUI_FROM + ';')
        archive.writestr("extension/LICENSE", "Vendor license fixture")
        if bad_path:
            archive.writestr("extension/../escape", "must never escape")
    return module.digest(path)


with tempfile.TemporaryDirectory(prefix="continue-distribution-") as temporary:
    directory = Path(temporary)
    archive = directory / "package.vsix"
    checksum = fixture(archive)
    target = "linux-arm64"
    with patch.dict(module.ARCHIVES, {target:checksum}):
        package = directory / "prepared" / "continue.continue-2.0.0-linux-arm64"
        module.extract_bundle(archive, package, target, checksum)
        assert module.valid_bundle(package,target)
        assert (package/"LICENSE").read_text()=="Vendor license fixture"
        backend=(package/"out/extension.js").read_text()
        assert backend.count(module.BACKEND_FROM)==10 and 'allowedWithoutPermission' not in backend
        assert 'defaultToolPolicy: "disabled"' in backend, "Excluded tools must remain excluded"
        # Evaluate actual generated defaults in a new JS process, with no user storage.
        result=subprocess.run(['node','-e',backend+';console.log(JSON.stringify(tools.map(t=>t.defaultToolPolicy)))'],capture_output=True,text=True,check=True)
        assert json.loads(result.stdout)==['allowedWithPermission']*10+['disabled']
        gui=(package/"gui/assets/index.js").read_text()
        result=subprocess.run(['node','-e',gui+';console.log(Xk)'],capture_output=True,text=True,check=True)
        assert result.stdout.strip()=='allowedWithPermission'
        # The shared search function covers both the tool and @web provider.
        policy_home=directory/'policy-home';(policy_home/'.continue').mkdir(parents=True)
        for mode,allowed in [('restricted',False),('none',False),('open',True)]:
            (policy_home/'.continue/.sandbox-network.json').write_text(json.dumps({'network':mode}))
            result=subprocess.run(['node','-e',backend+';fetchSearchResults("test",5,()=>{throw Error("must not fetch")}).then(x=>console.log(x)).catch(e=>{console.error(e.message);process.exit(2)})'],capture_output=True,text=True,env={**os.environ,'HOME':str(policy_home)})
            assert (result.returncode==0)==allowed,(mode,result.stderr)
        (policy_home/'.continue/.sandbox-network.json').unlink()
        result=subprocess.run(['node','-e',backend+';fetchSearchResults("test",5,null).catch(e=>process.exit(2))'],env={**os.environ,'HOME':str(policy_home)})
        assert result.returncode==2,'Missing policy must not grant search access'
        assert json.loads((package/"package.json").read_text())["__metadata"]["pinned"] is True
        # Cached preparation must work with networking unavailable.
        home=directory/"home"
        expected=home/".cache/vscode-sandboxes/continue/2.0.0/linux-arm64/bundle"/package.name
        expected.parent.mkdir(parents=True)
        import shutil
        shutil.copytree(package,expected)
        with patch.object(module.Path,'home',return_value=home),patch.object(module.platform,'machine',return_value='aarch64'),patch.object(module.sys,'platform','linux'),patch.object(module.urllib.request,'urlopen',side_effect=AssertionError('unexpected network')):
            assert module.prepare()==expected
        # New-user download path also handles the marketplace's gzip transport.
        fresh=directory/"fresh-home"
        with patch.object(module.Path,'home',return_value=fresh),patch.object(module.platform,'machine',return_value='aarch64'),patch.object(module.sys,'platform','linux'),patch.object(module.urllib.request,'urlopen',return_value=io.BytesIO(gzip.compress(archive.read_bytes()))) as download:
            prepared=module.prepare()
            assert module.valid_bundle(prepared,target) and download.call_count==1
        (package/"gui/assets/index.js").write_text('tampered')
        assert not module.valid_bundle(package,target), "A marker alone must not authorize broken defaults"
    for mode in ["checksum", "traversal", "schema"]:
        source=directory/(mode+'.vsix')
        digest=fixture(source,bad_path=mode=='traversal',backend_count=9 if mode=='schema' else 10)
        try:
            module.extract_bundle(source,directory/mode,target,'0'*64 if mode=='checksum' else digest)
        except ValueError:
            pass
        else:
            raise AssertionError('Accepted invalid '+mode+' package')
    # Verify the real Linux installation helper with a fresh extension home.
    if sys.platform=='linux':
        platform_target='linux-'+({'aarch64':'arm64','arm64':'arm64','x86_64':'x64'}[module.platform.machine()])
        payload=directory/'payload'/('continue.continue-2.0.0-'+platform_target)
        module.extract_bundle(archive,payload,platform_target,checksum)
        tarpath=directory/'payload.tar'
        with tarfile.open(tarpath,'w') as stream:stream.add(payload,arcname=payload.name)
        env=dict(os.environ,HOME=str(directory/'new-user'))
        with tarpath.open('rb') as stream:
            subprocess.run(['node',str(assets/'install-continue.js'),'install'],stdin=stream,env=env,check=True)
        subprocess.run(['node',str(assets/'install-continue.js'),'check'],env=env,check=True)
        installed=Path(env['HOME'])/'.vscode-server/extensions'/payload.name
        (installed/'out/extension.js').write_text('bad')
        assert subprocess.run(['node',str(assets/'install-continue.js'),'check'],env=env).returncode!=0
print('Fresh-user Continue defaults, package verification, offline reuse, and installation checks passed.')
