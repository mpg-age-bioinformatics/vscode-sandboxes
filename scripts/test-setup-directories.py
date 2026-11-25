#!/usr/bin/env python3
"""Simulate delayed directory visibility without modifying a real workspace."""
import os,pathlib,shutil,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parents[1]
helper=root/'internal/launcherbundle/bundles/common/assets/setup-skills.sh'
real=shutil.which('mkdir')
with tempfile.TemporaryDirectory(prefix='sandbox-directory-test-') as temporary:
 base=pathlib.Path(temporary);binary=base/'bin';binary.mkdir()
 wrapper=binary/'mkdir'
 wrapper.write_text('''#!/bin/sh
count=0
[ ! -f "$DIRECTORY_COUNT" ] || count=$(cat "$DIRECTORY_COUNT")
count=$((count+1));printf '%s' "$count" > "$DIRECTORY_COUNT"
case "$DIRECTORY_FAILURE" in
 permission) echo 'mkdir: Permission denied' >&2;exit 1;;
 always) echo 'mkdir: Already exists' >&2;exit 1;;
 once) if [ "$count" -eq 1 ];then echo 'mkdir: Already exists' >&2;exit 1;fi;;
esac
exec "$DIRECTORY_REAL_MKDIR" "$@"
''');wrapper.chmod(0o755)
 for failure,expected,success in [('once',2,True),('always',3,False),('permission',1,False)]:
  counter=base/(failure+'.count');target=base/(failure+' app')/'Contents'/'Resources'
  env=os.environ|{'PATH':str(binary)+os.pathsep+os.environ['PATH'],'DIRECTORY_COUNT':str(counter),'DIRECTORY_FAILURE':failure,'DIRECTORY_REAL_MKDIR':real}
  result=subprocess.run(['bash','-c','source "$1"; ensure_sandbox_directory "$2"','test',str(helper),str(target)],env=env,capture_output=True,text=True)
  assert (result.returncode==0)==success,(failure,result.stderr)
  assert int(counter.read_text())==expected,failure
  if success:
   before=counter.read_text();subprocess.run(['bash','-c','source "$1"; ensure_sandbox_directory "$2"','test',str(helper),str(target)],env=env,check=True)
   assert counter.read_text()==before,'Existing directories must not be recreated'
 conflict=base/'conflict';conflict.write_text('preserve me')
 result=subprocess.run(['bash','-c','source "$1"; ensure_sandbox_directory "$2"','test',str(helper),str(conflict)],capture_output=True,text=True)
 assert result.returncode!=0 and conflict.read_text()=='preserve me'
print('Transient EEXIST recovery, bounded retries, existing directories, file preservation and permission failures passed.')
