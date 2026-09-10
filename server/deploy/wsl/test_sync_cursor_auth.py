import contextlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import unittest
import urllib.error
from unittest import mock

MODULE_PATH = Path(__file__).with_name('sync_cursor_auth.py')
SPEC = importlib.util.spec_from_file_location('sync_cursor_auth', MODULE_PATH)
sync_cursor_auth = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(sync_cursor_auth)


class RecordingHandler(BaseHTTPRequestHandler):
    requests = []
    responder = None

    def do_GET(self):
        self.handle_request()

    def do_PUT(self):
        self.handle_request()

    def handle_request(self):
        length = int(self.headers.get('Content-Length', '0'))
        body = self.rfile.read(length)
        type(self).requests.append((self.command, self.path, dict(self.headers), body))
        type(self).responder(self)

    def log_message(self, format_string, *args):
        pass


@contextlib.contextmanager
def server(responder):
    handler = type('IsolatedHandler', (RecordingHandler,), {'requests': [], 'responder': staticmethod(responder)})
    instance = ThreadingHTTPServer(('127.0.0.1', 0), handler)
    thread = threading.Thread(target=instance.serve_forever, daemon=True)
    thread.start()
    try:
        yield instance, handler
    finally:
        instance.shutdown()
        instance.server_close()
        thread.join()


def json_response(handler, value, status=200, extra_headers=()):
    body = json.dumps(value).encode()
    handler.send_response(status)
    handler.send_header('Content-Type', 'application/json')
    handler.send_header('Content-Length', str(len(body)))
    for name, value in extra_headers:
        handler.send_header(name, value)
    handler.end_headers()
    handler.wfile.write(body)


class SyncCursorAuthSafetyTests(unittest.TestCase):
    def environment(self, url, credential_path):
        return mock.patch.dict(os.environ, {
            'USAGE_API_URL': url,
            'USAGE_AUTH_TOKEN': 'synthetic-bearer',
            'CURSOR_AUTH_PATH': credential_path,
        }, clear=True)

    def test_get_redirect_sends_nothing_to_target_and_does_not_read_credential(self):
        with server(lambda h: json_response(h, {'unexpected': True})) as (target, target_handler):
            target_url = f'http://127.0.0.1:{target.server_port}/target'
            with server(lambda h: json_response(h, {}, 302, [('Location', target_url)])) as (origin, _):
                with self.environment(f'http://127.0.0.1:{origin.server_port}', '/missing/must-not-be-read'):
                    with self.assertRaises(urllib.error.HTTPError) as rejected:
                        sync_cursor_auth.sync()
                    self.assertEqual(302, rejected.exception.code)
                    rejected.exception.close()
            self.assertEqual([], target_handler.requests)

    def test_put_redirect_sends_neither_bearer_nor_provider_credential_to_target(self):
        with tempfile.TemporaryDirectory() as temporary:
            credential = Path(temporary, 'auth.json')
            credential.write_text(json.dumps({'accessToken': 'synthetic-provider-token'}))
            with server(lambda h: json_response(h, {'unexpected': True})) as (target, target_handler):
                target_url = f'http://127.0.0.1:{target.server_port}/target'

                def origin_response(handler):
                    if handler.command == 'GET':
                        json_response(handler, {'is_success': False})
                    else:
                        json_response(handler, {}, 307, [('Location', target_url)])

                with server(origin_response) as (origin, origin_handler):
                    with self.environment(f'http://127.0.0.1:{origin.server_port}', str(credential)):
                        with self.assertRaises(urllib.error.HTTPError) as rejected:
                            sync_cursor_auth.sync()
                        self.assertEqual(307, rejected.exception.code)
                        rejected.exception.close()
                    self.assertEqual(2, len(origin_handler.requests))
                    self.assertIn(b'synthetic-provider-token', origin_handler.requests[1][3])
                self.assertEqual([], target_handler.requests)

    def test_invalid_url_and_headers_fail_before_credential_access(self):
        cases = [
            ('http://user:secret@example.test', 'token'),
            ('http://example.test/path?secret=yes', 'token'),
            ('http://example.test/path#fragment', 'token'),
            ('http://example.test', 'bad\nheader'),
            ('http://example.test\n', 'token'),
            ('http://@example.test', 'token'),
            ('http://example.test:invalid', 'token'),
            ('http://example.test', 'token\r'),
        ]
        for url, token in cases:
            with self.subTest(url=url, token=token):
                with mock.patch.dict(os.environ, {
                    'USAGE_API_URL': url, 'USAGE_AUTH_TOKEN': token,
                    'CURSOR_AUTH_PATH': '/missing/must-not-be-read'}, clear=True):
                    with mock.patch.object(Path, 'read_text', side_effect=AssertionError('credential file read')):
                        with mock.patch.object(sync_cursor_auth.urllib.request, 'build_opener', side_effect=AssertionError('network used')):
                            with self.assertRaises((ValueError, KeyError)):
                                sync_cursor_auth.sync()


if __name__ == '__main__':
    unittest.main()
