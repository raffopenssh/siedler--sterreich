import json, sys, base64, time, websocket, urllib.request

class CDP:
    def __init__(self, port):
        tabs = json.load(urllib.request.urlopen(f'http://127.0.0.1:{port}/json'))
        page = [t for t in tabs if t['type']=='page'][0]
        self.ws = websocket.create_connection(page['webSocketDebuggerUrl'], suppress_origin=True)
        self.ws.settimeout(120)
        self.i = 0
        self.dpr = 2
    def call(self, method, **params):
        self.i += 1
        self.ws.send(json.dumps({'id': self.i, 'method': method, 'params': params}))
        while True:
            m = json.loads(self.ws.recv())
            if m.get('id') == self.i:
                if 'error' in m: raise RuntimeError(m['error'])
                return m.get('result', {})
    def js(self, expr, timeout=90):
        r = self.call('Runtime.evaluate', expression=expr, awaitPromise=True, returnByValue=True, timeout=timeout*1000)
        if 'exceptionDetails' in r:
            raise RuntimeError(r['exceptionDetails'].get('exception', {}).get('description') or r['exceptionDetails'])
        return r.get('result', {}).get('value')
    def nav(self, url, settle=4):
        self.call('Page.enable')
        self.call('Page.navigate', url=url)
        time.sleep(settle)
    def shot(self, path):
        lm = self.call('Page.getLayoutMetrics'); v = lm['cssVisualViewport']; dpr = self.dpr
        r = self.call('Page.captureScreenshot', format='png', clip={'x':0,'y':0,'width':v['clientWidth'],'height':v['clientHeight'],'scale':dpr})
        open(path, 'wb').write(base64.b64decode(r['data']))
        print('saved', path, flush=True)
    def sleep(self, s): time.sleep(s)
