#!/usr/bin/env node
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const SiteListState=require('../internal/startui/site-presets.js');
const html=fs.readFileSync(require('node:path').join(__dirname,'../internal/startui/window.html'),'utf8');
const script=html.match(/<script>\n([\s\S]*?)<\/script>/)[1].replace("initialize().catch(e=>{$('setupError').textContent=e.message;$('error').textContent=e.message;});poll();",'');
function harness(info,projectMode=false){
 const elements=new Map([...html.matchAll(/id="([^"]+)"/g)].map(([,id])=>[id,{value:'',listeners:{},textContent:'',replaceChildren(){},classList:{toggle(){}},addEventListener(type,fn){this.listeners[type]=fn;}}]));
 const element=id=>{assert(elements.has(id),id);return elements.get(id);};
 const calls=[],events={};let current=info,fail=false;
 const context=vm.createContext({SiteListState,URL,setTimeout(){},document:{body:{dataset:{}},getElementById:element,querySelectorAll:()=>[],querySelector:()=>({value:'open'}),addEventListener(){}},window:{addEventListener(event,fn){events[event]=fn;}},fetch:async(route,options)=>{
  calls.push(route);
  if(route==='defaults')return {json:async()=>({slug:'python-sandbox',name:'Sandbox',project:'/workspace',version:'3.13',versionLabel:'Python version',projectMode,saved:{},sitePresets:{'python-sandbox':['pypi.org']},prerequisites:current})};
  if(route==='prerequisites'){if(fail)throw Error('offline controller');return {ok:true,json:async()=>current};}
  if(route==='install-prerequisites'){assert.equal(options.method,'POST');current={...current,installing:true};return {ok:true};}
  if(route==='start')throw Error('must never start with missing tools');
  throw Error('Unexpected route '+route);
 }});
 vm.runInContext(script,context);
 return {context,element,calls,events,set(info){current=info;},fail(){fail=true;}};
}
(async()=>{
 for(const [docker,vscode] of [[false,false],[false,true],[true,false],[true,true]]){
  for(const projectMode of [false,true]){
   const ready=docker&&vscode;
   const h=harness({docker,vscode,ready,supported:true,installing:false},projectMode);
   await h.context.initialize();
   assert.equal(h.element('setupScreen').hidden,ready);
   assert.equal(h.element('form').hidden,!ready);
   assert.equal(h.element('start').disabled,!ready);
   assert.equal(h.element('dockerDetected').textContent,docker?'Detected':'Not detected');
   assert.equal(h.element('vscodeDetected').textContent,vscode?'Detected':'Not detected');
   if(!ready){await h.element('form').listeners.submit({preventDefault(){}});assert(!h.calls.includes('start'));}
  }
 }
 const h=harness({docker:false,vscode:true,ready:false,supported:true,installing:false});
 await h.context.initialize();
 assert(!h.calls.includes('install-prerequisites'),'Opening setup must not install anything');
 await h.element('installPrerequisites').listeners.click();
 assert(h.element('installPrerequisites').disabled);
 assert.equal(h.context.document.body.dataset.running,'true');
 h.set({docker:true,vscode:true,ready:true,supported:true,installing:false});
 await h.context.refreshPrerequisites();
 assert(h.element('setupScreen').hidden);
 assert.equal(h.element('form').hidden,false);
 h.set({docker:true,vscode:false,ready:false,supported:true,installing:false});
 await h.context.refreshPrerequisites();
 assert.equal(h.element('setupScreen').hidden,false,'Setup must return if a tool disappears');
 assert(h.element('start').disabled);
 h.fail();await h.context.refreshPrerequisites();
 assert(h.element('form').hidden,'Detection failure must keep startup blocked');
 assert.match(h.element('setupError').textContent,/Could not check/);
 console.log('Missing-tool setup, project launchers, explicit install, recheck, removal, and fail-closed checks passed.');
})().catch(error=>{console.error(error);process.exitCode=1;});
