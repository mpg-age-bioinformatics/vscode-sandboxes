#!/usr/bin/env python3
"""Allocation gateway and byte bridges. Host policy is never read from the project."""
import errno
import ipaddress
import json
import os
import re
import socket
import socketserver
import sys
import threading
import urllib.parse


def authority(value, default=None):
    if not value or any(c.isspace() or ord(c) < 33 for c in value) or any(c in value for c in '/@\\?#%'):
        raise ValueError('invalid authority')
    match = re.fullmatch(r'(\[[0-9a-fA-F:]+\]|[a-zA-Z0-9_.*-]+)(?::([0-9]+))?', value)
    if not match:
        raise ValueError('invalid authority')
    host = match[1].lower().rstrip('.')
    if host.startswith('['):
        host = str(ipaddress.IPv6Address(host[1:-1]))
    port = int(match[2]) if match[2] else default
    if port is not None and not 1 <= port <= 65535:
        raise ValueError('invalid port')
    return host, port


def allowed(host, port, rules):
    for rule in rules:
        name, target_port = authority(rule)
        if (target_port is not None and target_port != port) or (target_port is None and port not in (80, 443)):
            continue
        if name.startswith('**.'):
            base = name[3:]
            matches = host == base or host.endswith('.' + base)
        elif name.startswith('*.'):
            matches = host.endswith(name[1:]) and host != name[2:]
        else:
            matches = name == host
        if matches:
            return True
    return False


def transfer(left, right):
    """Bounded-memory duplex forwarding, including TCP half closes."""
    def copy(source, destination):
        try:
            while True:
                data = source.recv(65536)
                if not data:
                    break
                destination.sendall(data)
        except OSError:
            pass
        finally:
            try:
                destination.shutdown(socket.SHUT_WR)
            except OSError:
                pass
    thread = threading.Thread(target=copy, args=(left, right), daemon=True)
    thread.start()
    copy(right, left)
    thread.join(timeout=5)


class Server(socketserver.ThreadingMixIn, socketserver.UnixStreamServer):
    daemon_threads = True
    request_queue_size = 64
    # Bounds open connections and threads under a malicious workspace.
    slots = threading.BoundedSemaphore(128)

    def process_request(self, request, address):
        if not self.slots.acquire(blocking=False):
            request.close()
            return
        try:
            super().process_request(request, address)
        except BaseException:
            self.slots.release()
            raise

    def process_request_thread(self, request, address):
        try:
            super().process_request_thread(request, address)
        finally:
            self.slots.release()

    def handle_error(self, request, address):
        pass  # Never dump headers, URLs, credentials or project content to logs.


