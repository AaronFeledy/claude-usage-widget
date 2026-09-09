#!/usr/bin/env python3
"""Seed the server's memory-only Cursor credential from an existing WSL CLI login.

Run through the accompanying service so the API token is supplied privately in
USAGE_AUTH_TOKEN. Provider credentials and API responses are never logged.
"""
import json
import os
from pathlib import Path
import urllib.error
import urllib.parse
import urllib.request

MAX_RESPONSE_BYTES = 1024 * 1024


class RejectRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, file_pointer, code, message, headers, new_url):
        return None


def validated_settings():
    raw_address = os.environ['USAGE_API_URL']
    if not raw_address or raw_address != raw_address.strip() or any(ord(char) < 32 or ord(char) == 127 for char in raw_address):
        raise ValueError('Invalid usage API URL')
    parsed = urllib.parse.urlsplit(raw_address)
    try:
        parsed.port
    except ValueError as error:
        raise ValueError('Invalid usage API URL') from error
    if (parsed.scheme not in ('http', 'https') or not parsed.hostname
            or parsed.username is not None or parsed.password is not None or '@' in parsed.netloc
            or parsed.query or parsed.fragment):
        raise ValueError('Invalid usage API URL')
    token = os.environ['USAGE_AUTH_TOKEN']
    if not token or token != token.strip() or any(ord(char) < 32 or ord(char) == 127 for char in token):
        raise ValueError('Invalid usage API token')
    return raw_address.rstrip('/'), {'Authorization': 'Bearer ' + token}


def read_json(opener, request, timeout):
    with opener.open(request, timeout=timeout) as response:
        body = response.read(MAX_RESPONSE_BYTES + 1)
    if len(body) > MAX_RESPONSE_BYTES:
        raise ValueError('Usage API response is too large')
    value = json.loads(body)
    if not isinstance(value, dict):
        raise ValueError('Unexpected usage API response')
    return value


def sync():
    address, headers = validated_settings()
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), RejectRedirects())
    request = urllib.request.Request(address + '/api/v1/usage/cursor', headers=headers)
    current = read_json(opener, request, 10)
    if current.get('is_success'):
        return
    credential = json.loads(Path(os.environ['CURSOR_AUTH_PATH']).read_text())
    access_token = credential.get('accessToken')
    if not access_token:
        raise ValueError('No Cursor access token available')
    headers['Content-Type'] = 'application/json'
    request = urllib.request.Request(address + '/api/v1/providers/cursor/credentials',
        data=json.dumps({'access_token': access_token}).encode(), headers=headers, method='PUT')
    result = read_json(opener, request, 30)
    if not result.get('usage', {}).get('is_success'):
        raise ValueError('Cursor authentication needs attention')


if __name__ == '__main__':
    try:
        sync()
    except (OSError, ValueError, KeyError, urllib.error.URLError):
        # Only a generic diagnostic: auth headers and provider payloads stay private.
        raise SystemExit('Cursor credential sync unavailable; other providers continue normally.')
