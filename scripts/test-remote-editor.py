#!/usr/bin/env python3
"""Provision editor fixtures without vendor requests or running downloaded code."""
import importlib.util
import io
import json
import pathlib
import subprocess
import sys
import tarfile
import tempfile
import unittest
import zipfile
from unittest.mock import patch

sys.dont_write_bytecode=True
ASSET=pathlib.Path(__file__).resolve().parents[1]/'internal/startui/remote_assets/prepare-editor.py'
spec=importlib.util.spec_from_file_location('editor',ASSET)
e=importlib.util.module_from_spec(spec)
spec.loader.exec_module(e)


def server_archive(unsafe=False):
    stream=io.BytesIO()
    with tarfile.open(fileobj=stream,mode='w:gz') as tar:
        for name,body in [('server/package.json',b'{}'),('server/bin/code-server',b'fixture'), *([('server/../../escape',b'bad')] if unsafe else [])]:
            info=tarfile.TarInfo(name);info.size=len(body);tar.addfile(info,io.BytesIO(body))
    return stream.getvalue()


class EditorTests(unittest.TestCase):
    def setUp(self):
        machine=patch.object(e.platform,'machine',return_value='x86_64')
        machine.start();self.addCleanup(machine.stop)

    def test_dependency_packs_and_stable_versions(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=pathlib.Path(temporary);run=root/'run';run.mkdir();home=root/'home';home.mkdir()
            commit='a'*40
            (run/'profile.json').write_text(json.dumps({'editorCommit':commit,'editorExtensions':['test.python']}))
            bundle=root/'continue-bundle';bundle.mkdir();(bundle/'marker').write_text('fixture')
            manifests={
                'test.python':{'publisher':'test','name':'python','version':'1.0.0','extensionPack':['test.debug']},
                'test.debug':{'publisher':'test','name':'debug','version':'1.0.0','extensionDependencies':['test.python']},
            }
            installed=[];queries=[]
            def download(request,**kwargs):
                if not isinstance(request,str):
                    payload=json.loads(request.data);self.assertEqual(payload['flags'],147)
                    identity=payload['filters'][0]['criteria'][0]['value'];queries.append(identity)
                    value={'results':[{'extensions':[{'versions':[
                        {'version':'2.0.0','properties':[{'key':'Microsoft.VisualStudio.Code.PreRelease','value':'true'}],'files':[]},
                        {'version':'1.0.0','properties':[],'files':[{'assetType':'Microsoft.VisualStudio.Services.VSIXPackage','source':'https://fixture/'+identity}]},
                    ]}]}]}
                    return io.BytesIO(json.dumps(value).encode())
                if 'update.code' in request:return io.BytesIO(server_archive())
                archive=io.BytesIO()
                with zipfile.ZipFile(archive,'w') as package:package.writestr('extension/package.json',json.dumps(manifests[request.split('/')[-1]]))
                return io.BytesIO(archive.getvalue())
            def output(command,**kwargs):
                if command[0]=='python3':return str(bundle)+'\n'
                inventory=home/'.vscode-server/extensions/extensions.json'
                entries=json.loads(inventory.read_text())
                self.assertTrue(entries[-1]['metadata']['pinned'])
                return 'test.python\ntest.debug\nContinue.continue\n'
            def install(command,**kwargs):
                if command[0]=='node':kwargs['stdin'].read()
                else:
                    (home/'.vscode-server/extensions').mkdir(parents=True,exist_ok=True)
                    self.assertIn('--do-not-include-pack-dependencies',command)
                    with zipfile.ZipFile(command[command.index('--install-extension')+1]) as package:
                        installed.append(json.loads(package.read('extension/package.json'))['name'])
                return subprocess.CompletedProcess(command,0)
            with patch.object(e.urllib.request,'urlopen',side_effect=download),patch.object(e.subprocess,'run',side_effect=install),patch.object(e.subprocess,'check_output',side_effect=output):
                e.prepare(run,home)
            self.assertEqual(installed,['python','debug'])
            self.assertEqual(queries,['test.python','test.debug'])
            server=home/'.vscode-server/bin'/commit
            self.assertTrue((server/'bin/code-server').is_file())
            self.assertEqual((home/'.vscode-server/cli/servers'/('Stable-'+commit)/'server').resolve(),server.resolve())

    def test_unsafe_archive_fails_before_install(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=pathlib.Path(temporary);run=root/'run';run.mkdir()
            (run/'profile.json').write_text(json.dumps({'editorCommit':'a'*40}))
            with patch.object(e.urllib.request,'urlopen',return_value=io.BytesIO(server_archive(True))),patch.object(e.subprocess,'run') as install:
                with self.assertRaisesRegex(RuntimeError,'Unsafe VS Code archive entry'):e.prepare(run,root/'home')
                install.assert_not_called()
            self.assertFalse((root/'escape').exists())


if __name__=='__main__':unittest.main()
