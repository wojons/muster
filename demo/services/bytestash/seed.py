#!/usr/bin/env python3
"""Seed ByteStash with demo folders and files."""

import json
import sys
import os
import struct
import urllib.request
import urllib.error

BASE = f"http://localhost:{sys.argv[1]}" if len(sys.argv) > 1 else "http://localhost:9802"

def api(method, path, data=None, headers=None):
    url = f"{BASE}{path}"
    req = urllib.request.Request(url, method=method)
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    if data is not None:
        req.data = data
    try:
        with urllib.request.urlopen(req) as resp:
            if resp.headers.get("Content-Type", "").startswith("application/json"):
                return resp.status, json.loads(resp.read())
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        body = e.read()
        try:
            return e.code, json.loads(body)
        except Exception:
            return e.code, body.decode("utf-8", errors="replace")

def upload_file(folder_id, filename, content_bytes, mime_type="application/octet-stream"):
    """Upload a file to a folder via multipart/form-data."""
    boundary = "----SeedUploadBoundary"
    body = b""
    body += f"--{boundary}\r\n".encode()
    body += f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'.encode()
    body += f"Content-Type: {mime_type}\r\n".encode()
    body += b"\r\n"
    body += content_bytes
    body += f"\r\n--{boundary}--\r\n".encode()

    headers = {"Content-Type": f"multipart/form-data; boundary={boundary}"}
    return api("POST", f"/folders/{folder_id}/files", data=body, headers=headers)

# ── Helpers to generate small file content ────────────────────

def make_png():
    """Minimal 1x1 red PNG."""
    import zlib
    sig = b"\x89PNG\r\n\x1a\n"
    # IHDR: 1x1, 8-bit RGB
    ihdr_data = struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)
    ihdr_crc = zlib.crc32(b"IHDR" + ihdr_data) & 0xFFFFFFFF
    ihdr = struct.pack(">I", 13) + b"IHDR" + ihdr_data + struct.pack(">I", ihdr_crc)
    # IDAT: single red pixel (filter byte 0, then R=255,G=0,B=0)
    raw = b"\x00\xff\x00\x00"
    compressed = zlib.compress(raw)
    idat_crc = zlib.crc32(b"IDAT" + compressed) & 0xFFFFFFFF
    idat = struct.pack(">I", len(compressed)) + b"IDAT" + compressed + struct.pack(">I", idat_crc)
    # IEND
    iend_crc = zlib.crc32(b"IEND") & 0xFFFFFFFF
    iend = struct.pack(">I", 0) + b"IEND" + struct.pack(">I", iend_crc)
    return sig + ihdr + idat + iend

def make_jpg():
    """Minimal JPEG (single pixel)."""
    # SOI + APP0 + DQT + SOF0 + DHT + SOS + scan data + EOI
    # Using a pre-built minimal JPEG
    return bytes([
        0xFF, 0xD8,  # SOI
        0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00,  # JFIF
        0xFF, 0xDB, 0x00, 0x43, 0x00,  # DQT
        0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0A, 0x0C, 0x14,
        0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12, 0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A,
        0x1C, 0x1C, 0x20, 0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29, 0x2C,
        0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32, 0x3C, 0x2E, 0x33, 0x34, 0x32,
        0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11, 0x00,  # SOF0 (1x1)
        0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00, 0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B,  # DHT
        0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0x7F, 0x80,  # SOS (1 component)
        0xFF, 0xD9,  # EOI
    ])

def make_pdf():
    """Minimal valid PDF."""
    return b"""%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Resources<<>>>>endobj
xref
0 4
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
trailer<</Size 4/Root 1 0 R>>
startxref
206
%%EOF"""

def make_docx():
    """Minimal valid DOCX (ZIP with required XML)."""
    import zipfile
    import io
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("[Content_Types].xml",
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
            '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
            '<Default Extension="xml" ContentType="application/xml"/>'
            '<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>'
            '</Types>')
        zf.writestr("_rels/.rels",
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>'
            '</Relationships>')
        zf.writestr("word/document.xml",
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
            '<w:body><w:p><w:r><w:t>ByteStash Demo Document</w:t></w:r></w:p></w:body>'
            '</w:document>')
    return buf.getvalue()

