#!/bin/bash
for l in "$@"; do
  node topo.mjs $l /tmp/gs/vid/$l 2>&1 | tail -1
  ffmpeg -v error -y -ss 3 -i /tmp/gs/vid/$l/topology.webm -vf "fps=7,scale=1000:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=96[p];[b][p]paletteuse=dither=bayer:bayer_scale=4" /tmp/gs/vid/$l/t.gif
  gif2webp -q 50 -lossy -m 6 /tmp/gs/vid/$l/t.gif -o /tmp/gs/vid/$l/topology.webp >/dev/null 2>&1
  ls -la /tmp/gs/vid/$l/topology.webp
done
echo TOPODONE
