"""Fail before workspace startup if the requested kernel protections are absent."""
import ctypes, errno, json, pathlib, threading

status = dict(line.split(':', 1) for line in pathlib.Path('/proc/self/status').read_text().splitlines() if ':' in line)
for name in ('CapInh', 'CapPrm', 'CapEff', 'CapBnd', 'CapAmb'):
    if int(status.get(name, '1').strip(), 16):
        raise RuntimeError('Container has unexpected capabilities: ' + name)
if status.get('NoNewPrivs', '').strip() != '1' or status.get('Seccomp', '').strip() != '2':
    raise RuntimeError('NoNewPrivs and seccomp filtering are required')
libc = ctypes.CDLL(None, use_errno=True)
for name, args in [('unshare', (0,)), ('ptrace', (0x7fffffff, 0, 0, 0))]:
    ctypes.set_errno(0)
    if getattr(libc, name)(*args) != -1 or ctypes.get_errno() != errno.EPERM:
        raise RuntimeError('Required syscall restriction is absent: ' + name)
# The strict profile must retain clone/clone3 so Python, Node and agent threads work.
done = []
thread = threading.Thread(target=lambda: done.append(True)); thread.start(); thread.join()
if not done: raise RuntimeError('Thread creation failed')
print('Verified strict Apptainer security: zero capabilities, NoNewPrivs, seccomp, blocked ptrace/unshare', flush=True)