def make_xlsx():
    """Minimal valid XLSX (ZIP with required XML)."""
    import zipfile
    import io
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("[Content_Types].xml",
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
            '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
            '<Default Extension="xml" ContentType="application/xml"/>'
            '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>'
            '</Types>')
        zf.writestr("_rels/.rels",
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>'
            '</Relationships>')
        zf.writestr("xl/workbook.xml",
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
            '<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets>'
            '</workbook>')
    return buf.getvalue()

def make_zip():
    """Minimal valid ZIP with a text file inside."""
    import zipfile
    import io
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("readme.txt", "ByteStash social media templates package.\n")
    return buf.getvalue()

def make_elf_so():
    """Minimal ELF shared object stub."""
    e_ident = b"\x7fELF\x02\x01\x01\x00" + b"\x00" * 8  # ELF64, LE, v1, System V
    header = e_ident
    header += struct.pack("<H", 3)  # e_type = ET_DYN (shared object)
    header += struct.pack("<H", 0x3E)  # e_machine = x86-64
    header += struct.pack("<I", 1)  # e_version
    header += struct.pack("<Q", 0)  # e_entry
    header += struct.pack("<Q", 64)  # e_phoff
    header += struct.pack("<Q", 0)  # e_shoff
    header += struct.pack("<I", 0)  # e_flags
    header += struct.pack("<H", 64)  # e_ehsize
    header += struct.pack("<H", 56)  # e_phentsize
    header += struct.pack("<H", 1)  # e_phnum
    header += struct.pack("<H", 64)  # e_shentsize
    header += struct.pack("<H", 0)  # e_shnum
    header += struct.pack("<H", 0)  # e_shstrndx
    # Single PT_LOAD program header
    phdr = struct.pack("<II", 1, 7)  # p_type=PT_LOAD, p_flags=RWX
    phdr += struct.pack("<Q", 0)  # p_offset
    phdr += struct.pack("<Q", 0)  # p_vaddr
    phdr += struct.pack("<Q", 0)  # p_paddr
    phdr += struct.pack("<Q", 0)  # p_filesz
    phdr += struct.pack("<Q", 0)  # p_memsz
    phdr += struct.pack("<Q", 0x1000)  # p_align
    return header + phdr + b"/* ByteStash lib-core stub */\n"

def make_elf_o():
    """Minimal ELF relocatable object stub."""
    e_ident = b"\x7fELF\x02\x01\x01\x00" + b"\x00" * 8
    header = e_ident
    header += struct.pack("<H", 1)  # e_type = ET_REL
    header += struct.pack("<H", 0x3E)  # e_machine = x86-64
    header += struct.pack("<I", 1)
    header += struct.pack("<Q", 0) * 3  # entry, phoff, shoff
    header += struct.pack("<I", 0)  # e_flags
    header += struct.pack("<H", 64)  # e_ehsize
    header += struct.pack("<H", 0)  # e_phentsize
    header += struct.pack("<H", 0)  # e_phnum
    header += struct.pack("<H", 64)  # e_shentsize
    header += struct.pack("<H", 1)  # e_shnum
    header += struct.pack("<H", 0)  # e_shstrndx
    return header + b"/* ByteStash auth-module stub */\n"

def make_binary():
    """Random-looking binary blob with a marker."""
    return b"BYTESTASH_SERVICE_BINARY\x00" + os.urandom(128)

def make_sh_script():
    """Deploy script."""
    return b"""#!/bin/bash
# ByteStash Deploy Script v1.0
set -euo pipefail

APP_NAME="bytestash"
DEPLOY_DIR="/opt/${APP_NAME}"
BACKUP_DIR="/var/backups/${APP_NAME}"

echo "=== ByteStash Deploy ==="
echo "Target: ${DEPLOY_DIR}"

# Backup current version
if [ -d "${DEPLOY_DIR}" ]; then
    tar -czf "${BACKUP_DIR}/backup-$(date +%Y%m%d-%H%M%S).tar.gz" -C "${DEPLOY_DIR}" .
    echo "Backup created."
fi

# Deploy new artifacts
mkdir -p "${DEPLOY_DIR}"
cp -r ./dist/* "${DEPLOY_DIR}/"
echo "Deploy complete."
""".replace(b"\n", b"\r\n")  # Use CRLF for shell script just for variety

# ── Seed Data ─────────────────────────────────────────────────

print("🌱 Seeding ByteStash demo data...\n")

