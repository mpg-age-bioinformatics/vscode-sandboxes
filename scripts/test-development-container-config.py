#!/usr/bin/env python3
"""Verify real image locking, nested URI encoding, and credential mounts."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import shutil
from urllib.parse import unquote, urlsplit

root = Path(__file__).resolve().parents[1]
shared = root / 'internal/launcherbundle/bundles/common/assets'
for language, user, version in [('python', 'vscode', '3.13'), ('r', 'rstudio', '4.5')]:
    template = root / f'internal/launcherbundle/bundles/{language}-sandbox/assets/devcontainer.json.template'
    config = json.loads(template.read_text().replace('__PYTHON_VERSION__',version).replace('__R_VERSION__',version).replace('__AGENT__','codex'))
    assert config['remoteUser'] == user and config['containerUser'] == user
    assert config['shutdownAction'] == 'none'
    assert 'Continue.continue@2.0.0' in config['customizations']['vscode']['extensions']
    assert config['customizations']['vscode']['settings']['remote.extensionKind'] == {'Continue.continue':['workspace']}
    assert config['build']['dockerfile'] == '../Dockerfile'
    assert config['build']['context'] == '..'
    assert 'image' not in config
    assert not any('docker.sock' in mount for mount in config['mounts'])
    for directory in ('.codex', '.claude'):
        assert any(f'target=/home/{user}/{directory},' in mount for mount in config['mounts'])
    assert config['features']['ghcr.io/devcontainers/features/node:2.1.2']['version'] == '22.22.1'
    assert config['postCreateCommand'] == 'bash code/.sandbox/initialize-development-container.sh'
    assert config['workspaceFolder'] == '${localWorkspaceFolder}'

with tempfile.TemporaryDirectory(prefix='development-container-test-') as temporary:
    work = Path(temporary).resolve() / 'workspace with spaces and ü'
    (work / 'code/.sandbox').mkdir(parents=True)
    binary = Path(temporary) / 'bin'
    binary.mkdir()
    log, state = Path(temporary)/'docker.log', Path(temporary)/'pulled'
    fake = binary / 'docker'
    fake.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
args=sys.argv[1:]
assert os.environ['DOCKER_HOST']=='unix:///var/run/docker.sock'
assert not os.environ.get('DOCKER_CONTEXT')
with open(os.environ['DOCKER_TEST_LOG'],'a') as stream: stream.write(json.dumps(args)+'\\n')
state=Path(os.environ['DOCKER_TEST_STATE'])
if args[:2]==['image','inspect']:
    if not state.exists(): sys.exit(1)
    print(json.dumps([{'RepoDigests':['mcr.microsoft.com/devcontainers/python@sha256:'+('a'*64)],'Id':os.getenv('DOCKER_TEST_IMAGE_ID','sha256:'+('b'*64)),'Architecture':'arm64'}]))
elif args[0]=='pull': state.write_text(args[1])
elif args[0]=='tag': pass
elif args[0]=='info': print('Sandbox Docker Engine: test')
elif args[0]=='ps': print(os.getenv('DOCKER_TEST_CONTAINER',''))
else: sys.exit(3)
''')
    fake.chmod(0o755)
    (work/'code/.sandbox/devcontainer.json').write_text(json.dumps({'build':{'dockerfile':'../Dockerfile','context':'..'}}))
    (work/'code/.sandbox/sandbox-runtime.json').write_text(json.dumps({'baseImage':'mcr.microsoft.com/devcontainers/python:3.13-bookworm'}))
    (work/'code/Dockerfile').write_text('ARG BASE_IMAGE\nFROM ${BASE_IMAGE}\n')
    env = dict(os.environ,PATH=f'{binary}:{os.environ["PATH"]}', DOCKER_TEST_LOG=str(log),DOCKER_TEST_STATE=str(state))
    def run(*args, expected=0, **overrides):
        result=subprocess.run(['node', str(shared/'development-container.js'),*args],cwd=work,env=dict(env,**overrides),capture_output=True,text=True)
        assert result.returncode == expected, result.stderr
        return result.stdout.strip()
    run('prepare','codex')
    lock=json.loads((work/'code/.sandbox/container-image.lock.json').read_text())
    assert lock['resolvedImage'].endswith('@sha256:'+'a'*64)
    built=json.loads((work/'code/.sandbox/devcontainer.json').read_text())
    assert built['build']['args']['BASE_IMAGE']==lock['resolvedImage']
    before=log.read_text().count('["pull",')
    run('prepare','claude')
    assert log.read_text().count('["pull",') == before, 'cached images were pulled again'
    uri=run('uri','python-project-123-codex-dev')
    parsed=urlsplit(uri)
    payload,parent=parsed.netloc.removeprefix('dev-container+').split('@',1)
    settings=json.loads(bytes.fromhex(payload).decode())
    assert parent=='ssh-remote+python-project-123-codex-dev.sbx'
    assert settings['hostPath']==str(work) and settings['localDocker'] is False
    assert settings['settings']['host']=='unix:///var/run/docker.sock'
    assert settings['configFile']['path']==str(work/'code/.sandbox/devcontainer.json')
    assert settings['configFile']['scheme']=='vscode-remote'
    assert settings['configFile']['authority']==parent
    config_uri=run('config-uri','python-project-123-codex-dev')
    assert config_uri.startswith('vscode-remote://ssh-remote%2Bpython-project-123-codex-dev.sbx/')
    assert unquote(urlsplit(config_uri).netloc)==parent
    assert unquote(urlsplit(config_uri).path)==str(work/'code/.sandbox/devcontainer.json')
    assert unquote(parsed.path)==str(work)
    run('record')
    run('record',expected=1,DOCKER_TEST_IMAGE_ID='sha256:'+'c'*64)
    run('uri','../../bad',expected=1)
    lock['resolvedImage']='--all'
    (work/'code/.sandbox/container-image.lock.json').write_text(json.dumps(lock))
    run('prepare','codex',expected=1)
    # Exercise the real bootstrap dispatcher, recording its CLI invocations.
    (work/'code/.sandbox/container-image.lock.json').unlink()
    shutil.copy(shared/'development-container.js',work/'code/.sandbox/development-container.js')
    (work/'code/.sandbox/prepare-continue.py').write_text('print("/sandbox/cached-continue")\n')
    (work/'code/.sandbox/install-coding-agents.sh').write_text('#!/bin/sh\nexit 0\n')
    home=Path(temporary)/'sandbox-home'
    cli=home/'.local/share/vscode-sandboxes/devcontainers-cli/0.90.0/node_modules/.bin/devcontainer'
    cli.parent.mkdir(parents=True)
    cli_log=Path(temporary)/'cli.jsonl'
    cli.write_text('#!/usr/bin/env python3\nimport os,sys,json\nwith open(os.environ["CLI_TEST_LOG"],"a") as f: f.write(json.dumps(sys.argv[1:])+"\\n")\n')
    cli.chmod(0o755)
    copy=binary/'cp'
    copy.write_text('#!/bin/sh\n[ "$1" = /etc/ssl/certs/ca-certificates.crt ]\n')
    copy.chmod(0o755)
    result=subprocess.run(['bash',str(shared/'start-development-container.sh'),'codex','python-project-123-codex-dev'],cwd=work,env=dict(env,HOME=str(home),CLI_TEST_LOG=str(cli_log)),capture_output=True,text=True)
    assert result.returncode==0,result.stderr
    calls=[json.loads(line) for line in cli_log.read_text().splitlines()]
    assert [args[0] for args in calls]==['up','exec','exec','exec','exec']
    for args in calls:
        labels=[args[index+1] for index,value in enumerate(args) if value=='--id-label']
        assert labels==[f'devcontainer.local_folder={work}',f'devcontainer.config_file={config_uri}'],labels
    # A successful existing container is verified without reinstalling its environment.
    cli_log.write_text('')
    result=subprocess.run(['bash',str(shared/'start-development-container.sh'),'both','python-project-123-codex-dev','1'],cwd=work,env=dict(env,HOME=str(home),CLI_TEST_LOG=str(cli_log),DOCKER_TEST_CONTAINER='existing-container'),capture_output=True,text=True)
    assert result.returncode==0,result.stderr
    calls=[json.loads(line) for line in cli_log.read_text().splitlines()]
    assert not any('code/.sandbox/initialize-development-container.sh' in args for args in calls)
    assert any('code/.sandbox/verify-development-container.sh' in args for args in calls)
    assert 'Reusing the initialized development container' in result.stdout
print('Development-container config, image-lock, and nested URI checks passed.')
