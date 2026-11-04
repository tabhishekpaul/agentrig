# Generates web/static/icon.svg: crimson squircle + stacked pyramid in three bands (python3 scripts/make-icon.py).
import math
N = 5.0          # superellipse exponent (squircle like editrig's)
def squircle(cx=32, cy=32, r=32, steps=96):
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        c, s = math.cos(t), math.sin(t)
        x = cx + r * math.copysign(abs(c) ** (2 / N), c)
        y = cy + r * math.copysign(abs(s) ** (2 / N), s)
        pts.append(f"{x:.2f},{y:.2f}")
    return "M" + " L".join(pts) + " Z"

apex, base, half = 17.5, 45.0, 16.5   # pyramid
gap = 2.6
h = base - apex
cuts = [apex + h * 0.36, apex + h * 0.68]  # band heights tuned so the top reads as a clear triangle
def hw(y): return half * (y - apex) / h
bands = [(apex, cuts[0] - gap / 2), (cuts[0] + gap / 2, cuts[1] - gap / 2), (cuts[1] + gap / 2, base)]
polys = []
for top, bot in bands:
    if top == apex:
        pts = [(32, apex), (32 + hw(bot), bot), (32 - hw(bot), bot)]
    else:
        pts = [(32 - hw(top), top), (32 + hw(top), top), (32 + hw(bot), bot), (32 - hw(bot), bot)]
    polys.append("M" + " L".join(f"{x:.2f},{y:.2f}" for x, y in pts) + " Z")

svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
  <title>agentrig</title>
  <defs>
    <linearGradient id="arig-bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#D4003A"/>
      <stop offset="1" stop-color="#B00030"/>
    </linearGradient>
  </defs>
  <path d="{squircle()}" fill="url(#arig-bg)"/>
  <path d="{' '.join(polys)}" fill="#FFFBF5" stroke="#FFFBF5" stroke-width="2.2" stroke-linejoin="round"/>
</svg>
'''
import os
root = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
open(os.path.join(root, "web/static/icon.svg"), "w").write(svg)
