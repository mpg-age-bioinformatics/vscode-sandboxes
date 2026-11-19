#!/usr/bin/env node
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const SiteListState=require('../internal/startui/site-presets.js');
const html=fs.readFileSync(path.join(__dirname,'../internal/startui/window.html'),'utf8');
const script=html.match(/<script>\n([\s\S]*?)<\/script>/)[1].replace("initialize().catch(e=>{$('setupError').textContent=e.message;$('error').textContent=e.message;});poll();",'');
function node(tag='input'){return {tagName:tag.toUpperCase(),focus(){this.focused=true;},select(){this.selected=true;},replaceWith(replacement){elements.set(this.id,replacement);},value:'',textContent:'',hidden:false,disabled:false,children:[],listeners:{},classList:{toggle(){}},
 replaceChildren(){this.children=[];this.value='';},append(child){this.children.push(child);},
 addEventListener(type,handler){const old=this.listeners[type];this.listeners[type]=old?(...args)=>{old(...args);handler(...args);}:handler;}};}
const elements=new Map();
const element=id=>{if(!elements.has(id)){const created=node();created.id=id;elements.set(id,created);}return elements.get(id);};
let network='restricted',starts=0;
const context=vm.createContext({SiteListState,URL,AbortController,console,document:{body:{dataset:{}},getElementById:element,createElement:node,
 querySelectorAll:()=>[],querySelector:()=>({value:network}),addEventListener(){}},window:{addEventListener(){}},fetch:async(route)=>{
 if(route==='defaults')return {json:async()=>({slug:'python-sandbox',project:'/project/workspace',version:'3.13',versionLabel:'Python version',saved:{},import:'',sitePresets:{'python-sandbox':['pypi.org'],'r-sandbox':['cran.r-project.org']}})};
 if(route==='start'){starts++;return {ok:true};}
 throw Error('Unexpected request: '+route);
}});
vm.runInContext(script,context);
(async()=>{
 await context.initialize();
 element('baseURL').value='https://Lite.Example:443/v1';element('baseURL').listeners.input();
 assert.equal(element('sites').value,'pypi.org\nlite.example');
 assert.match(element('endpointNetworkNotice').textContent,/Warning: lite.example/);
 assert.equal(element('endpointNetworkNotice').hidden,false);
 element('sites').value+='\nmanual.example';
 element('baseURL').value='https://other.example:8443/v1';element('baseURL').listeners.input();
 assert.equal(element('sites').value,'pypi.org\nmanual.example\nother.example:8443');
 context.updateSites('r-sandbox');
 assert.equal(element('sites').value,'cran.r-project.org\nother.example:8443');
 element('resetSites').listeners.click();
 assert.equal(element('sites').value,'cran.r-project.org\nother.example:8443');
 network='none';context.visibility();
 assert.match(element('endpointNetworkNotice').textContent,/No internet cannot be used/);
 assert.equal(element('start').disabled,true);
 await element('form').listeners.submit({preventDefault(){}});
 assert.equal(starts,0);
 network='open';context.visibility();
 assert.equal(element('token').required,true);
 for(const key of ['', '   ']){
  element('token').value=key;element('token').listeners.input();
  assert.equal(element('start').disabled,true);
  await element('form').listeners.submit({preventDefault(){}});
  assert.equal(starts,0);
  assert.match(element('error').textContent,/Enter an API key/);
 }
 element('token').value='sandbox-key';element('token').listeners.input();
 assert.equal(element('start').disabled,false);
 network='restricted';context.visibility();
 element('baseURL').value='';element('token').value='';element('baseURL').listeners.input();
 assert.equal(element('sites').value,'cran.r-project.org');
 assert.equal(element('endpointNetworkNotice').hidden,true);
 element('token').value='';element('token').listeners.input();
 assert.equal(element('token').required,false);
 assert.equal(element('start').disabled,false);
 console.log('Private endpoint automatic allowlist, warnings, and offline startup checks passed.');
})().catch(error=>{console.error(error);process.exitCode=1;});
