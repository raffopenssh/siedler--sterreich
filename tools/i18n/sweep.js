// Runtime sweep: paste into the browser console (or browser eval) with ?lang=en
// — lists visible text nodes / title / placeholder / aria-label that still look German.
(() => {
  const DE=/[äöüÄÖÜß]|\b(und|oder|der|die|das|mit|für|nicht|kein|keine|wird|werden|Gemeinde|Parzelle|Parzellen|Kataster|bearbeitet|Spiel|wählen|zurück|Münzen|Schätze|Lizenzen|laden|Karte|noch|schon|dein|deine|hier|jetzt|alle|wieder|Baum|Gebäude|Fläche|Nutzung|Wert|Preis|kaufen|Kaufen|verkaufen|Ernte|ernten|Brunnen|Wasser|Angebot|Spieler|Aufgabe|Zelle|Zellen|Gelände|frei|Besitzer|Daten|Fehler|Wald|Wiese|Acker|Feld|Bauland|bei|von|zum|zur|auf|aus|im|ein|eine|Jahre|Riesen|entfernt|geladen|gefunden|Suche|Ort|Lage|Höhe|Hang|Boden|Tag|Nacht)\b/;
  const out=new Set(); const w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT); let t;
  const vis=e=>{ for(let n=e;n;n=n.parentElement){ const cs=getComputedStyle(n); if(cs.display==='none'||cs.visibility==='hidden') return false;} return true; };
  while(t=w.nextNode()){ const p=t.parentElement; if(!p||['SCRIPT','STYLE'].includes(p.tagName))continue; const v=t.nodeValue.trim(); if(v&&DE.test(v)&&vis(p)) out.add(v.slice(0,160)); }
  document.querySelectorAll('[placeholder],[title],[aria-label]').forEach(e=>['placeholder','title','aria-label'].forEach(a=>{const v=e.getAttribute(a); if(v&&DE.test(v)&&vis(e)) out.add('@'+a+': '+v.slice(0,160));}));
  return [...out].join('\n');
})()
