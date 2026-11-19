#!/usr/bin/env node
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const SiteListState=require('../internal/startui/site-presets.js');
const html=fs.readFileSync(require('node:path').join(__dirname,'../internal/startui/window.html'),'utf8');
assert(/id="token" type="password"/.test(html),'The retained key must stay masked');
assert(!/id="(?:endpoint|configFile|model|browseConfig|newConfig|saveConfig|listModels)"/.test(html));
const script=html.match(/<script>\n([\s\S]*?)<\/script>/)[1].replace("initialize().catch(e=>{$('setupError').textContent=e.message;$('error').textContent=e.message;});poll();",'');
function harness(){
 const elements=new Map([...html.matchAll(/id="([^"]+)"/g)].map(([,id])=>[id,{value:'',listeners:{},textContent:'',replaceChildren(){},classList:{toggle(){}},addEventListener(type,handler){const previous=this.listeners[type];this.listeners[type]=previous?(...args)=>{previous(...args);return handler(...args);}:handler;}}]));
 const element=id=>{assert(elements.has(id),'Unknown/removed UI element: '+id);return elements.get(id);};
 const starts=[];
 const context=vm.createContext({SiteListState,URL,setTimeout(){},document:{body:{dataset:{}},createDocumentFragment:()=>({append(){}}),createTextNode:text=>text,getElementById:element,querySelectorAll:()=>[],querySelector:()=>({value:'open'}),addEventListener(){}},window:{addEventListener(){}},fetch:async(route,options)=>{
  if(route==='defaults')return {json:async()=>({slug:'python-sandbox',project:'/workspace',version:'3.13',versionLabel:'Python version',saved:{endpoint:'private',configFile:'/old/file.toml',model:'old-model',token:'never-restore-this-key'},import:'/old/file.toml',sitePresets:{'python-sandbox':['pypi.org']}})};
  if(route==='status')return {json:async()=>({done:true,text:'Workspace ready'})};
  assert.equal(route,'start','Startup must not load, preview, or save connection files');
  starts.push(JSON.parse(options.body));return {ok:true};
 }});
 vm.runInContext(script,context);
 return {context,element,starts};
}
(async()=>{
 for(const [url,key,expected,error] of [
  ['', '', 'default'],
  ['  ', '  ', 'default'],
  ['https://private.example/v1', '', null, /Enter an API key/],
  ['', 'sandbox-key', null, /Enter a URL/],
  ['https://private.example/v1', 'sandbox-key', 'private'],
 ]){
  const {context,element,starts}=harness();await context.initialize();
  assert.equal(element('token').value,'','A saved key must never populate the form');
  assert.equal(context.connectionType(),'default','Legacy file and endpoint selections must be ignored');
  element('baseURL').value=url;element('baseURL').listeners.input();
  element('token').value=key;element('token').listeners.input();
  assert.equal(element('start').disabled,!expected);
  await element('form').listeners.submit({preventDefault(){}});
  if(!expected){assert.equal(starts.length,0);assert.match(element('error').textContent,error);continue;}
  assert.equal(starts.length,1);assert.equal(starts[0].endpoint,expected);
  assert.equal(starts[0].baseURL,url.trim());assert.equal(starts[0].token,key.trim());
  for(const field of ['configFile','model','configOverrides'])assert(!(field in starts[0]));
  await context.poll();
  assert.equal(element('token').value,key,'Completing startup must retain the masked key for this session');
 }
 console.log('Optional private connection, paired URL/key validation, default OpenAI, and file-free startup checks passed.');
})().catch(error=>{console.error(error);process.exitCode=1;});
