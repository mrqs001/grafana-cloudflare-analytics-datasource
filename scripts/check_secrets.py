#!/usr/bin/env python3
"""Fail without printing secret contents if the known development token is staged/built."""
import base64
import os
from pathlib import Path
import subprocess
import sys

secret = os.environ.get('CF_API_TOKEN', '')
if not secret and Path('.env').exists():
    for line in Path('.env').read_text().splitlines():
        if line.startswith('CF_API_TOKEN='):
            secret = line.split('=', 1)[1].strip().strip('"').strip("'")
tracked = subprocess.check_output(['git', 'ls-files', '-z']).decode().split('\0')
bad_paths = [p for p in tracked if p and (p == '.env' or p.startswith('.local/') or (Path(p).name.startswith('.env.') and Path(p).name != '.env.example'))]
if bad_paths:
    sys.exit('ERROR: private local files are tracked; remove them from the index')
if not secret:
    print('PASS private files excluded (known-token content scan skipped: token not supplied)')
    sys.exit(0)
patterns = [secret.encode(), base64.b64encode(secret.encode())]
for name in tracked:
    if not name:
        continue
    content = subprocess.check_output(['git', 'show', ':' + name])
    if any(pattern in content for pattern in patterns):
        sys.exit('ERROR: known token detected in staged content; contents withheld')
for path in Path('dist').rglob('*'):
    if path.is_file() and any(pattern in path.read_bytes() for pattern in patterns):
        sys.exit('ERROR: known token detected in build output; contents withheld')
print('PASS private files excluded; known token absent from Git index and built assets')
