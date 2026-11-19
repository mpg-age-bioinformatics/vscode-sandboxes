#!/usr/bin/env node
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const html=fs.readFileSync(require('node:path').join(__dirname,'../internal/startui/window.html'),'utf8');
const start=html.indexOf('async function exportActivity(){'),end=html.indexOf("$('exportActivity').addEventListener",start);
(async()=>{
 for(const mode of ['native','browser','failed']){
  const button={},error={},links=[],saved=[];
  const context=vm.createContext({document:{createElement:()=>({click(){this.clicked=true;},remove(){}}),body:{append:link=>links.push(link)}},window:{webkit:mode==='native'?{messageHandlers:{exportActivity:{postMessage:async text=>saved.push(text)}}}:undefined},$:id=>id==='error'?error:button,fetch:async(route)=>{assert.equal(route,'activity');return {ok:mode!=='failed',text:async()=>mode==='failed'?'Export failed':'redacted startup activity'};}});
  vm.runInContext(html.slice(start,end),context);await context.exportActivity();
  assert.equal(button.disabled,false);
  if(mode==='native')assert.deepEqual(saved,['redacted startup activity']);
  if(mode==='browser'){assert.equal(links[0].href,'activity?download');assert.equal(links[0].download,'startup-activity.txt');assert(links[0].clicked);}
  if(mode==='failed')assert.equal(error.textContent,'Export failed');
 }
 console.log('Native and browser activity export and error checks passed.');
})().catch(error=>{console.error(error);process.exitCode=1;});
