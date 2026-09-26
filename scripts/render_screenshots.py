#!/usr/bin/env python3
"""Render the HTML screen captures from TestRenderMockups into PNGs.

usage: render_screenshots.py IN_DIR OUT_DIR
Needs Pillow and DejaVu Sans Mono (Ubuntu: fonts-dejavu-core).
"""
import html.parser
import os
import re
import sys

from PIL import Image, ImageDraw, ImageFont

FONT = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FONT_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
SIZE = 26           # font pixel size; cell is derived from it
PAD = 24            # pixels of black around the screen
BG = "#000000"
FG = "#aaaaaa"


class Spans(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.rows = [[]]
        self.style = {}

    def handle_starttag(self, tag, attrs):
        if tag == "span":
            css = dict(attrs).get("style", "")
            self.style = dict(kv.split(":", 1) for kv in css.split(";") if ":" in kv)

    def handle_endtag(self, tag):
        if tag == "span":
            self.style = {}

    def handle_data(self, data):
        parts = data.split("\n")
        for i, part in enumerate(parts):
            if i > 0:
                self.rows.append([])
            if part:
                self.rows[-1].append((part, dict(self.style)))


def width(ch):
    o = ord(ch)
    if 0x1100 <= o <= 0x115F or 0x2E80 <= o <= 0xA4CF or 0xAC00 <= o <= 0xD7A3 or 0xF900 <= o <= 0xFAFF or 0xFE30 <= o <= 0xFE4F or 0xFF00 <= o <= 0xFF60 or 0xFFE0 <= o <= 0xFFE6 or 0x1F300 <= o <= 0x1FAFF:
        return 2
    return 1


def render(src, dst):
    p = Spans()
    p.feed(open(src, encoding="utf-8").read())
    rows = [r for r in p.rows]
    while rows and not rows[-1]:
        rows.pop()
    font = ImageFont.truetype(FONT, SIZE)
    bold = ImageFont.truetype(FONT_BOLD, SIZE)
    cw = font.getlength("M")
    ch = int(SIZE * 1.25)
    cols = max(sum(width(c) for text, _ in r for c in text) for r in rows)
    img = Image.new("RGB", (int(cols * cw) + 2 * PAD, len(rows) * ch + 2 * PAD), BG)
    d = ImageDraw.Draw(img)
    for y, row in enumerate(rows):
        x = 0
        for text, st in row:
            fg = st.get("color", FG)
            bg = st.get("background")
            f = bold if st.get("font-weight") == "bold" else font
            for c in text:
                w = width(c)
                x0, y0 = PAD + x * cw, PAD + y * ch
                if bg:
                    d.rectangle([x0, y0, x0 + w * cw, y0 + ch], fill=bg)
                d.text((x0, y0 + ch * 0.08), c, font=f, fill=fg)
                x += w
    img.save(dst, optimize=True)
    return img.size


def main():
    src_dir, out_dir = sys.argv[1], sys.argv[2]
    os.makedirs(out_dir, exist_ok=True)
    for name in sorted(os.listdir(src_dir)):
        if not name.endswith(".html"):
            continue
        out = os.path.join(out_dir, re.sub(r"^\d+-", "", name[:-5]) + ".png")
        size = render(os.path.join(src_dir, name), out)
        print(out, size)


if __name__ == "__main__":
    main()
