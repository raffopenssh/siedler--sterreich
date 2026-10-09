const fs=require('fs');const src=fs.readFileSync('srv/static/i18n.js','utf8');const dict=src.slice(0,src.indexOf('// Runtime:'));
const ctx={};new Function('with(this){'+dict+'; this.I18N_EXACT=I18N_EXACT; this.I18N_RX=I18N_RX;}').call(ctx);
const words=process.argv.slice(2).join(' ').split('|');
console.log(words.filter(w=>ctx.I18N_EXACT[w]===undefined && !ctx.I18N_RX.some(r=>r[0].test(w))).join('|'));
