// Impressum/imprint: live counter of our NE monitoring contribution (GET /api/contrib/stats).
(function(){
  var el=document.getElementById('contrib-stats'); if(!el) return;
  var de=el.dataset.lang!=='en';
  var L=de?{h:'Bisher gemeldet',kg:'Katastralgemeinden',km:'km² Fläche',cells:'Zellen',last:'letzter Bericht',of:'von'}
         :{h:'Reported so far',kg:'cadastral municipalities',km:'km² area',cells:'cells',last:'last report',of:'of'};
  function num(n,d){return new Intl.NumberFormat(de?'de-AT':'en-GB',{maximumFractionDigits:d||0}).format(n);}
  function tile(v,l){return '<span class="legal-stat"><b>'+v+'</b><span>'+l+'</span></span>';}
  fetch('/api/contrib/stats').then(function(r){return r.ok?r.json():null}).then(function(s){
    if(!s||!s.kgs) return;
    var d=new Date(s.last+'T00:00:00Z');
    el.innerHTML='<span class="legal-stats-h">'+L.h+'</span>'
      +tile(num(s.kgs)+' <small>'+L.of+' '+num(s.universe)+'</small>',L.kg)
      +tile(num(s.area_km2),L.km)
      +tile(num(s.cells),L.cells)
      +tile(d.toLocaleDateString(de?'de-AT':'en-GB',{day:'2-digit',month:'2-digit',year:'numeric'}),L.last);
    el.hidden=false;
  }).catch(function(){});
})();
