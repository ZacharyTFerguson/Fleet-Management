#!/usr/bin/env python3
"""Write simple FLEET desk PNG icons (stdlib only)."""
import struct
import zlib
from pathlib import Path

OUT = Path(__file__).resolve().parent.parent / "public"


def png(w: int, h: int, rgba: bytes) -> bytes:
    def chunk(tag: bytes, data: bytes) -> bytes:
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    raw = b""
    stride = w * 4
    for y in range(h):
        raw += b"\x00" + rgba[y * stride : (y + 1) * stride]
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")


def paint(size: int) -> bytes:
    ink = (11, 18, 24, 255)
    steel = (142, 182, 201, 255)
    sodium = (240, 192, 64, 255)
    px = [ink] * (size * size)
    m = size // 16

    def box(x0, y0, x1, y1, c):
        for y in range(max(0, y0), min(size, y1)):
            for x in range(max(0, x0), min(size, x1)):
                px[y * size + x] = c

    box(m, m, size - m, size - m, steel)
    box(m * 2, m * 2, size - m * 2, size - m * 2, ink)
    # three fill blocks
    box(m * 4, m * 5, size - m * 4, m * 8, sodium)
    box(m * 4, m * 9, size - m * 4, m * 12, steel)
    return b"".join(struct.pack("BBBB", *c) for c in px)


def main() -> None:
    OUT.mkdir(exist_ok=True)
    (OUT / "icon-192.png").write_bytes(png(192, 192, paint(192)))
    (OUT / "icon-512.png").write_bytes(png(512, 512, paint(512)))
    (OUT / "apple-touch-icon.png").write_bytes(png(180, 180, paint(180)))


if __name__ == "__main__":
    main()
