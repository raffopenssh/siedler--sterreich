"""CPU-profile a JS snippet via CDP and print top self-time functions.
usage: python3 tools/cpuprof.py <port> "<js expression (awaited)>" [rate]"""
import sys, json, collections
sys.path.insert(0, 'tools'); from cdp import CDP
port=int(sys.argv[1]); expr=sys.argv[2]; rate=float(sys.argv[3]) if len(sys.argv)>3 else 1
c=CDP(port); c.call('Emulation.setCPUThrottlingRate', rate=rate)
c.call('Profiler.enable'); c.call('Profiler.setSamplingInterval', interval=200); c.call('Profiler.start')
c.js(expr); p=c.call('Profiler.stop')['profile']; c.call('Profiler.disable'); c.call('Emulation.setCPUThrottlingRate', rate=1)
nodes={n['id']:n for n in p['nodes']}; parent={}
for n in p['nodes']:
    for ch in n.get('children',[]): parent[ch]=n['id']
selfT=collections.Counter(); totT=collections.Counter()
dts=p['timeDeltas']; 
for s,dt in zip(p['samples'],dts):
    n=nodes[s]; cf=n['callFrame']; key=f"{cf['functionName'] or '(anon)'}:{cf['lineNumber']+1}"
    selfT[key]+=dt
    seen=set(); i=s
    while i in nodes:
        cf=nodes[i]['callFrame']; k=f"{cf['functionName'] or '(anon)'}:{cf['lineNumber']+1}"
        if k not in seen: totT[k]+=dt; seen.add(k)
        i=parent.get(i); 
        if i is None: break
tot=sum(dts)/1000
print(f"total {tot:.0f} ms sampled")
print("-- self ms --"); [print(f"{v/1000:7.0f}  {k}") for k,v in selfT.most_common(22)]
print("-- total ms --"); [print(f"{v/1000:7.0f}  {k}") for k,v in totT.most_common(25) if 'root' not in k]
