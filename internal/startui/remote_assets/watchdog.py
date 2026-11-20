"""One detached login-node watchdog per HPC user; no per-allocation watchdogs."""
import contextlib, fcntl, json, os, pathlib, pwd, re, subprocess, sys, tempfile, time

IDLE_SECONDS = 24 * 60 * 60
POLL_SECONDS = 60
TOKEN = re.compile(r'^[0-9a-f]{32}$')
JOB = re.compile(r'^[0-9]+$')

def owned(path):
    return not path.is_symlink() and path.stat().st_uid == os.getuid()

def read_json(path):
    if not owned(path) or path.stat().st_size > 4 * 1024 * 1024:
        raise ValueError('Invalid watchdog metadata')
    return json.loads(path.read_text())

def atomic(path, value):
    fd, name = tempfile.mkstemp(prefix='.watchdog-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            json.dump(value, stream); stream.write('\n')
        os.replace(name, path)
    finally:
        if os.path.exists(name): os.unlink(name)

def identity(run, token, job):
    if not TOKEN.fullmatch(token) or not JOB.fullmatch(job) or not owned(run):
        raise ValueError('Invalid allocation identity')
    for name, expected in [('owner-token', token), ('job-id', job)]:
        target = run / name
        if not owned(target) or target.read_text().strip() != expected:
            raise ValueError('Allocation identity changed')

def proc_info(pid, proc=pathlib.Path('/proc')):
    directory = proc / str(pid)
    if directory.stat().st_uid != os.getuid(): raise ValueError('Wrong process owner')
    raw = (directory / 'stat').read_text()
    fields = raw[raw.rfind(')') + 2:].split()
    return {'ppid': int(fields[1]), 'start': fields[19], 'state': fields[0]}

def runtime_dir(run):
    return run / 'runtime' if (run / 'runtime').is_dir() else run

def record_server(run, pid):
    token = (run / 'owner-token').read_text().strip()
    job = (run / 'job-id').read_text().strip()
    identity(run, token, job)
    process = proc_info(pid)
    atomic(runtime_dir(run) / 'ssh-server.json', {'token': token, 'jobID': job, 'pid': pid,
           'start': process['start'], 'user': pwd.getpwuid(os.getuid()).pw_name,
           'pidNamespace': os.readlink('/proc/self/ns/pid'),
           'readyAt': time.time()})

def connected(run):
    # OpenSSH runs ~/.ssh/rc only after authentication, before the session command.
    token = (run / 'owner-token').read_text().strip()
    job = (run / 'job-id').read_text().strip()
    identity(run, token, job)
    if not os.environ.get('SSH_CONNECTION'):
        raise ValueError('Not an authenticated SSH session')
    run = runtime_dir(run)
    descriptor = os.open(run / 'ssh-activity.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        old = read_json(run / 'ssh-activity.json') if (run / 'ssh-activity.json').exists() else {}
        atomic(run / 'ssh-activity.json', {'token': token, 'jobID': job,
               'lastAuthenticatedAt': max(time.time(), old.get('lastAuthenticatedAt', 0))})

def probe(run, token, job, proc=pathlib.Path('/proc')):
    identity(run, token, job)
    run = runtime_dir(run)
    server = read_json(run / 'ssh-server.json')
    if server['token'] != token or server['jobID'] != job:
        raise ValueError('Wrong SSH server identity')
    root = int(server['pid'])
    if server.get('pidNamespace'):
        # The service has a private PID namespace. This trusted probe runs in a
        # host-side Slurm step and resolves the exact server namespace/start tick,
        # never attaching to or entering the agent's PID namespace.
        matches = []
        for directory in proc.iterdir():
            if not directory.name.isdigit(): continue
            try:
                candidate = int(directory.name)
                status = dict(line.split(':', 1) for line in (directory / 'status').read_text().splitlines() if ':' in line)
                if (proc_info(candidate, proc)['start'] == server['start'] and
                    int(status['NSpid'].split()[-1]) == int(server['pid']) and
                    os.readlink(directory / 'ns/pid') == server['pidNamespace']):
                    matches.append(candidate)
            except (OSError, ValueError, KeyError, IndexError): pass
        if len(matches) != 1: raise ValueError('Cannot identify isolated SSH server')
        root = matches[0]
    current = proc_info(root, proc)
    if current['start'] != server['start'] or current['state'] in ('Z', 'X'):
        raise ValueError('SSH server process changed')
    processes = {}
    for directory in proc.iterdir():
        if not directory.name.isdigit(): continue
        try: processes[int(directory.name)] = proc_info(int(directory.name), proc)
        except (OSError, ValueError, IndexError): pass
    count = 0
    authenticated = re.compile(r'sshd(?:-session)?: ' + re.escape(server['user']) + r'@')
    for pid, info in processes.items():
        if info['state'] in ('Z', 'X'): continue
        parent, visited = info['ppid'], set()
        while parent in processes and parent != root and parent not in visited:
            visited.add(parent); parent = processes[parent]['ppid']
        if parent != root: continue
        try:
            title = (proc / str(pid) / 'cmdline').read_bytes().replace(b'\0', b' ').decode(errors='replace')
            if authenticated.search(title): count += 1
        except OSError: pass
    last = 0
    if (run / 'ssh-activity.json').exists():
        event = read_json(run / 'ssh-activity.json')
        if event['token'] != token or event['jobID'] != job:
            raise ValueError('Wrong connection event identity')
        last = float(event['lastAuthenticatedAt'])
    return {'token': token, 'jobID': job, 'connections': count,
            'lastAuthenticatedAt': last, 'readyAt': server['readyAt']}

def advance(entry, observation, now, idle_seconds=IDLE_SECONDS):
    """A connection renews only this exact allocation, never another sandbox."""
    result = dict(entry)
    if observation['token'] != entry['token'] or observation['jobID'] != entry['jobID']:
        raise ValueError('Probe returned a different allocation')
    event = float(observation.get('lastAuthenticatedAt', 0))
    if observation['connections'] > 0:
        result.update(connected=True, idleSince=None)
    elif entry.get('connected') or event > entry.get('lastAuthenticatedAt', 0):
        result.update(connected=False, idleSince=now)
    else:
        result['connected'] = False
        if result.get('idleSince') is None:
            result['idleSince'] = max(float(observation['readyAt']), now)
    result['lastAuthenticatedAt'] = max(event, entry.get('lastAuthenticatedAt', 0))
    result['lastCheckedAt'] = now
    expire = result.get('idleSince') is not None and now - result['idleSince'] >= idle_seconds
    return result, expire

def root_path():
    return pathlib.Path.home() / '.local/share/vscode-sandboxes/watchdog'

@contextlib.contextmanager
def registry(root):
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    if not owned(root): raise ValueError('Invalid watchdog directory')
    descriptor = os.open(root / 'registry.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        file = root / 'allocations.json'
        data = read_json(file) if file.exists() else {}
        yield data
        atomic(file, data)

def register(run, token, job, root=None):
    root = root or root_path()
    run = run.resolve()
    identity(run, token, job)
    with registry(root) as entries:
        old = entries.get(token)
        if old and (old['jobID'] != job or old['runDir'] != str(run)):
            raise ValueError('Allocation registration changed')
        entries.setdefault(token, {'token': token, 'jobID': job, 'runDir': str(run),
                                  'registeredAt': time.time(), 'connected': False, 'idleSince': None})
        descriptor = os.open(root / 'daemon.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            try: fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError: return
            logfd = os.open(root / 'watchdog.log', os.O_CREAT | os.O_WRONLY | os.O_APPEND | os.O_NOFOLLOW, 0o600)
            try:
                # Inherit the held lock: simultaneous registrations cannot start two monitors.
                subprocess.Popen([sys.executable, str(pathlib.Path(__file__).resolve()),
                                  'daemon', str(root), str(descriptor)],
                                 stdin=subprocess.DEVNULL, stdout=logfd, stderr=logfd,
                                 start_new_session=True, close_fds=True, pass_fds=(descriptor,))
            finally: os.close(logfd)
        finally:
            # Do not LOCK_UN: the child owns the same open-file-description lock.
            os.close(descriptor)

def command(entry, args, timeout=25):
    run = pathlib.Path(entry['runDir'])
    identity(run, entry['token'], entry['jobID'])
    modules = run / 'modules.sh'
    if not owned(modules): raise ValueError('Invalid module setup')
    result = subprocess.run(['bash', '-c', 'set -eu; source "$1" >&2; shift; exec "$@"',
                             'watchdog', str(modules)] + args,
                            capture_output=True, text=True, timeout=timeout)
    if result.returncode: raise RuntimeError('Cluster command failed')
    return result.stdout

def queue(entry):
    result = command(entry, ['squeue', '--noheader', '--user', str(os.getuid()),
                            '--format=%i|%T|%k'])
    expected = entry['jobID'] + '|'
    for line in result.splitlines():
        if line.startswith(expected):
            fields = line.split('|')
            if len(fields) == 3 and fields[2] == 'vscode-sandboxes:' + entry['token']:
                return fields[1]
            return None
    return None

def observe(entry, help_cache):
    run = pathlib.Path(entry['runDir'])
    help_key = str(run / 'modules.sh')
    if help_key not in help_cache:
        help_cache[help_key] = command(entry, ['srun', '--help'])
    args = ['srun', '--jobid=' + entry['jobID'], '--nodes=1', '--ntasks=1',
            '--cpus-per-task=1', '--quiet', '--mpi=none']
    if '--overlap' in help_cache[help_key]: args.append('--overlap')
    if '--exact' in help_cache[help_key]: args.append('--exact')
    if (run / 'runtime').is_dir():
        # Host-side observation is required with --containall: a second private
        # container cannot inspect the service's PID namespace. No agent code or
        # writable script is executed on the host; the control directory is RO.
        args += ['python3', str(run / 'watchdog.py'), 'probe', str(run),
                 entry['token'], entry['jobID']]
        return json.loads(command(entry, args))
    image = (run / 'image').read_text().strip()
    apptainer = (run / 'apptainer').read_text().strip()
    args += [apptainer, 'exec', '--cleanenv', '--contain', '--bind',
             str(run) + ':/run/vscode-launcher:ro', image, 'python3',
             '/run/vscode-launcher/watchdog.py', 'probe', '/run/vscode-launcher',
             entry['token'], entry['jobID']]
    return json.loads(command(entry, args))

def log(message, entry=None):
    stamp = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    print(stamp, ('job=' + entry['jobID']) if entry else '', message, flush=True)

def monitor(root, descriptor, interval=POLL_SECONDS, idle_seconds=IDLE_SECONDS):
    help_cache = {}
    log('Watchdog started')
    while True:
        with registry(root) as entries:
            if not entries:
                atomic(root / 'status.json', {'pid': os.getpid(), 'running': False, 'count': 0})
                # Exit and registration share this critical section. A new registration
                # either reaches this daemon or starts its successor, never gets lost.
                fcntl.flock(descriptor, fcntl.LOCK_UN)
                log('No allocations remain; exiting')
                return
            snapshot = dict(entries)
        for token, entry in snapshot.items():
            remove, updated, expire = False, dict(entry), False
            try:
                identity(pathlib.Path(entry['runDir']), entry['token'], entry['jobID'])
            except (OSError, ValueError, KeyError):
                remove = True
            try:
                state = None if remove else queue(entry)
                if state is None: remove = True
                elif state == 'RUNNING':
                    run = pathlib.Path(entry['runDir'])
                    now = time.time()
                    if (run / 'ready').exists():
                        updated, expire = advance(entry, observe(entry, help_cache), now, idle_seconds)
                    else:
                        # No endpoint yet: no user can be connected. Bound stalled startup too.
                        updated.setdefault('preparingSince', now)
                        expire = now - updated['preparingSince'] >= idle_seconds
                    if expire:
                        # Recheck the live scheduler identity immediately before cancellation.
                        if queue(entry) == 'RUNNING':
                            # A user may have reconnected since the first observation.
                            if (run / 'ready').exists():
                                updated, expire = advance(updated, observe(entry, help_cache), time.time(), idle_seconds)
                            if expire:
                                command(entry, ['scancel', entry['jobID']])
                                log('Stopped after 24 hours without an authenticated connection', entry)
                                remove = True
                        else: remove = True
            except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError):
                # Unknown liveness is not evidence of disconnection. Never kill on probe failure.
                log('Could not verify connection state; retaining allocation and retrying', entry)
            with registry(root) as entries:
                current = entries.get(token)
                if current and current['jobID'] == entry['jobID'] and current['runDir'] == entry['runDir']:
                    if remove: entries.pop(token)
                    else: entries[token] = updated
        with registry(root) as entries:
            atomic(root / 'status.json', {'pid': os.getpid(), 'running': True,
                   'count': len(entries), 'checkedAt': time.time(), 'idleSeconds': idle_seconds})
            empty = not entries
        if not empty: time.sleep(interval)


def main():
    operation = sys.argv[1]
    if operation == 'register': register(pathlib.Path(sys.argv[2]), sys.argv[3], sys.argv[4])
    elif operation == 'server': record_server(pathlib.Path(sys.argv[2]), int(sys.argv[3]))
    elif operation == 'connected': connected(pathlib.Path(sys.argv[2]))
    elif operation == 'probe': print(json.dumps(probe(pathlib.Path(sys.argv[2]), sys.argv[3], sys.argv[4])))
    elif operation == 'daemon': monitor(pathlib.Path(sys.argv[2]), int(sys.argv[3]))
    else: raise ValueError('Unknown watchdog operation')

if __name__ == '__main__':
    try: main()
    except Exception:
        print('Watchdog operation failed; check server-side allocation metadata and Slurm access.', file=sys.stderr)
        sys.exit(1)