class Gateway(socketserver.BaseRequestHandler):
    def handle(self):
        upstream = None
        response_started = False
        try:
            self.request.settimeout(20)
            header = bytearray()
            # Read exactly the headers: CONNECT clients may immediately send TLS.
            while not header.endswith(b'\r\n\r\n'):
                byte = self.request.recv(1)
                if not byte or len(header) >= 32768:
                    raise ValueError('oversized/incomplete headers')
                header.extend(byte)
            lines = header[:-4].decode('ascii').split('\r\n')
            method, target, version = lines.pop(0).split(' ')
            if version not in ('HTTP/1.0', 'HTTP/1.1') or not re.fullmatch('[A-Z]+', method) or any(ord(c) <= 32 or ord(c) == 127 for c in target):
                raise ValueError('invalid request')
            headers = {}
            for line in lines:
                key, value = line.split(':', 1)
                if not re.fullmatch(r"[!#$%&'*+.^_`|~0-9a-zA-Z-]+", key) or any(ord(c) < 32 or ord(c) == 127 for c in value):
                    raise ValueError('invalid header')
                key = key.lower()
                if key in headers:
                    raise ValueError('duplicate header')
                headers[key] = value.strip()
            if 'transfer-encoding' in headers or 'upgrade' in headers:
                raise ValueError('unsupported request framing')
            if method == 'CONNECT':
                host, port = authority(target)
                if port is None or 'content-length' in headers:
                    raise ValueError('invalid CONNECT')
            else:
                url = urllib.parse.urlsplit(target)
                if url.scheme != 'http' or url.username or url.password or url.fragment:
                    raise ValueError('use CONNECT for HTTPS')
                host, port = authority(url.netloc, 80)
            if 'host' in headers and authority(headers['host'], port) != (host, port):
                raise ValueError('Host does not match destination')
            if not allowed(host, port, self.server.rules):
                self.request.sendall(b'HTTP/1.1 403 Forbidden\r\nConnection: close\r\nContent-Length: 0\r\n\r\n')
                return
            # Resolve on the gateway only. One selected address is used by connect;
            # container DNS and all direct IP traffic remain unavailable.
            upstream = socket.create_connection((host, port), timeout=20)
            upstream.settimeout(3600)
            if method == 'CONNECT':
                self.request.sendall(b'HTTP/1.1 200 Connection Established\r\n\r\n')
                response_started = True
                self.request.settimeout(3600)
                upstream.settimeout(3600)
                transfer(self.request, upstream)
            else:
                length = headers.get('content-length', '0')
                if not re.fullmatch('[0-9]+', length) or int(length) > 64 * 1024 * 1024:
                    raise ValueError('invalid body length')
                connection = {item.strip().lower() for item in headers.get('connection', '').split(',')}
                if connection & {'content-length', 'host', 'transfer-encoding'}:
                    raise ValueError('unsafe Connection header')
                omitted = connection | {'host', 'connection', 'proxy-connection', 'proxy-authorization', 'keep-alive', 'te', 'trailer', 'expect'}
                path = url.path or '/'
                if url.query:
                    path += '?' + url.query
                request = f'{method} {path} HTTP/1.1\r\nHost: {url.netloc}\r\nConnection: close\r\n'
                request += ''.join(f'{key}: {value}\r\n' for key, value in headers.items() if key not in omitted)
                upstream.sendall((request + '\r\n').encode('ascii'))
                if headers.get('expect', '').lower() == '100-continue':
                    self.request.sendall(b'HTTP/1.1 100 Continue\r\n\r\n')
                remaining = int(length)
                while remaining:
                    data = self.request.recv(min(remaining, 65536))
                    if not data:
                        raise ValueError('truncated body')
                    upstream.sendall(data)
                    remaining -= len(data)
                # Exactly one validated HTTP request; never forward pipelined bytes.
                while True:
                    data = upstream.recv(65536)
                    if not data:
                        break
                    response_started = True
                    self.request.sendall(data)
        except (OSError, ValueError):
            try:
                if response_started:
                    return
                self.request.sendall(b'HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\nContent-Length: 0\r\n\r\n')
            except OSError:
                pass
        finally:
            if upstream:
                upstream.close()


class Bridge(socketserver.BaseRequestHandler):
    def handle(self):
        try:
            if self.server.destination[0] == 'unix':
                target = socket.socket(socket.AF_UNIX)
                target.connect(self.server.destination[1])
            else:
                target = socket.create_connection(self.server.destination, timeout=20)
                target.settimeout(None)
            with target:
                transfer(self.request, target)
        except OSError:
            pass


def main():
    mode = sys.argv[1]
    if mode == 'gateway':
        profile = json.load(open(sys.argv[2]))
        if profile.get('network') != 'restricted':
            raise ValueError('gateway requires restricted policy')
        rules = [item for item in re.split(r'[,\s]+', profile.get('sites', '')) if item]
        for rule in rules:
            authority(rule)
        os.chdir(sys.argv[3])
        with Server('proxy.sock', Gateway) as server:
            server.rules = tuple(rules)
            os.chmod('proxy.sock', 0o600)
            server.serve_forever()
    elif mode == 'proxy-bridge':
        os.chdir(sys.argv[2])
        class TCPServer(socketserver.ThreadingMixIn, socketserver.TCPServer):
            daemon_threads = True
        with TCPServer(('127.0.0.1', int(sys.argv[3])), Bridge) as server:
            server.destination = ('unix', 'proxy.sock')
            server.serve_forever()
    elif mode == 'ssh-bridge':
        os.chdir(sys.argv[2])
        with Server('ssh.sock', Bridge) as server:
            os.chmod('ssh.sock', 0o600)
            server.destination = ('127.0.0.1', int(sys.argv[3]))
            server.serve_forever()
    elif mode == 'verify':
        interfaces = {name for _, name in socket.if_nameindex()}
        if interfaces != {'lo'} or os.readlink('/proc/self/ns/net') == sys.argv[2]:
            raise RuntimeError('Network isolation verification failed; refusing startup')
        for address in ('1.1.1.1', '2606:4700:4700::1111'):
            family = socket.AF_INET6 if ':' in address else socket.AF_INET
            try:
                stream = socket.socket(family)
            except OSError as error:
                if family == socket.AF_INET6 and error.errno in (errno.EAFNOSUPPORT, errno.EPROTONOSUPPORT):
                    continue
                raise
            with stream:
                stream.settimeout(1)
                if stream.connect_ex((address, 443)) == 0:
                    raise RuntimeError('Direct network access is available; refusing startup')
        print('Verified private network namespace: loopback only; IPv4/IPv6 egress blocked.')
    else:
        raise ValueError('unknown mode')


if __name__ == '__main__':
    main()
