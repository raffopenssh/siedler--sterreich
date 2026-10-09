const fs=require('fs');
const src=fs.readFileSync('srv/static/i18n.js','utf8');
const dict=src.slice(0, src.indexOf('// Runtime:'));
const ctx={}; new Function('with(this){'+dict+'; this.I18N_EXACT=I18N_EXACT; this.I18N_RX=I18N_RX;}').call(ctx);
const {I18N_EXACT,I18N_RX}=ctx;
function trx(s){ var hit=I18N_EXACT[s]; if(hit!==undefined) return hit; var t=s.trim(); if(t!==s){hit=I18N_EXACT[t]; if(hit!==undefined) return s.replace(t,hit);} for(var i=0;i<I18N_RX.length;i++){ if(I18N_RX[i][0].test(t)){ var r=t.replace(I18N_RX[i][0],I18N_RX[i][1]); if(r!==t) return s.replace(t,r); } } return s; }
const file=process.argv[2]; const js=fs.readFileSync(file,'utf8');
const DE=/[A-Za-zÄÖÜäöüß]{3}/;
// tokenize strings
const out=[]; let i=0, line=1; const n=js.length;
function add(s){ if(!DE.test(s)) return; if(/^[\s\W]*$/.test(s)) return;
 if(/^[a-z][A-Za-z0-9_]*$/.test(s)) return; if(/^[\d\s.,%°:/()+-]*$/.test(s)) return;
 if(/^(#|rgba?\(|hsla?\(|var\(|url\(|data:|http|\/|\.|\$\{\}|\[)/.test(s)) return;
 if(/[{};=<>]/.test(s)) return; if(/^[A-Za-z0-9_$.:\/-]*$/.test(s)) return;
 if(/^[\w-]+(,\s*[\w-]+)+$/.test(s)) return; if(/^(font|bold|italic|normal|\d+px)/.test(s)) return;
 if(!/[a-zäöüß]{2}/i.test(s)) return; const t=trx(s); if(t===s) out.push(line+': '+JSON.stringify(s)); }
while(i<n){ const c=js[i];
 if(c==='\n'){line++;i++;continue;}
 if(c==='/'&&js[i+1]==='/'){ while(i<n&&js[i]!=='\n')i++; continue; }
 if(c==='/'&&js[i+1]==='*'){ const e=js.indexOf('*/',i+2); line+=(js.slice(i,e).match(/\n/g)||[]).length; i=e+2; continue; }
 if(c==="'"||c==='"'){ let j=i+1,s=''; while(j<n&&js[j]!==c){ if(js[j]==='\\'){s+=js[j+1]==='n'?'\n':js[j+1];j+=2;continue;} s+=js[j];j++; } add(s); i=j+1; continue; }
 if(c==='`'){ let j=i+1,s='',depth=0; while(j<n){ if(depth===0&&js[j]==='`')break; if(js[j]==='\\'){s+=js[j+1];j+=2;continue;} if(js[j]==='$'&&js[j+1]==='{'){ depth=1; j+=2; let k=j; while(depth>0){ if(js[k]==='{')depth++; else if(js[k]==='}')depth--; else if(js[k]==='`'){ k++; while(js[k]!=='`')k++; } k++; } s+='${}'; j=k; depth=0; continue;} if(js[j]==='\n')line++; s+=js[j]; j++; }
   // split template at tags and placeholders to approximate text nodes
   s.split(/<[^>]*>|\$\{\}/).forEach(p=>{ if(p.trim()) add(p.replace(/\$\{\}/g,'X')); });
   i=j+1; continue; }
 // regex literal: crude skip when preceded by ( , = : [ ! & | ? { } ; return
 if(c==='/'){ let k=i-1; while(k>=0&&/\s/.test(js[k]))k--; const prev=js[k]; if(/[(,=:\[!&|?{};]/.test(prev)||js.slice(k-5,k+1).match(/return$/)){ let j=i+1,cls=false; while(j<n){ if(js[j]==='\\'){j+=2;continue;} if(js[j]==='[')cls=true; else if(js[j]===']')cls=false; else if(js[j]==='/'&&!cls)break; else if(js[j]==='\n')break; j++; } i=j+1; continue; } }
 i++; }
console.log(out.join('\n')); console.error(out.length+' candidates');
