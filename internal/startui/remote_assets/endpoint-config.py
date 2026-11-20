# Apply only launcher-managed configuration in the allocation-local home.
import json,os,pathlib,sys,tempfile
try:
    config=json.load(sys.stdin)
    home=pathlib.Path.home()
    def write(target,data):
        if target.is_symlink(): raise ValueError('Symlinked configuration')
        with tempfile.NamedTemporaryFile(mode='w',dir=target.parent,delete=False) as stream:
            stream.write(data);name=stream.name
        os.chmod(name,0o600)
        os.replace(name,target)
    def apply(folder,filename,backup_name,data):
        directory=home/folder
        if directory.is_symlink(): raise ValueError('Symlinked configuration directory')
        directory.mkdir(mode=0o700,exist_ok=True)
        os.chmod(directory,0o700)
        target=directory/filename;backup=directory/backup_name;marker=directory/'.sandbox-managed'
        for candidate in (target,backup,marker):
            if candidate.is_symlink(): raise ValueError('Symlinked configuration')
        if data is not None:
            if not marker.exists():
                if target.exists(): write(backup,target.read_text())
                write(marker,'')
            write(target,data)
        elif marker.exists():
            if backup.exists(): os.replace(backup,target)
            else: target.unlink(missing_ok=True)
            marker.unlink()
    if config.get('continue') is not None and os.environ.get('HTTP_PROXY'):
        connection=json.loads(config['continue'])
        for model in connection.get('models',[]):
            model.setdefault('requestOptions',{})['proxy']=os.environ['HTTP_PROXY']
        config['continue']=json.dumps(connection,indent=2)+'\n'
    apply('.continue','config.yaml','.sandbox-original-config.yaml',config.get('continue'))
    apply('.codex','config.toml','.sandbox-original.toml',config.get('codex'))
    print('ENDPOINT_CONFIGURED')
except Exception:
    print('Could not apply private endpoint settings inside the remote container. Existing credentials are not printed.',file=sys.stderr)
    sys.exit(1)
