"""Universal App Store creative asset (product page header + search), 5244x2950 PNG, no alpha.
Based on render_feature_v2.py. Key content kept inside the central ~3840x1646 safe area.
Usage: python render_banner_5244.py  (run from repo root or graphics/)"""
import os, sys, io, urllib.request
from PIL import Image, ImageDraw, ImageFilter, ImageFont
W, H = 5244, 2950
SW, SH = 3840, 1646                     # header crop (21:9); search crop is 3:2 centred, wider vertically
SX, SY = (W - SW) // 2, (H - SH) // 2
R = os.path.dirname(os.path.abspath(__file__)); ROOT = os.path.dirname(R)
CFG = dict(
    title='Take-Home Pay US',
    tag='Estimate your paycheck in seconds',
    icon=os.path.join(ROOT, 'store-assets', 'icon-1024.png'),
    shots=[os.path.join(ROOT, 'ios-release', 'screenshots', 'iphone67-01-calculate.png'),
           os.path.join(ROOT, 'ios-release', 'screenshots', 'ipad129-01-calculate.png')],
    bg_urls=['https://images.rawpixel.com/editor_1024/czNmcy1wcml2YXRlL3Jhd3BpeGVsX2ltYWdlcy93ZWJzaXRlX2NvbnRlbnQvbHIvd2s1NDgwODg1Ni1pbWFnZS1rcDZjNmg0ai5qcGc.jpg'],
    bg_cache=os.path.join(R, 'bg-nyc-skyline-cc0.jpg'),
    accent=(64, 224, 170), shade=(8, 14, 24),
    out=os.path.join(R, 'ios-banner-5244x2950.png'))

def font(names, s):
    for n in names:
        for d in ('', 'C:/Windows/Fonts/'):
            try: return ImageFont.truetype(d + n, s)
            except OSError: pass
    return ImageFont.load_default()

def get_bg(c):
    if not os.path.exists(c['bg_cache']):
        for u in c['bg_urls']:
            try:
                data = urllib.request.urlopen(urllib.request.Request(u, headers={'User-Agent': 'Mozilla/5.0'}), timeout=60).read()
                open(c['bg_cache'], 'wb').write(data); break
            except Exception as e: print('bg fetch failed', u, e)
    im = Image.open(c['bg_cache']).convert('RGB')
    r = max(W / im.width, H / im.height)
    im = im.resize((int(im.width * r + 1), int(im.height * r + 1)), Image.LANCZOS)
    x, y = (im.width - W) // 2, (im.height - H) // 2
    im = im.crop((x, y, x + W, y + H)).filter(ImageFilter.GaussianBlur(6))   # soften upscaling
    # darkening: strong at left/centre-left under the text, lighter at the edges
    m = Image.linear_gradient('L').rotate(90).resize((W, H))                  # 255 left -> 0 right
    m = m.point(lambda v: int(120 + 95 * v / 255))
    return Image.composite(Image.new('RGB', (W, H), c['shade']), im, m)

def rounded(p, size, rad=0.22):
    ic = Image.open(p).convert('RGBA').resize((size, size), Image.LANCZOS)
    mk = Image.new('L', (size, size), 0); ImageDraw.Draw(mk).rounded_rectangle([0, 0, size - 1, size - 1], int(size * rad), fill=255)
    ic.putalpha(mk); return ic

