#!/usr/bin/env python3
"""Exercise idle clocks, SSH identity, and the singleton lifecycle without Slurm."""
import concurrent.futures
import fcntl
import importlib.util
import json
import os
import pathlib
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
ASSET = pathlib.Path(__file__).resolve().parents[1] / 'internal/startui/remote_assets/watchdog.py'
spec = importlib.util.spec_from_file_location('watchdog', ASSET)
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)


class WatchdogTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name).resolve()

    def allocation(self, n=1):
        token = '%032x' % n
        run = self.root / token
        run.mkdir()
        (run / 'owner-token').write_text(token)
        (run / 'job-id').write_text(str(n))
        (run / 'modules.sh').write_text('')
        (run / 'ready').touch()
        return dict(token=token, jobID=str(n), runDir=str(run))

    def observation(self, entry, connections=0, event=0):
        return dict(token=entry['token'], jobID=entry['jobID'], connections=connections,
                    readyAt=100, lastAuthenticatedAt=event)

    def test_independent_clocks_and_short_connections(self):
        a, b = self.allocation(), self.allocation(2)
        a, _ = w.advance(a, self.observation(a), 100)
        b, _ = w.advance(b, self.observation(b), 100)
        a, expired = w.advance(a, self.observation(a, connections=1), 86501)
        self.assertFalse(expired)
        self.assertIsNone(a['idleSince'])
        b, expired = w.advance(b, self.observation(b), 86501)
        self.assertTrue(expired)
        b, expired = w.advance(b, self.observation(b, event=86500), 86502)
        self.assertFalse(expired)
        self.assertEqual(b['idleSince'], 86502)
        # A fresh disconnect receives its own complete grace period.
        a, expired = w.advance(a, self.observation(a), 86503)
        self.assertFalse(expired)
        self.assertEqual(a['idleSince'], 86503)
        with self.assertRaises(ValueError):
            w.advance(a, self.observation(b), 86504)

    def test_full_grace_period_and_boundary(self):
        entry = self.allocation()
        entry, expired = w.advance(entry, self.observation(entry), 100)
        self.assertFalse(expired)
        entry, expired = w.advance(entry, self.observation(entry), 86499)
        self.assertFalse(expired)
        entry, expired = w.advance(entry, self.observation(entry), 86500)
        self.assertTrue(expired)

    def test_untrusted_future_and_nonfinite_times_never_extend_idle(self):
        for index, bad in enumerate((float('inf'), float('nan'), 999999999),1):
            entry = self.allocation(index)
            observation = self.observation(entry, event=bad)
            observation['readyAt'] = bad
            entry, expired = w.advance(entry, observation, 100)
            self.assertEqual(entry['idleSince'], 100)
            entry, expired = w.advance(entry, observation, 100+48*3600)
            self.assertTrue(expired)

    def test_host_anchor_ignores_runtime_metadata_and_fake_sshd(self):
        entry = self.allocation(); run = pathlib.Path(entry['runDir'])
        (run/'runtime').mkdir(); (run/'watchdog-policy').write_text('host-anchored-v2')
        proc = self.root/'proc'; proc.mkdir()
        self.process(proc, 5, 1, 'container launcher')
        self.process(proc, 10, 5, 'sshd -D')
        self.process(proc, 11, 10, 'sshd: '+w.pwd.getpwuid(os.getuid()).pw_name+'@notty')
        (proc/'10/ns').mkdir(); (proc/'10/ns/pid').symlink_to('pid:[42]')
        w.atomic(run/'runtime/ssh-server.json', dict(token=entry['token'], jobID=entry['jobID'], pid=10,start='77',user=w.pwd.getpwuid(os.getuid()).pw_name,readyAt=100))
        with self.assertRaises(ValueError): w.probe(run,entry['token'],entry['jobID'],proc)
        w.anchor_server(run,5,proc)
        before=w.probe(run,entry['token'],entry['jobID'],proc)
        self.assertTrue(before['hostAnchored']);self.assertEqual(before['connections'],1)
        w.atomic(run/'runtime/ssh-server.json', {'pid':999,'readyAt':float('inf')})
        w.atomic(run/'runtime/ssh-activity.json', {'lastAuthenticatedAt':float('inf')})
        self.process(proc,12,11,'bash')
        self.process(proc,13,12,'sshd: '+w.pwd.getpwuid(os.getuid()).pw_name+'@notty')
        after=w.probe(run,entry['token'],entry['jobID'],proc)
        self.assertEqual(after['connections'],1);self.assertEqual(after['lastAuthenticatedAt'],0)
        self.assertEqual(after['readyAt'],before['readyAt'])
        with patch.dict(os.environ, {'SSH_CONNECTION':'127.0.0.1 123 127.0.0.1 22'}):
            with patch.object(w.time,'time',return_value=200):
                w.connected_host(run,entry['token'],entry['jobID'])
        observed=w.probe(run,entry['token'],entry['jobID'],proc)
        self.assertEqual(observed['lastAuthenticatedAt'],200)
        self.assertTrue((run/'host-activity.json').is_file())


    def test_authenticated_hook_is_per_allocation_and_monotonic(self):
        a, b = self.allocation(), self.allocation(2)
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError): w.connected(pathlib.Path(a['runDir']))
        with patch.dict(os.environ, {'SSH_CONNECTION': '127.0.0.1 123 127.0.0.1 456'}):
            with patch.object(w.time, 'time', return_value=200): w.connected(pathlib.Path(a['runDir']))
            with patch.object(w.time, 'time', return_value=100): w.connected(pathlib.Path(a['runDir']))
        event = w.read_json(pathlib.Path(a['runDir'], 'ssh-activity.json'))
        self.assertEqual(event['lastAuthenticatedAt'], 200)
        self.assertEqual(event['token'], a['token'])
        self.assertFalse(pathlib.Path(b['runDir'], 'ssh-activity.json').exists())

    def test_registration_never_renews_clock(self):
        entry = self.allocation()
        entry.update(idleSince=100, lastAuthenticatedAt=90, connected=False)
        root = self.root / 'registry'
        with w.registry(root) as entries: entries[entry['token']] = entry
        fd = os.open(root / 'daemon.lock', os.O_CREAT | os.O_RDWR, 0o600)
        self.addCleanup(os.close, fd)
        fcntl.flock(fd, fcntl.LOCK_EX)
        with patch.object(w.subprocess, 'Popen') as spawn:
            w.register(pathlib.Path(entry['runDir']), entry['token'], entry['jobID'], root)
            spawn.assert_not_called()
        with w.registry(root) as entries: self.assertEqual(entries[entry['token']], entry)

    def test_registration_restarts_crashed_daemon_without_renewing_clock(self):
        entry = self.allocation()
        entry.update(idleSince=100, lastAuthenticatedAt=90, connected=False)
        root = self.root / 'registry'
        binary = self.root / 'squeue'
        binary.write_text('#!/bin/sh\nprintf "%s\\n" "1|PENDING|vscode-sandboxes:' + entry['token'] + '"\n')
        binary.chmod(0o700)
        pathlib.Path(entry['runDir'], 'modules.sh').write_text('export PATH="' + str(self.root) + ':$PATH"\n')
        with w.registry(root) as entries: entries[entry['token']] = entry
        workers, children = [], []
        spawn = w.subprocess.Popen
        def keep_child(*args, **kwargs):
            child = spawn(*args, **kwargs); children.append(child); return child
        patcher = patch.object(w.subprocess, 'Popen', side_effect=keep_child)
        patcher.start()
        self.addCleanup(patcher.stop)
        self.addCleanup(lambda: [child.wait(timeout=5) for child in children])
        def stop(pid):
            try: os.kill(pid, signal.SIGTERM)
            except ProcessLookupError: pass
        self.addCleanup(lambda: [stop(pid) for pid in workers])
        previous = None
        for _ in range(2):
            w.register(pathlib.Path(entry['runDir']), entry['token'], entry['jobID'], root)
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                status = w.read_json(root / 'status.json') if (root / 'status.json').exists() else {}
                if status.get('pid') not in (None, previous): break
                time.sleep(0.02)
            else: self.fail('Daemon failed to start or restart')
            pid = status['pid']; workers.append(pid)
            with w.registry(root) as entries:
                self.assertEqual(entries[entry['token']]['idleSince'], 100)
                self.assertEqual(entries[entry['token']]['lastAuthenticatedAt'], 90)
            stop(pid)
            fd = os.open(root / 'daemon.lock', os.O_RDWR)
            try:
                while True:
                    try: fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB); break
                    except BlockingIOError:
                        if time.monotonic() >= deadline: self.fail('Terminated daemon retained its lock')
                        time.sleep(0.02)
            finally: os.close(fd)
            children[-1].wait(timeout=5)
            workers.remove(pid)
            previous = pid

    def process(self, root, pid, parent, title, start='77'):
        directory = root / str(pid)
        directory.mkdir()
        fields = ['S', str(parent)] + ['0'] * 17 + [start]
        (directory / 'stat').write_text(str(pid) + ' (sshd) ' + ' '.join(fields))
        (directory / 'cmdline').write_bytes(title.encode() + b'\0')
        (directory / 'exe').symlink_to('/usr/sbin/sshd' if title.startswith('sshd') else '/usr/bin/bash')

    def test_exact_server_descendants_and_pid_reuse(self):
        entry = self.allocation()
        run = pathlib.Path(entry['runDir'])
        proc = self.root / 'proc'; proc.mkdir()
        self.process(proc, 10, 1, 'sshd -D')
        self.process(proc, 11, 10, 'sshd: user [preauth]')
        self.process(proc, 12, 10, 'sshd: user@notty')
        self.process(proc, 13, 12, 'bash')
        self.process(proc, 20, 1, 'sshd -D')
        self.process(proc, 21, 20, 'sshd: user@pts/0')
        self.process(proc, 22, 10, 'sshd: someone-else@notty')
        w.atomic(run / 'ssh-server.json', dict(token=entry['token'], jobID=entry['jobID'],
                 pid=10, start='77', user='user', readyAt=100))
        self.assertEqual(w.probe(run, entry['token'], entry['jobID'], proc)['connections'], 1)
        data = w.read_json(run / 'ssh-server.json'); data['start'] = '78'
        w.atomic(run / 'ssh-server.json', data)
        with self.assertRaises(ValueError): w.probe(run, entry['token'], entry['jobID'], proc)

    def test_private_namespace_pid_maps_to_exact_host_process(self):
        entry = self.allocation(); run = pathlib.Path(entry['runDir'])
        (run/'runtime').mkdir()
        proc = self.root/'proc'; proc.mkdir()
        self.process(proc,100,1,'sshd -D')
        self.process(proc,101,100,'sshd: user@notty')
        self.process(proc,200,1,'sshd -D')
        for pid,inner,ns in [(100,2,'pid:[42]'),(101,3,'pid:[42]'),(200,2,'pid:[43]')]:
            (proc/str(pid)/'status').write_text('NSpid:\t%d\t%d\n'%(pid,inner))
            (proc/str(pid)/'ns').mkdir()
            (proc/str(pid)/'ns/pid').symlink_to(ns)
        w.atomic(run/'runtime/ssh-server.json',dict(token=entry['token'],jobID=entry['jobID'],
                 pid=2,start='77',user='user',readyAt=100,pidNamespace='pid:[42]'))
        self.assertEqual(w.probe(run,entry['token'],entry['jobID'],proc)['connections'],1)
        (proc/'100/ns/pid').unlink();(proc/'100/ns/pid').symlink_to('pid:[99]')
        with self.assertRaises(ValueError):w.probe(run,entry['token'],entry['jobID'],proc)

    def test_probe_failure_never_cancels_and_exits_when_empty(self):
        entry = self.allocation()
        root = self.root / 'registry'
        with w.registry(root) as entries: entries[entry['token']] = entry
        fd = os.open(root / 'daemon.lock', os.O_CREAT | os.O_RDWR, 0o600)
        self.addCleanup(os.close, fd)
        fcntl.flock(fd, fcntl.LOCK_EX)
        states = iter(['RUNNING', None])
        with patch.object(w, 'queue', side_effect=lambda e: next(states)), \
             patch.object(w, 'observe', side_effect=RuntimeError('unreachable')), \
             patch.object(w, 'command') as command:
            w.monitor(root, fd, interval=0)
            command.assert_not_called()
        self.assertFalse(w.read_json(root / 'status.json')['running'])
        with w.registry(root) as entries: self.assertFalse(entries)
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)

    def test_final_probe_saves_reconnection(self):
        entry = self.allocation(); entry['idleSince'] = 1
        root = self.root / 'registry'
        with w.registry(root) as entries: entries[entry['token']] = entry
        fd = os.open(root / 'daemon.lock', os.O_CREAT | os.O_RDWR, 0o600)
        self.addCleanup(os.close, fd)
        fcntl.flock(fd, fcntl.LOCK_EX)
        states = iter(['RUNNING', 'RUNNING', None])
        observations = iter([self.observation(entry), self.observation(entry, connections=1)])
        with patch.object(w, 'queue', side_effect=lambda e: next(states)), \
             patch.object(w, 'observe', side_effect=lambda *a: next(observations)), \
             patch.object(w, 'command') as command:
            w.monitor(root, fd, interval=0)
            command.assert_not_called()

    def test_pending_jobs_do_not_expire_and_reused_ids_are_ignored(self):
        entry = self.allocation(); entry['idleSince'] = 1
        root = self.root / 'registry'
        with w.registry(root) as entries: entries[entry['token']] = entry
        fd = os.open(root / 'daemon.lock', os.O_CREAT | os.O_RDWR, 0o600)
        self.addCleanup(os.close, fd)
        states = iter(['PENDING', None])
        with patch.object(w, 'queue', side_effect=lambda e: next(states)), \
             patch.object(w, 'observe') as observe, patch.object(w, 'command') as command:
            w.monitor(root, fd, interval=0)
            observe.assert_not_called(); command.assert_not_called()
        with patch.object(w, 'command', return_value='1|RUNNING|vscode-sandboxes:other-token\n'):
            self.assertIsNone(w.queue(entry))
        # Missing allocation metadata cannot leave an immortal empty monitor.
        with w.registry(root) as entries: entries[entry['token']] = entry
        pathlib.Path(entry['runDir'], 'job-id').unlink()
        with patch.object(w, 'command') as command:
            w.monitor(root, fd, interval=0)
            command.assert_not_called()

    def test_expiration_and_identity_change(self):
        entry = self.allocation(); entry['idleSince'] = 1
        root = self.root / 'registry'
        with w.registry(root) as entries: entries[entry['token']] = entry
        fd = os.open(root / 'daemon.lock', os.O_CREAT | os.O_RDWR, 0o600)
        self.addCleanup(os.close, fd)
        with patch.object(w, 'queue', return_value='RUNNING'), \
             patch.object(w, 'observe', return_value=self.observation(entry)), \
             patch.object(w, 'command') as command:
            w.monitor(root, fd, interval=0)
            command.assert_called_once_with(entry, ['scancel', entry['jobID']])
        with w.registry(root) as entries: entries[entry['token']] = entry
        pathlib.Path(entry['runDir'], 'owner-token').write_text('different')
        with patch.object(w, 'queue') as queue:
            w.monitor(root, fd, interval=0)
            queue.assert_not_called()

    def test_concurrent_registration_has_one_daemon_and_exits(self):
        root = self.root / 'registry'
        allocations = [self.allocation(n) for n in range(1, 9)]
        # Fake scheduler blocks a little so all concurrent registrations overlap.
        binary = self.root / 'squeue'
        binary.write_text('#!/bin/sh\nsleep 0.2\n'); binary.chmod(0o700)
        for entry in allocations:
            pathlib.Path(entry['runDir'], 'modules.sh').write_text('export PATH="' + str(self.root) + ':$PATH"\n')
        def register(entry):
            return subprocess.run([sys.executable, '-c',
                'import runpy,pathlib,sys; w=runpy.run_path(sys.argv[1]); '
                'w["register"](pathlib.Path(sys.argv[2]),sys.argv[3],sys.argv[4],pathlib.Path(sys.argv[5]))',
                str(ASSET), entry['runDir'], entry['token'], entry['jobID'], str(root)],
                capture_output=True, timeout=10)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
            for result in executor.map(register, allocations):
                self.assertEqual(result.returncode, 0, result.stderr)
        # Registrations arriving after the daemon's snapshot are checked on its
        # next production poll, even when earlier registrations are already gone.
        deadline = time.monotonic() + w.POLL_SECONDS + 10
        while time.monotonic() < deadline:
            status = w.read_json(root / 'status.json') if (root / 'status.json').exists() else {}
            if status.get('running') is False: break
            time.sleep(0.05)
        else: self.fail('Watchdog did not exit after scheduler became empty:\n' + (root / 'watchdog.log').read_text())
        log = (root / 'watchdog.log').read_text()
        self.assertEqual(log.count('Watchdog started'), 1, log)
        with w.registry(root) as entries: self.assertFalse(entries)
        # A later registration starts a successor and it too exits without help.
        self.assertEqual(register(allocations[0]).returncode, 0)
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if (root / 'watchdog.log').read_text().count('No allocations remain; exiting') == 2: break
            time.sleep(0.05)
        else: self.fail('Successor did not exit')


if __name__ == '__main__': unittest.main(verbosity=2)
