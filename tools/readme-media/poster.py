import sys, subprocess
from PIL import Image, ImageDraw, ImageFilter
lang, vid, t = sys.argv[1], sys.argv[2], sys.argv[3]
src = vid if vid.endswith('.png') else None
if not src:
    subprocess.run(['ffmpeg','-v','error','-y','-ss',t,'-i',vid,'-frames:v','1',f'/tmp/gs/poster_{lang}.png'],check=True); src=f'/tmp/gs/poster_{lang}.png'
im = Image.open(src).convert('RGB')
im = im.resize((1600, 1000), Image.LANCZOS)
ov = Image.new('RGBA', im.size, (0,0,0,0))
d = ImageDraw.Draw(ov)
d.rectangle([0,0,im.width,im.height], fill=(8,30,26,40))
cx, cy, r = im.width//2, im.height//2, 96
d.ellipse([cx-r,cy-r,cx+r,cy+r], fill=(14,124,102,235), outline=(255,255,255,255), width=6)
d.polygon([(cx-26,cy-44),(cx-26,cy+44),(cx+52,cy)], fill=(255,255,255,255))
out = Image.alpha_composite(im.convert('RGBA'), ov).convert('RGB')
out.save(f'/tmp/gs/poster_{lang}.webp', quality=82)
print(f'/tmp/gs/poster_{lang}.webp')