# Create folders
status, marketing = api("POST", "/folders", json.dumps({"name": "Marketing Assets"}).encode(),
                        headers={"Content-Type": "application/json"})
print(f"✅ Folder: {marketing['name']} ({marketing['id']})")

status, engineering = api("POST", "/folders", json.dumps({"name": "Engineering"}).encode(),
                          headers={"Content-Type": "application/json"})
print(f"✅ Folder: {engineering['name']} ({engineering['id']})")

status, hr = api("POST", "/folders", json.dumps({"name": "HR"}).encode(),
                 headers={"Content-Type": "application/json"})
print(f"✅ Folder: {hr['name']} ({hr['id']})")

# ── Marketing Assets (8 files) ────────────────────────────────

marketing_files = [
    ("brand-guide.pdf", make_pdf(), "application/pdf"),
    ("logo-primary.png", make_png(), "image/png"),
    ("logo-dark.png", make_png(), "image/png"),
    ("hero-banner.jpg", make_jpg(), "image/jpeg"),
    ("social-templates.zip", make_zip(), "application/zip"),
    ("ad-copy-v3.docx", make_docx(),
     "application/vnd.openxmlformats-officedocument.wordprocessingml.document"),
    ("q2-campaign-plan.xlsx", make_xlsx(),
     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"),
    ("style-guide.md",
     b"# Acme Brand Style Guide\n\n## Colors\n- Primary: #1A73E8 (Blue)\n- Secondary: #34A853 (Green)\n- Accent: #EA4335 (Red)\n\n## Typography\n- Headings: Inter, sans-serif\n- Body: Source Sans Pro, 16px\n- Code: JetBrains Mono\n\n## Logo Usage\n- Minimum clear space: 24px\n- Never stretch or skew\n- Use primary logo on light backgrounds, dark logo on dark backgrounds\n\n## Voice & Tone\n- Professional but approachable\n- Active voice, present tense\n- Avoid jargon -- write for a general audience\n",
     "text/markdown"),
]

for name, content, mime in marketing_files:
    status, f = upload_file(marketing["id"], name, content, mime)
    print(f"  📄 {f['name']} ({f['size']} bytes, {f['mime_type']})")

# ── Engineering (5 binaries) ──────────────────────────────────

engineering_files = [
    ("lib-core-v2.1.so", make_elf_so(), "application/x-sharedlib"),
    ("auth-module.o", make_elf_o(), "application/x-object"),
    ("deploy-script.sh", make_sh_script(), "text/x-shellscript"),
    ("service-binary", make_binary(), "application/octet-stream"),
    ("config.yaml",
     b"# ByteStash Service Configuration\nserver:\n  port: 9802\n  host: 0.0.0.0\n  workers: 4\n\nstorage:\n  backend: local\n  path: /data/bytestash\n  max_file_size: 1073741824  # 1 GB\n\ncache:\n  enabled: true\n  ttl_seconds: 3600\n\nlogging:\n  level: info\n  format: json\n  output: stdout\n",
     "text/yaml"),
]

for name, content, mime in engineering_files:
    status, f = upload_file(engineering["id"], name, content, mime)
    print(f"  📄 {f['name']} ({f['size']} bytes, {f['mime_type']})")

# ── HR (3 docs) ───────────────────────────────────────────────

hr_files = [
    ("employee-handbook.pdf", make_pdf(), "application/pdf"),
    ("onboarding-guide.docx", make_docx(),
     "application/vnd.openxmlformats-officedocument.wordprocessingml.document"),
    ("benefits-summary-2026.pdf", make_pdf(), "application/pdf"),
]

for name, content, mime in hr_files:
    status, f = upload_file(hr["id"], name, content, mime)
    print(f"  📄 {f['name']} ({f['size']} bytes, {f['mime_type']})")

# ── Verify ────────────────────────────────────────────────────

status, all_folders = api("GET", "/folders")
total_files = 0
mid = marketing["id"]
eid = engineering["id"]
hid = hr["id"]
for fid in [mid, eid, hid]:
    status, flist = api("GET", f"/folders/{fid}/files")
    total_files += len(flist)
status, health = api("GET", "/health")

print(f"\n🎯 Seeded: {len(all_folders)} folders, {total_files} files")
print(f"   Health: {json.dumps(health)}")
