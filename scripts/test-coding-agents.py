#!/usr/bin/env python3
"""Both commands are installed, visible in terminals, and reusable offline."""
from pathlib import Path
import os
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
installer = root/'internal/launcherbundle/bundles/common/assets/install-coding-agents.sh'
with tempfile.TemporaryDirectory(prefix='sandbox-agents-test-') as temporary:
    work = Path(temporary)
    home, binary = work/'home', work/'bin'
    binary.mkdir()
    log = work/'npm.log'
    npm = binary/'npm'
    npm.write_text('''#!/usr/bin/env python3
from pathlib import Path
import os,sys
args=sys.argv[1:]
prefix=Path(args[args.index('--prefix')+1])
package=args[-1]
binary='codex' if package.startswith('@openai/codex@') else 'claude'
with open(os.environ['AGENT_TEST_LOG'],'a') as f: f.write(package+'\\n')
command=prefix/'node_modules/.bin'/binary
command.parent.mkdir(parents=True)
command.write_text('#!/bin/sh\\necho '+package+'\\n')
command.chmod(0o755)
''')
    npm.chmod(0o755)
    env = dict(os.environ, HOME=str(home), PATH=f'{binary}:'+os.environ['PATH'], AGENT_TEST_LOG=str(log))
    for _ in range(2):
        result = subprocess.run(['bash', str(installer)], env=env, capture_output=True, text=True)
        assert result.returncode == 0, result.stderr
        assert '@openai/codex@0.163.0' in result.stdout
        assert '@anthropic-ai/claude-code@2.1.328' in result.stdout
    assert len(log.read_text().splitlines()) == 2, 'Reopening should not reinstall cached agents'
    result = subprocess.run(['bash','-c','source "$HOME/.profile"; command -v codex; command -v claude'],env=env,capture_output=True,text=True)
    assert result.returncode == 0 and len(result.stdout.splitlines()) == 2
    assert (home/'.bashrc').read_text().count('export PATH=') == 1
print('Both-agent installation, terminal PATH, and offline reuse checks passed.')
