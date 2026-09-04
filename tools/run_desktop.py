import sys, json; sys.path.insert(0,'/tmp/shots2')
from cdp import CDP
c = CDP(int(sys.argv[1])); OUT='/tmp/shots2/desktop/'
c.call('Emulation.setDeviceMetricsOverride', width=1920, height=1080, deviceScaleFactor=2, mobile=False)
BASE='http://localhost:8000/?lang=de&dev=1&pid=c39355caaff8a4518f37d1dcb750398d&pname=Raffael&rejoin=a8a8d0e9b383e44e1e9001fa281ba9700fe76ef6809d4be6&sid=c09d88bc0f77ef6efa90f71e372580d8&nc=93'
W=lambda ms:f'await new Promise(r=>setTimeout(r,{ms}));'
A=lambda body: c.js(f'(async()=>{{{body}}})()')

# already on the game page (tab nav'd earlier); ensure fresh load anyway
c.nav(BASE+'#v=15.5205,48.3955,17.5', settle=5)
A(W(3000)+'DEV.freeze(false); await DEV.goto(15.5205,48.3955,17.5);'+W(2500)+'DEV.freeze();')
c.shot(OUT+'03_Spielkarte_Duernstein_Wachau.png')

A('DEV.freeze(false); await DEV.goto(15.5205,48.3955,18);'+W(1500)+'await DEV.building(0);'+W(800)+'DEV.freeze();')
c.shot(OUT+'04_Parzellen-Info_Gebaeude_LiDAR_Umgebung.png')

A('DEV.closeAll(); DEV.freeze(false); await DEV.goto(15.5205,48.3955,17.5);'+W(1500)+'const p=DEV.ezCandidates(4,20)[0]; await DEV.parcel(p.parcel_id);'+W(600)+"document.querySelector('#pp-ez-link')?.click();"+W(800)+'DEV.freeze();')
c.shot(OUT+'05_Einlagezahl_EZ_Grundbuch_Sammelkauf.png')

A('DEV.closeAll(); DEV.freeze(false); await DEV.goto(15.5218,48.3945,17.8);'+W(2000)+'DEV.freeze();')
c.shot(OUT+'06_Naturschutz_Aufforstung_umgewandelte_Parzellen.png')

r=A('DEV.closeAll(); DEV.freeze(false); const ts=DEV.treasures(); const sp=ts.find(t=>/n2k|species/.test(t.treasure_type||t.type||""))||ts[0]; await DEV.treasure(sp?sp.id:null,false,17.5);'+W(2000)+'DEV.n2k(true);'+W(800)+'DEV.freeze(); return JSON.stringify(sp);')
print('treasure', r)
c.shot(OUT+'07_Schaetze_Rote-Liste-Arten_Natura2000.png')

A('DEV.closeAll(); DEV.n2k(false); DEV.freeze(false); DEV.trees("hint"); await DEV.goto(15.5152,48.3951,17);'+W(2000)+'DEV.freeze();')
c.shot(OUT+'08_Riesenbaum-Hinweis_nach_erstem_Schatz.png')

A('DEV.freeze(false); DEV.trees("revealed");'+W(500)+'DEV.tree(0);'+W(800)+'DEV.freeze();')
c.shot(OUT+'09_Riesenbaeume_LiDAR_Hoehenhistogramm.png')

A('DEV.closeAll(); DEV.kg("12105");'+W(1500))
c.shot(OUT+'10_KG-Statistik_Katastralgemeinde_Duernstein.png')

A('DEV.closeAll(); DEV.freeze(false); await DEV.goto(15.5205,48.3955,18.5);'+W(1500)+'await DEV.building(0);'+W(1500)+'DEV.freeze();')
c.shot(OUT+'11_Gebaeude-Info_LiDAR-Hoehe_Dach_Adressen_Marktwert.png')

r=A('DEV.closeAll(); DEV.freeze(false); await DEV.goto(15.5205,48.3955,17.5);'+W(1000)+'const p=DEV.parcelsNear(p=>p.building_count>0 && p.area_sqm>300)[0]; const res=await DEV.similar(p.parcel_id,5000);'+W(500)+'await DEV.goto(p.lon||G.cam.lon,p.lat||G.cam.lat,14.5);'+W(2500)+'DEV.freeze(); return p.parcel_id;')
print('similar', r)
c.shot(OUT+'12_Aehnliche_Parzellen_5km_LiDAR-Terrain-Vergleich.png')

A('DEV.closeAll(); DEV.loading(62,"Dürnstein (12105)");'+W(800))
c.shot(OUT+'13_Ladebildschirm_Wissenskarte.png')
A('DEV.loading(false);')

A('DEV.closeAll(); DEV.freeze(false); await DEV.goto(15.5152,48.3951,18.2);'+W(1500)+'DEV.trees("revealed"); DEV.gps(15.5141,48.3959,8);'+W(500)+'DEV.freeze(); render();')
c.shot(OUT+'14_Kundschafter_GPS_Entfernung_zum_Riesenbaum.png')
print('DONE')
