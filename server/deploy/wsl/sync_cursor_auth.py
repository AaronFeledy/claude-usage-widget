#!/usr/bin/env python3
"""Seed the server's memory-only Cursor credential from an existing WSL CLI login.

Run through the accompanying service so the API token is supplied privately in
USAGE_AUTH_TOKEN. Provider credentials and API responses are never logged.
"""
import json
import os
from pathlib import Path
import urllib.error
import urllib.request


def sync():
    address = os.environ['USAGE_API_URL'].rstrip('/')
    headers = {'Authorization': 'Bearer ' + os.environ['USAGE_AUTH_TOKEN']}
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    request = urllib.request.Request(address + '/api/v1/usage/cursor', headers=headers)
    with opener.open(request, timeout=10) as response:
        current = json.load(response)
    if current.get('is_success'):
        return
    credential = json.loads(Path(os.environ['CURSOR_AUTH_PATH']).read_text())
    access_token = credential.get('accessToken')
    if not access_token:
        raise ValueError('No Cursor access token available')
    headers['Content-Type'] = 'application/json'
    request = urllib.request.Request(address + '/api/v1/providers/cursor/credentials',
        data=json.dumps({'access_token': access_token}).encode(), headers=headers, method='PUT')
    with opener.open(request, timeout=30) as response:
        result = json.load(response)
    if not result.get('usage', {}).get('is_success'):
        raise ValueError('Cursor authentication needs attention')


if __name__ == '__main__':
    try:
        sync()
    except (OSError, ValueError, KeyError, urllib.error.URLError):
        # Only a generic diagnostic: auth headers and provider payloads stay private.
        raise SystemExit('Cursor credential sync unavailable; other providers continue normally.')
