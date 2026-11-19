#!/usr/bin/env python3
"""Check offline system-library reuse and failure on incompatible package locks."""
from pathlib import Path
import tempfile,subprocess,os,json
root=Path(__file__).resolve().parents[1]
script=root/'internal/launcherbundle/bundles/r-sandbox/assets/install-r-system-dependencies.sh'
with tempfile.TemporaryDirectory(prefix='r-system-package-test-') as temporary:
 p=Path(temporary); (p/'bin').mkdir(); (p/'code/.sandbox').mkdir(parents=True)
 fake=p/'bin/dpkg-query'
 fake.write_text('''#!/usr/bin/env python3
import os,sys
package=sys.argv[-1]
if package=='libfreetype6-dev': sys.exit(1)
if 'Status-Status' in sys.argv[-2]: print('installed',end='')
else: print('' if os.getenv('EMPTY_VERSION')==package else '1.2.3',end='')
'''); fake.chmod(0o755)
 for name in ['sudo','apt-get']:
  f=p/'bin'/name; f.write_text('#!/bin/sh\necho "Unexpected package-network access" >&2\nexit 99\n'); f.chmod(0o755)
 env=dict(os.environ,PATH=f'{p}/bin:'+os.environ['PATH'])
 run=lambda **extra:subprocess.run(['bash',str(script)],cwd=p,env=dict(env,**extra),capture_output=True,text=True)
 first=run(); assert first.returncode==0,first.stderr
 lock=p/'code/.sandbox/r-system-packages.lock'; saved=lock.read_text()
 assert 'libfreetype-dev=1.2.3' in saved and 'libfreetype6-dev=' not in saved
 assert run().returncode==0, 'Installed packages should reopen without apt'
 lock.write_text(saved.replace('cmake=1.2.3','cmake=9.9.9'))
 drift=run(); assert drift.returncode!=0 and 'differs' in drift.stderr
 lock.unlink()
 empty=run(EMPTY_VERSION='cmake'); assert empty.returncode!=0 and not lock.exists()
print('R package-lock offline reuse, drift, and missing-version checks passed.')