def shadow(im, obj, pos, blur=60, op=160, off=24):
    a = obj.split()[-1].point(lambda v: v * op // 255)
    s = Image.new('RGBA', obj.size, (0, 0, 0, 0)); s.putalpha(a)
    pad = blur * 3; S = Image.new('RGBA', (obj.width + 2 * pad, obj.height + 2 * pad), (0, 0, 0, 0)); S.paste(s, (pad, pad))
    S = S.filter(ImageFilter.GaussianBlur(blur))
    im.alpha_composite(S, (pos[0] - pad + off // 2, pos[1] - pad + off)); im.alpha_composite(obj, pos)

def device(shot, h, ratio=0.462, bz=26, rad=96, notch=True):
    s = Image.open(shot).convert('RGB'); w = int(h * ratio) if ratio else int(h * s.width / s.height) + 2 * bz
    sw, sh = w - 2 * bz, h - 2 * bz
    s = s.resize((sw, int(s.height * sw / s.width)), Image.LANCZOS)
    S2 = Image.new('RGB', (sw, sh), s.getpixel((sw // 2, s.height - 3))); S2.paste(s.crop((0, 0, sw, min(sh, s.height))), (0, 0))
    P = Image.new('RGBA', (w, h), (0, 0, 0, 0)); d = ImageDraw.Draw(P)
    d.rounded_rectangle([0, 0, w - 1, h - 1], rad, fill=(18, 18, 22, 255), outline=(95, 95, 105, 255), width=5)
    mk = Image.new('L', (sw, sh), 0); ImageDraw.Draw(mk).rounded_rectangle([0, 0, sw - 1, sh - 1], rad - bz, fill=255)
    P.paste(S2, (bz, bz), mk)
    if notch: d.rounded_rectangle([w // 2 - 80, bz + 20, w // 2 + 80, bz + 66], 23, fill=(0, 0, 0, 255))
    return P

def render(c):
    im = get_bg(c).convert('RGBA')
    ph = int(SH * 0.94)                                    # device height inside safe area
    p1 = device(c['shots'][0], ph)
    p2 = device(c['shots'][1], int(ph * 0.86), ratio=None, bz=22, rad=60, notch=False) if 'ipad' in os.path.basename(c['shots'][1]) else device(c['shots'][1], int(ph * 0.9))
    right = SX + SW - 40
    x1 = right - p1.width; x2 = x1 - int(p2.width * 0.62)
    shadow(im, p2, (x2, SY + (SH - p2.height) // 2 + 40)); shadow(im, p1, (x1, SY + (SH - p1.height) // 2))
    isz = 520; ix = SX + 60
    d = ImageDraw.Draw(im); maxw = x2 - ix - 120
    def fit(t, names, s):
        f = font(names, s)
        while d.textlength(t, font=f) > maxw and s > 40: s -= 4; f = font(names, s)
        return f
    ft = fit(c['title'], ['seguisb.ttf', 'arialbd.ttf', 'DejaVuSans-Bold.ttf'], 230)
    fg = fit(c['tag'], ['segoeui.ttf', 'arial.ttf', 'DejaVuSans.ttf'], 120)
    tb = d.textbbox((0, 0), c['title'], font=ft); gb = d.textbbox((0, 0), c['tag'], font=fg)
    th, gh = tb[3] - tb[1], gb[3] - gb[1]
    block = isz + 90 + th + 80 + gh; y0 = SY + (SH - block) // 2
    shadow(im, rounded(c['icon'], isz), (ix, y0), blur=40, op=150)
    ty = y0 + isz + 90
    d.text((ix + 6, ty - tb[1] + 6), c['title'], font=ft, fill=(0, 0, 0, 130)); d.text((ix, ty - tb[1]), c['title'], font=ft, fill=(255, 255, 255, 255))
    d.rounded_rectangle([ix + 4, ty + th + 26, ix + 230, ty + th + 42], 8, fill=c['accent'] + (255,))
    gy = ty + th + 80
    d.text((ix + 4, gy - gb[1] + 4), c['tag'], font=fg, fill=(0, 0, 0, 140)); d.text((ix, gy - gb[1]), c['tag'], font=fg, fill=(240, 244, 248, 255))
    out = im.convert('RGB'); assert out.size == (W, H) and out.mode == 'RGB'
    out.save(c['out'], optimize=True); print(c['out'], out.size, out.mode)
    out.resize((1024, 576), Image.LANCZOS).save(os.path.join(R, 'ios-banner-preview-1024.png'))
    return out

if __name__ == '__main__': render(CFG)
