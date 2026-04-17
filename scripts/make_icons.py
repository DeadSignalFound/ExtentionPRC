from PIL import Image, ImageDraw, ImageFont
import os

out = os.path.join(os.path.dirname(__file__), "..", "extension", "icons")
os.makedirs(out, exist_ok=True)

def draw(size):
    im = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    pad = max(1, size // 16)
    d.ellipse((pad, pad, size - pad, size - pad), fill=(88, 101, 242, 255))
    try:
        f = ImageFont.truetype("seguibl.ttf", int(size * 0.55))
    except Exception:
        try:
            f = ImageFont.truetype("arialbd.ttf", int(size * 0.55))
        except Exception:
            f = ImageFont.load_default()
    t = "D"
    bbox = d.textbbox((0, 0), t, font=f)
    w = bbox[2] - bbox[0]
    h = bbox[3] - bbox[1]
    d.text(((size - w) / 2 - bbox[0], (size - h) / 2 - bbox[1]), t, fill=(255, 255, 255, 255), font=f)
    im.save(os.path.join(out, f"icon-{size}.png"))

for s in (16, 32, 48, 128):
    draw(s)

print("done", out)
