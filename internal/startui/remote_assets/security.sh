# Shared, embedded into submission, batch and relay scripts by remoteAsset.
# Do not inherit runtime switches, host secrets, bind lists or loader injection.
secure_apptainer() {
 env -i PATH="$PATH" HOME="$HOME" USER="$(id -un)" LOGNAME="$(id -un)" \
  TMPDIR="${TMPDIR:-}" APPTAINER_TMPDIR="${APPTAINER_TMPDIR:-}" \
  APPTAINER_CACHEDIR="${APPTAINER_CACHEDIR:-$HOME/.apptainer/cache}" \
  "$apptainer" "$@"
}
security_assets() {
 cat > "$run/seccomp-agents.json" <<'VSCODE_SECCOMP_POLICY'
__VSCODE_SECCOMP_JSON__
VSCODE_SECCOMP_POLICY
 chmod 600 "$run/seccomp-agents.json"
}
security_mounts() {
 python3 -I - "$project" "$HOME" "$state" "$image" "$run" "${1:-}" <<'VSCODE_MOUNTS'
import os,pathlib,pwd,sys
project,home,state,image,run,folders=sys.argv[1:]
def variants(value):
 p=pathlib.Path(os.path.abspath(os.path.expanduser(value)))
 return {p,p.resolve()}
homes=variants(home)|variants(pwd.getpwuid(os.getuid()).pw_dir)
controls=variants(state)|variants(run)|variants(image)|variants(str(pathlib.Path(home)/'.local/share/vscode-sandboxes/watchdog'))
reserved=['/proc','/sys','/dev','/run','/tmp','/var/tmp','/etc','/usr','/bin','/sbin','/lib','/lib64']
def inside(p,parent):return p==parent or parent in p.parents
def check(value,writable):
 for p in variants(value):
  for h in homes:
   if inside(h,p):raise ValueError('Mounting home or an ancestor is forbidden: '+value)
   if inside(p,h):
    top=p.relative_to(h).parts[0]
    if top.startswith('.') or (writable and top=='bin'):
     raise ValueError('Mounting hidden home entries or writable ~/bin is forbidden: '+value)
  if any(inside(p,pathlib.Path(r)) for r in reserved):
   raise ValueError('Mounting container system/runtime paths is forbidden: '+value)
  if any(inside(p,c) or inside(c,p) for c in controls):
   raise ValueError('Mount overlaps launcher control files or the image: '+value)
check(project,True)
for folder in folders.split(','):
 folder=folder.strip()
 if folder:check(folder,False)
VSCODE_MOUNTS
}
