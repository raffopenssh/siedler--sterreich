"""Wheel-zoom responsiveness drill under CPU throttling (raw CDP).
usage: python3 tools/perf_wheel.py <cdp-port> [rate=4] [lon lat zoom] [perf=auto|low|high]
Dispatches *real* mouse wheel notches (Input.dispatchMouseEvent, like a Windows mouse:
deltaY ±100) and reports frame gaps, long tasks, zoom overshoot and reversal latency."""
import sys, json, time
sys.path.insert(0, 'tools'); from cdp import CDP
port = int(sys.argv[1]); rate = float(sys.argv[2]) if len(sys.argv) > 2 else 4
lon, lat, zoom = (float(sys.argv[3]), float(sys.argv[4]), float(sys.argv[5])) if len(sys.argv) > 5 else (16.3725, 48.2082, 15.5)
perf = sys.argv[6] if len(sys.argv) > 6 else 'auto'
c = CDP(port)
c.call('Emulation.setCPUThrottlingRate', rate=rate)
c.js(f"(async()=>{{ G.cam.lon={lon}; G.cam.lat={lat}; G.cam.zoom={zoom}; render(); loadMoreParcels(); await new Promise(r=>setTimeout(r,8000)); {'DEV.perf(null)' if perf=='auto' else 'DEV.perf(%r)'%perf}; }})()")
c.js("""(()=>{ window.__F=[]; window.__LT=[]; let last=performance.now(); window.__stop=false;
 (function tick(){ const n=performance.now(); __F.push(n-last); last=n; if(!__stop) requestAnimationFrame(tick); })();
 window.__po=new PerformanceObserver(l=>{for(const e of l.getEntries()) __LT.push(e.duration|0)}); __po.observe({entryTypes:['longtask']});
 window.__Z=[]; window.__zt=setInterval(()=>__Z.push([performance.now()|0, +G.cam.zoom.toFixed(2)]), 50); })()""")
def notch(dy, n, gap):
    for _ in range(n):
        c.call('Input.dispatchMouseEvent', type='mouseWheel', x=500, y=360, deltaX=0, deltaY=dy)
        time.sleep(gap)
t0 = time.time()
notch(-100, 6, 0.08)            # 6 notches in
time.sleep(1.0)
t_rev = c.js("performance.now()")
notch(100, 6, 0.08)             # reverse: 6 notches out
time.sleep(2.5)
r = c.js("""(()=>{ __stop=true; __po.disconnect(); clearInterval(__zt);
 const f=__F.slice(1); return { frames:f.length, max:Math.max(...f)|0, over50:f.filter(x=>x>50).length, over100:f.filter(x=>x>100).length,
   longtasks:__LT, zoomTrace:__Z, zoom:+G.cam.zoom.toFixed(2), perf:DEV.perf(), polys:G.parcelPolys.length, fps:G.buildingFootprints.length }; })()""")
c.call('Emulation.setCPUThrottlingRate', rate=1)
zt = r.pop('zoomTrace'); zmax = max(z for _, z in zt); zmin = min(z for _, z in zt)
after = [z for t, z in zt if t >= t_rev]
# reversal latency: ms from first reverse notch until zoom starts decreasing
lat_ms = None
for i in range(1, len(after)):
    if after[i] < after[i-1] - 0.01: lat_ms = i * 50; break
print(json.dumps(dict(r, rate=rate, zoom_peak=zmax, zoom_min=zmin, reversal_ms=lat_ms), indent=1))
