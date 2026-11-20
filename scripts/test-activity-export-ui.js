#!/usr/bin/env node
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const html=fs.readFileSync(require('node:path').join(__dirname,'../internal/startui/window.html'),'utf8');
const start=html.indexOf('async function exportActivity('),end=html.indexOf("$('exportActivity').addEventListener",start);
(async()=>{
 for(const remote of [false,true])for(const mode of ['native','browser','failed','cancelled']){
  const button={},error={},links=[],saved=[];
  const filename=remote?'vscode-sandox.20261120.203045.log':'startup-activity.txt';
  const route=remote?'remote/activity':'activity';
  const context=vm.createContext({Date:class extends Date{constructor(){super(2026,10,20,20,30,45);}},document:{createElement:()=>({click(){this.clicked=true;},remove(){}}),body:{append:link=>links.push(link)}},window:{webkit:['native','cancelled'].includes(mode)?{messageHandlers:{exportActivity:{postMessage:async payload=>{saved.push(payload);return mode==='cancelled'?'':'saved';}}}}:undefined},$:id=>['error','remoteError'].includes(id)?error:button,fetch:async(actual)=>{assert.equal(actual,route);return {ok:mode!=='failed',text:async()=>mode==='failed'?'Export failed':'redacted activity'};}});
  vm.runInContext(html.slice(start,end),context);await context.exportActivity(remote);
  assert.equal(button.disabled,false);
  if(['native','cancelled'].includes(mode)){
   if(remote){assert.equal(saved[0].text,'redacted activity');assert.equal(saved[0].filename,filename);}
   else assert.deepEqual(saved,['redacted activity']);
  }
  if(mode==='browser'){assert.equal(links[0].href,route+'?download');assert.equal(links[0].download,filename);assert(links[0].clicked);}
  if(mode==='failed')assert.equal(error.textContent,'Export failed');
 }
 assert(html.includes('id="exportRemoteActivity"'));
 assert(html.includes("$('exportRemoteActivity').addEventListener('click',()=>exportActivity(true))"));
 console.log('Local/Remote native and browser export, timestamped filename, cancellation and errors passed.');
})().catch(error=>{console.error(error);process.exitCode=1;});
