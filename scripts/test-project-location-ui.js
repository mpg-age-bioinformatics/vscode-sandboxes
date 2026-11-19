#!/usr/bin/env node
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const SiteListState=require('../internal/startui/site-presets.js');
const html=fs.readFileSync(require('node:path').join(__dirname,'../internal/startui/window.html'),'utf8');
assert(html.indexOf('id="project"')<html.indexOf('id="sandbox"'),'Project folder must be first');
const script=html.match(/<script>\n([\s\S]*?)<\/script>/)[1].replace("initialize().catch(e=>$('error').textContent=e.message);poll();",'');
function harness(overrides={}){
 const root='/Users/test/sandboxes',recommended=root+'/python-sandbox-project';
 const defaults={slug:'python-sandbox',project:recommended,projectRoot:root,platform:'darwin',version:'3.13',versionLabel:'Python version',saved:{},import:'',sitePresets:{'python-sandbox':['pypi.org']},...overrides};
 const elements=new Map();
 function node(tag='input'){return {tagName:tag.toUpperCase(),value:'',textContent:'',hidden:false,disabled:false,children:[],listeners:{},classList:{toggle(){}},
  addEventListener(type,handler){this.listeners[type]=handler;},append(child){this.children.push(child);},replaceChildren(){this.children=[];},showModal(){this.shown=true;},close(){this.shown=false;}};}
 function element(id){if(!elements.has(id)){const value=node();value.id=id;elements.set(id,value);}return elements.get(id);}
 let starts=0,locations=0,picks=0;
 const context=vm.createContext({SiteListState,URL,AbortController,console,document:{body:{dataset:{}},getElementById:element,createElement:node,querySelectorAll:()=>[],querySelector:()=>({value:'open'}),addEventListener(){}},window:{addEventListener(){},webkit:{messageHandlers:{chooseFolder:{postMessage:async()=>{picks++;return '/Users/test/Desktop/chosen';}}}}},fetch:async(route,options)=>{
  if(route==='defaults')return {json:async()=>defaults};
  if(route==='project-location'){
   locations++;const {path}=JSON.parse(options.body);
   assert(!options.body.includes('token'));
   return {ok:true,json:async()=>({path,recommended:path===root||path.startsWith(root+'/'),recommendedProject:recommended,existing:overrides.existing})};
  }
  if(route==='start'){starts++;return {ok:true};}
  throw Error('Unexpected request: '+route);
 }});
 vm.runInContext(script,context);
 return {context,element,recommended,get picks(){return picks;},get starts(){return starts;},get locations(){return locations;}};
}
const tick=()=>new Promise(resolve=>setImmediate(resolve));
(async()=>{
 const h=harness(),{context,element,recommended}=h;
 await context.initialize();
 assert(!element('projectLocationDialog').shown,'Default location must not prompt');
 // Clicking or focusing editable paths must leave the native picker closed.
 for(const id of ['project']){
  element(id).listeners.click?.();element(id).listeners.focus?.();
 }
 await tick();
 assert.equal(h.picks,0,'Editing a path must not open Browse');
 assert.equal(element('project').value,recommended);
 // Native Browse selection must pause for the popup before accepting the path.
 const browse=element('browseProject').listeners.click();await tick();
 assert.equal(h.picks,1,'The Browse button must open the native picker');
 assert.equal(element('projectLocationDialog').shown,true);
 assert.match(element('projectLocationPermissions').textContent,/may require Full Disk Access/);
 assert.equal(element('projectLocationMacHelp').hidden,false);
 assert.equal(element('start').disabled,true);
 await element('form').listeners.submit({preventDefault(){}});assert.equal(h.starts,0);
 element('projectUseRecommended').onclick();await browse;
 assert.equal(element('project').value,recommended);
 assert.equal(element('start').disabled,false);
 // A typed location cannot bypass review by submitting without a change event.
 element('project').value='/Users/test/Desktop/existing-project';
 let pending=element('form').listeners.submit({preventDefault(){}});await tick();
 element('projectCancelLocation').onclick();await pending;
 assert.equal(h.starts,0);
 assert.equal(element('project').value,'/Users/test/Desktop/existing-project','Cancel must preserve the path');
 pending=element('form').listeners.submit({preventDefault(){}});await tick();
 element('projectKeepLocation').onclick();await pending;
 assert.equal(h.starts,1);
 assert.equal(element('project').value,'/Users/test/Desktop/existing-project');
 const count=h.locations;assert.equal(await context.reviewProjectLocation(),true);assert.equal(h.locations,count,'Do not repeat accepted warnings');
 // Saved custom paths stay intact and receive the same explanation on reopening.
 const existing=harness({existing:{sandbox:'python-sandbox',version:'3.13',ready:true}});
 await existing.context.initialize();
 assert.equal(existing.element('sandbox').disabled,true);
 assert.equal(existing.element('start').textContent,'Restart sandbox →');
 assert.match(existing.element('projectSandboxStatus').textContent,/restart/);
 const saved=harness({saved:{project:'/Users/test/Documents/existing'}});
 pending=saved.context.initialize();await tick();
 assert.equal(saved.element('project').value,'/Users/test/Documents/existing');
 assert.equal(saved.element('projectLocationDialog').shown,true);
 saved.element('projectKeepLocation').onclick();await pending;
 // Project runners cannot silently switch to a different workspace.
 const runner=harness({projectMode:true,project:'/Users/test/Desktop/existing'});
 pending=runner.context.initialize();await tick();
 assert.equal(runner.element('projectUseRecommended').hidden,true);
 runner.element('projectLocationDialog').oncancel({preventDefault(){}});await pending;
 assert.equal(runner.element('project').value,'/Users/test/Desktop/existing');
 // Windows must not show macOS-specific permission advice.
 const windows=harness({platform:'windows',saved:{project:'/external/project'}});
 pending=windows.context.initialize();await tick();
 assert.equal(windows.element('projectLocationMacHelp').hidden,true);
 assert(!windows.element('projectLocationPermissions').textContent.includes('Full Disk Access'));
 windows.element('projectKeepLocation').onclick();await pending;
 console.log('Project defaults, Browse/manual/saved location warnings, choices, cancellation, and platform checks passed.');
})().catch(error=>{console.error(error);process.exitCode=1;});
