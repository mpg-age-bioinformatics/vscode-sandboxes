# Runs over SSH on the login node, as the authenticated HPC user.
import contextlib,datetime,fcntl,json,os,pathlib,re,subprocess,sys,tempfile
operation,images_arg,state_arg=sys.argv[1:4]
payload=json.loads(sys.argv[4]) if len(sys.argv)>4 else {}
images=pathlib.Path(os.path.expanduser(images_arg))
state=pathlib.Path(os.path.expanduser(state_arg))
uid=os.getuid()
key_pattern=re.compile(r'^[0-9a-f]{64}$')
token_pattern=re.compile(r'^[0-9a-f]{32}$')
def owned(path):
    return not path.is_symlink() and path.stat().st_uid==uid

def read(path):
    if not owned(path) or path.stat().st_size>16*1024*1024: raise RuntimeError('Invalid or oversized metadata: '+str(path))
    return json.loads(path.read_text())

def atomic(path,value):
    fd,name=tempfile.mkstemp(prefix='.metadata-',dir=path.parent)
    try:
        with os.fdopen(fd,'w') as stream: json.dump(value,stream,indent=2);stream.write('\n')
        os.replace(name,path)
    finally:
        if os.path.exists(name): os.unlink(name)

@contextlib.contextmanager
def registry():
    images.mkdir(parents=True,exist_ok=True,mode=0o700)
    if not owned(images): raise RuntimeError('Image directory must be owned by your HPC user')
    lock=images/'.metadata.lock'
    descriptor=os.open(lock,os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    with os.fdopen(descriptor,'w') as stream:
        fcntl.flock(stream,fcntl.LOCK_EX)
        index=images/'metadata.json'
        data=read(index) if index.exists() else {'schema':1,'images':{}}
        if data.get('schema')!=1 or not isinstance(data.get('images'),dict): raise RuntimeError('Unsupported image metadata schema')
        yield data
        atomic(index,data)

def available(record):
    key=record['id']
    if not key_pattern.fullmatch(key): return False
    directory=images/key
    image=directory/'image.sif'
    marker=directory/'ready'
    if not image.is_file() or not marker.is_file() or not owned(image) or not owned(marker): return False
    if marker.read_text().strip()!=key: return False
    record.update(path=str(image.resolve()),size=image.stat().st_size,state='ready')
    return image.stat().st_size>0

def jobs():
    result=[]
    queue=subprocess.run(['squeue','--noheader','--user',str(uid),'--format=%i|%T|%N|%k'],check=True,capture_output=True,text=True).stdout
    for line in queue.splitlines():
        parts=line.split('|')
        if len(parts)!=4 or not parts[0].isdigit() or not parts[3].startswith('vscode-sandboxes:'): continue
        token=parts[3].split(':',1)[1]
        if not token_pattern.fullmatch(token): continue
        run=state/'runs'/token
        try:
            if not owned(run) or (run/'owner-token').read_text().strip()!=token or (run/'job-id').read_text().strip()!=parts[0]: continue
            profile=read(run/'profile.json')
            result.append({'profile':profile,'token':token,'jobID':parts[0],'runDir':str(run.resolve()),'active':True,'queueState':parts[1],'node':parts[2]})
        except (OSError,ValueError): continue
    return result

if operation=='images':
    with registry() as data:
        output=[record for record in data['images'].values() if available(record)]
    print(json.dumps(sorted(output,key=lambda r:r.get('createdAt',''),reverse=True)))
elif operation=='lookup':
    with registry() as data:
        if payload.get('id'):
            record=data['images'].get(payload['id'])
            output=record if record and available(record) else None
        else:
            matches=[r for r in data['images'].values() if r.get('recipeKey')==payload.get('recipeKey') and available(r)]
            output=max(matches,key=lambda r:r.get('createdAt','')) if matches else None
    print(json.dumps(output))
elif operation=='reserve':
    key=payload['id']
    if not key_pattern.fullmatch(key): raise RuntimeError('Invalid image identifier')
    with registry() as data:
        directory=images/key
        directory.mkdir(mode=0o700,exist_ok=True)
        if not owned(directory): raise RuntimeError('Invalid image cache entry')
        record=data['images'].get(key)
        if record and available(record): output=record
        else:
            payload.update(state='preparing',path=str((directory/'image.sif').resolve()),createdAt=datetime.datetime.now(datetime.timezone.utc).isoformat())
            data['images'][key]=payload
            output=payload
    print(json.dumps(output))
elif operation in ('jobs','authorize','stop'):
    entries=jobs()
    if operation=='jobs': print(json.dumps(entries))
    else:
        selected=next((entry for entry in entries if entry['token']==payload['token'] and entry['jobID']==payload['jobID']),None)
        if selected is None: raise RuntimeError('The selected sandbox is no longer running or its identity changed')
        if operation=='stop':
            subprocess.run(['scancel',selected['jobID']],check=True)
        else:
            key=payload['publicKey'].strip()
            if not re.fullmatch(r'ssh-ed25519 [A-Za-z0-9+/=]+(?: [^\r\n]*)?',key): raise RuntimeError('Invalid SSH key')
            path=pathlib.Path(selected['runDir'])/'authorized_keys'
            if not owned(path): raise RuntimeError('Invalid allocation key file')
            with path.open('a+') as stream:
                fcntl.flock(stream,fcntl.LOCK_EX)
                stream.seek(0);existing=stream.read().splitlines()
                if key not in existing: stream.write('\n'+key+'\n')
        print(json.dumps(selected))
else: raise RuntimeError('Unknown catalog operation')
