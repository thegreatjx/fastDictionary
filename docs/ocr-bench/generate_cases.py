#!/usr/bin/env python3
"""Generate a small, deterministic English/Japanese screenshot OCR set.

The PNGs are written to --out and a portable manifest is written to --manifest.
Only the script and CASES.md are versioned; generated screenshots stay local.
"""

import argparse
import json
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageFont

LATIN_FONTS = [
    "C:/Windows/Fonts/arial.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
]
CJK_FONTS = [
    "C:/Windows/Fonts/YuGothM.ttc",
    "C:/Windows/Fonts/meiryo.ttc",
    "C:/Windows/Fonts/msgothic.ttc",
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
]


def pick_font(size, cjk=False):
    candidates = CJK_FONTS if cjk else LATIN_FONTS
    for path in candidates:
        if Path(path).exists():
            try:
                return ImageFont.truetype(path, size), False
            except OSError:
                pass
    return ImageFont.load_default(), True


def tight_text(text, size, fg, bg, font=None, vertical=False, shadow=False):
    f = font or pick_font(size, cjk=any(ord(ch) > 127 for ch in text))[0]
    canvas = Image.new("RGB", (1400, 320), bg)
    draw = ImageDraw.Draw(canvas)
    if shadow:
        draw.text((24, 26), text, font=f, fill=(70, 70, 70))
    if vertical:
        y = 20
        for ch in text:
            draw.text((20, y), ch, font=f, fill=fg)
            box = draw.textbbox((20, y), ch, font=f)
            y = max(y + size + 5, box[3] + 5)
    else:
        draw.text((20, 20), text, font=f, fill=fg)
    diff = ImageChops.difference(canvas, Image.new("RGB", canvas.size, bg))
    box = diff.getbbox() or (20, 20, 120, 80)
    return canvas.crop(
        (
            max(0, box[0] - 3),
            max(0, box[1] - 3),
            min(canvas.width, box[2] + 3),
            min(canvas.height, box[3] + 3),
        )
    )


def two_color_banner(text, size, font):
    canvas = Image.new("RGB", (760, 120), (80, 40, 140))
    draw = ImageDraw.Draw(canvas)
    draw.rectangle([380, 0, 760, 120], fill=(40, 190, 220))
    draw.text((55, 38), text, font=font, fill=(240, 240, 240))
    return canvas


def gradient_non_text():
    canvas = Image.new("RGB", (320, 160))
    px = canvas.load()
    for y in range(canvas.height):
        for x in range(canvas.width):
            px[x, y] = (40 + x // 8, 80 + y // 6, 160 + (x + y) // 12)
    draw = ImageDraw.Draw(canvas)
    draw.ellipse((130, 40, 190, 100), fill=(230, 230, 230))
    return canvas


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default="cases")
    parser.add_argument("--manifest", default="")
    args = parser.parse_args()

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    manifest_path = Path(args.manifest) if args.manifest else out / "cases.json"

    latin34, latin_fallback = pick_font(34)
    latin18, _ = pick_font(18)
    latin12, _ = pick_font(12)
    latin26, _ = pick_font(26)
    ja18, ja_fallback = pick_font(18, cjk=True)
    ja12, _ = pick_font(12, cjk=True)
    ja20, _ = pick_font(20, cjk=True)

    cases = []

    def add(name, category, lang, expected, image, font_fallback=False):
        path = out / f"{name}.png"
        image.save(path)
        cases.append(
            {
                "name": name,
                "path": path.name,
                "category": category,
                "lang": lang,
                "expected": expected,
                "font_fallback": font_fallback,
            }
        )

    add(
        "dark_banner_fast",
        "dark_banner_ui",
        "en",
        "Fast and efficient",
        tight_text("Fast and efficient", 34, (220, 220, 220), (31, 31, 31), latin34),
        latin_fallback,
    )
    add(
        "dark_ui_settings",
        "dark_banner_ui",
        "en",
        "Settings and translation preferences",
        tight_text("Settings and translation preferences", 18, (230, 230, 230), (35, 35, 35), latin18),
        latin_fallback,
    )
    add(
        "hover_word_tiny",
        "hover_word_tiny",
        "en",
        "Fast and efficient",
        tight_text("Fast and efficient", 12, (20, 20, 20), (255, 255, 255), latin12),
        latin_fallback,
    )
    add(
        "artistic_shadow",
        "artistic_and_styled",
        "en",
        "January 6-29",
        tight_text("January 6-29", 34, (245, 245, 245), (90, 40, 120), latin34, shadow=True),
        latin_fallback,
    )
    add(
        "low_contrast_gray",
        "low_contrast",
        "en",
        "Fast and efficient",
        tight_text("Fast and efficient", 26, (140, 140, 140), (200, 200, 200), latin26),
        latin_fallback,
    )
    add(
        "equal_luma_color",
        "low_contrast",
        "en",
        "Fast and efficient",
        tight_text("Fast and efficient", 26, (80, 150, 105), (180, 100, 100), latin26),
        latin_fallback,
    )
    add(
        "two_color_banner",
        "two_color_bg",
        "en",
        "Fast and efficient",
        two_color_banner("Fast and efficient", 34, latin34),
        latin_fallback,
    )
    add(
        "ja_ui_light",
        "multilingual_mixed",
        "ja",
        "設定を開いて翻訳言語を選択します。",
        tight_text("設定を開いて翻訳言語を選択します。", 18, (20, 20, 20), (255, 255, 255), ja18),
        ja_fallback,
    )
    add(
        "ja_ui_dark",
        "dark_banner_ui",
        "ja",
        "翻訳結果をコピーしました。",
        tight_text("翻訳結果をコピーしました。", 18, (230, 230, 230), (31, 31, 31), ja18),
        ja_fallback,
    )
    add(
        "ja_small",
        "hover_word_tiny",
        "ja",
        "これは日本語のテストです。",
        tight_text("これは日本語のテストです。", 12, (20, 20, 20), (255, 255, 255), ja12),
        ja_fallback,
    )
    add(
        "ja_mixed",
        "multilingual_mixed",
        "ja",
        "Translate 日本語 text into English",
        tight_text("Translate 日本語 text into English", 20, (20, 20, 20), (255, 255, 255), ja20),
        ja_fallback,
    )
    add(
        "ja_vertical",
        "vertical_ja",
        "ja",
        "縦書きの日本語を認識します",
        tight_text("縦書きの日本語を認識します", 20, (20, 20, 20), (255, 255, 255), ja20, vertical=True),
        ja_fallback,
    )
    add("negative_non_text", "negative_non_text", "none", "", gradient_non_text())

    manifest_path.write_text(json.dumps(cases, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"wrote {len(cases)} cases to {out}")
    print(f"manifest: {manifest_path}")
    if ja_fallback:
        print("warning: no CJK font found; Japanese cases use Pillow's default font")


if __name__ == "__main__":
    main()
