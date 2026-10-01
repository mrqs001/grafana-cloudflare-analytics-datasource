#!/usr/bin/env python3
"""Package dist under the plugin ID; never traverse the workspace or follow links."""
import hashlib
import json
from pathlib import Path
import zipfile

root = Path('dist')
metadata = json.loads((root / 'plugin.json').read_text())
plugin_id, version = metadata['id'], metadata['info']['version']
if not (root / 'module.js').is_file() or not list(root.glob('gpx_*')):
    raise SystemExit('Build frontend and backend before packaging')
output = Path('artifacts')
output.mkdir(exist_ok=True)
archive = output / f'{plugin_id}-{version}.zip'
with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as bundle:
    for file in sorted(root.rglob('*')):
        if file.is_symlink():
            raise SystemExit('Refusing a symlink in dist')
        if file.is_file():
            bundle.write(file, str(Path(plugin_id) / file.relative_to(root)))
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
archive.with_suffix('.zip.sha256').write_text(f'{digest}  {archive.name}\n')
print(archive)
