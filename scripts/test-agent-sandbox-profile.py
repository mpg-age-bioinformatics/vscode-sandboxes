#!/usr/bin/env python3
"""Ensure namespace compatibility changes preserve Docker's other syscall rules."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1]
assets = root / 'internal/launcherbundle/bundles/common/assets'
profile = json.loads((assets / 'agent-sandbox-seccomp.json').read_text())
extra = profile['syscalls'].pop()
assert set(extra['names']) == {'clone', 'unshare', 'mount', 'umount2', 'pivot_root'}
assert extra['action'] == 'SCMP_ACT_ALLOW'
assert profile['defaultAction'] == 'SCMP_ACT_ERRNO'
canonical = json.dumps(profile, sort_keys=True, separators=(',', ':')).encode()
assert hashlib.sha256(canonical).hexdigest() == '8fbd0c8cf3e4bfe63e36a3584d2b5682e3a2aeade5986f1d699577edb4b14a9e', 'Docker baseline rules changed'
for language in ('python', 'r'):
    template = root / f'internal/launcherbundle/bundles/{language}-sandbox/assets/devcontainer.json.template'
    config = json.loads(template.read_text())
    assert config['securityOpt'] == ['seccomp=${localWorkspaceFolder}/code/.sandbox/agent-sandbox-seccomp.json']
    assert not config.get('privileged') and not config.get('capAdd')
    assert not any('docker.sock' in mount for mount in config['mounts'])
    setup = (template.parent.parent / 'scripts/setup-project.sh').read_text()
    for name in ('agent-sandbox-seccomp.json', 'agent-sandbox-seccomp-LICENSE.txt', 'agent-sandbox-seccomp-NOTICE.txt'):
        assert name in setup and (assets / name).is_file()
assert 'codex sandbox -- /bin/true' in (assets / 'verify-development-container.sh').read_text()
print('Docker seccomp baseline, namespace allowances, packaging, and startup verification checks passed.')
