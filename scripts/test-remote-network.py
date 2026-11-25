#!/usr/bin/env python3
"""Exercise the real gateway over UNIX sockets, without internet or root."""
import http.server
import importlib.util
import json
import os
import pathlib
import socket
import socketserver
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
ASSET = pathlib.Path(__file__).resolve().parents[1]/'internal/startui/remote_assets/network.py'
spec = importlib.util.spec_from_file_location('network', ASSET)
n = importlib.util.module_from_spec(spec)
spec.loader.exec_module(n)


class HTTP(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.0'
    def do_GET(self):
        body = (self.path + '|' + str(self.headers.get('Proxy-Authorization'))).encode()
        self.send_response(200)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def do_POST(self):
        body = self.rfile.read(int(self.headers['Content-Length']))
        self.send_response(200)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args):
        pass


class NetworkTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temporary.name)
        self.web = http.server.ThreadingHTTPServer(('127.0.0.1', 0), HTTP)
        threading.Thread(target=self.web.serve_forever, daemon=True).start()
        self.port = self.web.server_address[1]
        # Long server paths work because AF_UNIX bind/connect use relative names.
        self.directory = self.root/('a'*90)/('b'*90)
        self.directory.mkdir(parents=True)
        profile = self.root/'profile.json'
        profile.write_text(json.dumps({'network': 'restricted', 'allowPrivateNetwork': True, 'sites': f'127.0.0.1:{self.port}'}))
        self.process = subprocess.Popen([sys.executable, str(ASSET), 'gateway', str(profile), str(self.directory)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        for _ in range(100):
            if (self.directory/'proxy.sock').exists():
                break
            if self.process.poll() is not None:
                self.fail(self.process.communicate())
            time.sleep(.01)
        else:
            self.fail('gateway not ready')

    def tearDown(self):
        self.process.terminate()
        self.process.communicate(timeout=5)
        self.web.shutdown()
        self.web.server_close()
        self.temporary.cleanup()

    def connect(self):
        stream = socket.socket(socket.AF_UNIX)
        # Kernel UNIX path limit; chdir is safe in this single test driver.
        before = os.getcwd()
        try:
            os.chdir(self.directory)
            stream.connect('proxy.sock')
        finally:
            os.chdir(before)
        stream.settimeout(3)
        return stream

    def request(self, message):
        with self.connect() as stream:
            stream.sendall(message.encode())
            stream.shutdown(socket.SHUT_WR)
            data = bytearray()
            while True:
                try:
                    block = stream.recv(65536)
                except ConnectionResetError:
                    # Linux can reset a closed socket with unread pipelined input.
                    # Preserve the response so callers still verify rejection
                    # and that the second request was never forwarded.
                    return bytes(data)
                if not block:
                    return bytes(data)
                data.extend(block)

    def test_private_destinations_and_pinned_dns(self):
        for address in ('127.0.0.1', '10.0.0.1', '169.254.169.254', '::1', 'fc00::1', '224.0.0.1'):
            entries = [(socket.AF_INET, socket.SOCK_STREAM, 6, '', (address, 443))]
            with patch.object(n.socket, 'getaddrinfo', return_value=entries), patch.object(n.socket, 'socket') as create:
                with self.assertRaises(ValueError): n.connect_destination('allowed.example', 443)
                create.assert_not_called()
        entries = [(socket.AF_INET, socket.SOCK_STREAM, 6, '', ('8.8.8.8', 443))]
        with patch.object(n.socket, 'getaddrinfo', return_value=entries) as resolve, patch.object(n.socket, 'socket') as create:
            n.connect_destination('allowed.example', 443)
            resolve.assert_called_once()
            create.return_value.connect.assert_called_once_with(('8.8.8.8', 443))
        with n.connect_destination('127.0.0.1', self.port, allow_private=True) as stream:
            self.assertIsNotNone(stream)

    def test_matching(self):
        rules = ['example.org', '*.sub.example.org', '**.wide.example.org', 'models.example.org:8443']
        for host, port in [('example.org',80), ('example.org',443), ('a.sub.example.org',443), ('a.b.sub.example.org',443), ('wide.example.org',443), ('a.wide.example.org',443), ('models.example.org',8443)]:
            self.assertTrue(n.allowed(host,port,rules),(host,port))
        for host, port in [('evil-example.org',443), ('example.org.evil.org',443), ('sub.example.org',443), ('models.example.org',443), ('example.org',22), ('1.1.1.1',443), ('::1',443)]:
            self.assertFalse(n.allowed(host,port,rules),(host,port))

    def test_denied_destinations(self):
        for target in ['example.org:443', '127.0.0.1:1', '[::1]:443']:
            with self.subTest(target=target):
                response=self.request(f'CONNECT {target} HTTP/1.1\r\nHost: {target}\r\n\r\n')
                self.assertTrue(response.startswith(b'HTTP/1.1 403'),response)

    def test_http_and_post(self):
        host=f'127.0.0.1:{self.port}'
        response=self.request(f'GET http://{host}/visible?ok=1 HTTP/1.1\r\nHost: {host}\r\nProxy-Authorization: secret-test-only\r\n\r\n')
        self.assertIn(b'/visible?ok=1|None',response)
        response=self.request(f'POST http://{host}/ HTTP/1.1\r\nHost: {host}\r\nContent-Length: 5\r\n\r\nhello')
        self.assertTrue(response.endswith(b'hello'),response)

    def test_no_smuggling(self):
        host=f'127.0.0.1:{self.port}'
        for extra in ['Host: evil.org\r\n', f'Host: {host}\r\nHost: {host}\r\n', 'Transfer-Encoding: chunked\r\n', 'Upgrade: websocket\r\n', 'Content-Length: -1\r\n', 'Content-Length: 99999999999\r\n', 'Connection: content-length\r\nContent-Length: 0\r\n']:
            with self.subTest(extra=extra):
                response=self.request(f'GET http://{host}/ HTTP/1.1\r\n{extra}\r\n')
                self.assertTrue(response.startswith(b'HTTP/1.1 502'),response)
        response=self.request(f'GET http://{host}/one HTTP/1.1\r\nHost: {host}\r\n\r\nGET http://{host}/two HTTP/1.1\r\n\r\n')
        self.assertIn(b'/one|None',response)
        self.assertNotIn(b'/two',response)

    def test_connect_binary_half_close(self):
        # The CONNECT tunnel transports bytes without interpreting TLS/content.
        class Echo(socketserver.BaseRequestHandler):
            def handle(self):
                while True:
                    data=self.request.recv(65536)
                    if not data:break
                    self.request.sendall(data)
        with socketserver.ThreadingTCPServer(('127.0.0.1',0),Echo) as echo:
            threading.Thread(target=echo.serve_forever,daemon=True).start()
            # Policy is read once; rewriting its on-disk metadata cannot grant access.
            (self.root/'profile.json').write_text(json.dumps({'network':'restricted','sites':f'127.0.0.1:{echo.server_address[1]}'}))
            response=self.request(f'CONNECT 127.0.0.1:{echo.server_address[1]} HTTP/1.1\r\n\r\n')
            self.assertTrue(response.startswith(b'HTTP/1.1 403'),response)
            echo.shutdown()
        with self.connect() as stream:
            stream.sendall(f'CONNECT 127.0.0.1:{self.port} HTTP/1.1\r\n\r\n'.encode())
            data=bytearray()
            while not data.endswith(b'\r\n\r\n'):data.extend(stream.recv(1))
            self.assertTrue(data.startswith(b'HTTP/1.1 200'),data)
            stream.sendall(b'GET /tunnel HTTP/1.0\r\n\r\n')
            stream.shutdown(socket.SHUT_WR)
            reply=bytearray()
            while True:
                data=stream.recv(65536)
                if not data:break
                reply.extend(data)
            self.assertIn(b'/tunnel|None',reply)

    def test_malformed_authorities(self):
        for value in ['evil.org/path', 'user@evil.org', 'evil.org:0', 'evil.org:65536', 'evil.org\t', 'evil.org%00', 'evil.org#fragment', 'a\\b']:
            with self.assertRaises(ValueError,msg=value):n.authority(value)


if __name__ == '__main__':
    unittest.main()
