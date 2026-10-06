// Impressum/imprint: live counter of our NE monitoring contribution (GET /api/contrib/stats).
(function(){
  var el=document.getElementById('contrib-stats'); if(!el) return;
  var de=el.dataset.lang!=='en';
  function num(n,d){return new Intl.NumberFormat(de?'de-AT':'en-GB',{maximumFractionDigits:d||0}).format(n);}
  function b(v){return '<b>'+v+'</b>';}
  fetch('/api/contrib/stats').then(function(r){return r.ok?r.json():null}).then(function(s){
    if(!s||!s.kgs) return;
    var date=new Date(s.last+'T00:00:00Z').toLocaleDateString(de?'de-AT':'en-GB',{day:'2-digit',month:'2-digit',year:'numeric'});
    el.innerHTML=de
      ?'Bisher gemeldet: '+b(num(s.kgs))+' von '+num(s.universe)+' Katastralgemeinden österreichweit ('+b(num(s.area_km2)+' km²')+', '+b(num(s.cells))+' Zellen), letzter Bericht am '+date+'.'
      :'Reported so far: '+b(num(s.kgs))+' of '+num(s.universe)+' cadastral municipalities across Austria ('+b(num(s.area_km2)+' km²')+', '+b(num(s.cells))+' cells), last report on '+date+'.';
    el.hidden=false;
  }).catch(function(){});
})();
