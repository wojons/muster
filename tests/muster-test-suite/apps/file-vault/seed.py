#!/usr/bin/env python3
"""
Seed script for File Vault — populates folders and files.
Run standalone: python3 seed.py
"""
import json
import sys
import urllib.request

BASE = "http://localhost:9802"


def post(path, body):
    req = urllib.request.Request(
        BASE + path,
        data=json.dumps(body).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        resp = urllib.request.urlopen(req)
        return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        print(f"  ERROR {path}: HTTP {e.code} — {body}", file=sys.stderr)
        return None
    except urllib.error.URLError as e:
        print(f"  ERROR {path}: {e.reason}", file=sys.stderr)
        return None


def main():
    print("🌱 Seeding File Vault...")

    # ── Folder 1: Documents ──
    print("  Creating folders...")
    docs = post("/folders", {"name": "Documents"})
    if not docs:
        print("ERROR: Could not create Documents folder — is file-vault running?", file=sys.stderr)
        sys.exit(1)

    # ── Folder 2: Images (inside Documents) ──
    images = post("/folders", {"name": "Images", "parent_id": docs["id"]})

    # ── File 1: README in root ──
    print("  Creating files...")
    f1 = post("/files", {
        "name": "README.md",
        "content": "# File Vault\n\nSecure file storage and sharing service.",
        "content_type": "text/markdown"
    })
    if f1:
        print(f"    Created: {f1['name']} (id={f1['id']})")

    # ── File 2: report in Documents ──
    f2 = post("/files", {
        "name": "Q1-Report.pdf",
        "content": "PDF_CONTENT_PLACEHOLDER_Q1_2026",
        "content_type": "application/pdf",
        "folder_id": docs["id"]
    })
    if f2:
        print(f"    Created: {f2['name']} (id={f2['id']})")
        # Share f2 with a colleague
        post("/shares", {"file_id": f2["id"], "shared_with": "alice@example.com", "permission": "read"})

    # ── File 3: logo in Images ──
    if images:
        f3 = post("/files", {
            "name": "logo.png",
            "content": "LOGO_BINARY_PLACEHOLDER",
            "content_type": "image/png",
            "folder_id": images["id"]
        })
        if f3:
            print(f"    Created: {f3['name']} (id={f3['id']})")

    print(f"  ✅ Seeded 2 folders and 3 files (plus 1 share).")
    print(f"     Server: {BASE}")


if __name__ == "__main__":
    main()
