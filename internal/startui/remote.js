// Remote credentials stay separate; image builds use a dedicated local sandbox.
(function(){
 const element=id=>document.getElementById(id);
 const fields={network:'remoteNetwork',sites:'remoteSites',login:'remoteLogin',image:'remoteImage',imageSource:'remoteImageSource',version:'remoteVersion',architecture:'remoteArchitecture',project:'remoteProject',folders:'remoteFolders',stateDir:'remoteStateDir',imagesDir:'remoteImagesDir',partition:'remotePartition',cpus:'remoteCPUs',memory:'remoteMemory',time:'remoteTime',gpus:'remoteGPUs',apptainer:'remoteApptainer',moduleNames:'remoteModules',modules:'remoteModuleSetup',waitMinutes:'remoteWaitMinutes'};
 const csv=value=>[...new Set(value.split(',').map(item=>item.trim()).filter(Boolean))].join(', ');
 const numeric=new Set(['cpus','gpus','waitMinutes']);
 let images=[],sandboxes=[],sitePresets={},lastDefaultSites='';
 let cache=null,cacheReading=false,cacheClearing=false,cacheError='',cacheActionError='',cacheMessage='',cacheNextCheck=0;
 let dockerfile='',lastSource='',presetVersions={python:'3.13',r:'4.5'};
 let loaded=false,visible=false,connectMode=false,requesting=false,clientError='',status={running:false,session:{}},requirements={};
 async function request(route,options){const response=await fetch(route,options);if(!response.ok)throw Error(await response.text());return response;}
 function sourceControls(){
  const source=element('remoteImageSource').value||'sif',preset=source==='python'||source==='r',build=!['sif','cached'].includes(source);
  element('remoteCachedImageRow').hidden=source!=='cached';
  element('remoteVersionRow').hidden=!preset;element('remoteVersion').required=preset;
  element('remoteVersionLabel').textContent=source==='r'?'R version':'Python version';
  element('remoteVersion').placeholder=source==='r'?'4.5 or 4.5.1':'3.13';
  element('remoteVersionHint').textContent=source==='r'?'Choose a major.minor series or a published patch version.':'Python uses the same major.minor image series as Local.';
  element('remoteImageRow').hidden=source!=='sif'&&source!=='registry';element('remoteImage').required=source==='sif'||source==='registry';
  element('remoteImageLabel').textContent=source==='sif'?'SIF path on the HPC':'Online registry image';
  element('remoteImage').placeholder=source==='sif'?'/shared/images/workspace.sif':'docker.io/library/ubuntu:24.04';
  element('remoteImageHint').textContent=source==='sif'?'Use an existing SIF with SSH, VS Code dependencies and Codex.':'Use an OCI image name, optionally prefixed with docker://, rather than a Docker Hub web page URL.';
  element('remoteDockerfileRow').hidden=source!=='dockerfile';element('remoteArchitectureRow').hidden=!build;element('remoteBuildNotice').hidden=!build;
  element('remoteSourceHint').textContent=preset||source==='bioinformatics'?'Uses the same base image and project layout as Local, with an Apptainer runtime.':source==='cached'?'Automatically lists ready images from this server. Choose one below to reuse its original environment.':source==='sif'?'Opens your existing image without rebuilding it.':'Adds SSH, VS Code and agent dependencies and retains the complete Dockerfile.';
 }
 function profile(){const result={};for(const [key,id]of Object.entries(fields)){const value=element(id).value;result[key]=numeric.has(key)?Number(value):key==='modules'?value:value.trim();}result.folders=csv(result.folders);result.moduleNames=csv(result.moduleNames);result.cachedImage=result.imageSource==='cached'?element('remoteCachedImage').value:'';result.dockerfile=result.imageSource==='dockerfile'?dockerfile:'';if(!['python','r'].includes(result.imageSource))result.version='';return result;}
 function endpointSelection(){
  const baseURL=element('remoteEndpointURL').value.trim(),apiKey=element('remoteEndpointKey').value.trim();
  if(!baseURL&&!apiKey)return {};
  if(!baseURL||!apiKey){clientError='Enter both a private endpoint URL and an API key.';render();return null;}
  return {baseURL,apiKey,privateAgent:element('remotePrivateAgent').value||'continue'};
 }
 function defaultSites(){
  const source=element('remoteImageSource').value;
  return [...new Set([...(sitePresets[(source==='python'||source==='r'?source:'bioinformatics')+'-sandbox']||[]),'api.openai.com','auth.openai.com','chatgpt.com','ab.chatgpt.com','api.anthropic.com'])].join(', ');
 }
 function networkNotice(){
  const mode=element('remoteNetwork').value||'open',url=element('remoteEndpointURL').value.trim();
  for(const [id,value]of [['remoteNetworkNone','none'],['remoteNetworkRestricted','restricted'],['remoteNetworkOpen','open']])element(id).checked=value===mode;
  element('remoteSitesRow').hidden=mode!=='restricted';
  let text='';
  if(url&&mode==='none')text='No internet cannot be used with a private endpoint. Choose Selected sites or Open.';
  else if(url&&mode==='restricted'){
   try{const parsed=new URL(url);text='Warning: private endpoint host '+parsed.host+' is automatically included in Selected sites for new allocations.';}catch(_){text='Enter a valid private endpoint URL.';}
  }
  element('remoteNetworkNotice').textContent=text;element('remoteNetworkNotice').hidden=!text;
 }
 function render(){
  sourceControls();networkNotice();
  element('remoteNewPanel').hidden=connectMode;element('remoteConnectPanel').hidden=!connectMode;
  for(const [id,selected]of [['remoteNewTab',!connectMode],['remoteConnectTab',connectMode]]){element(id).setAttribute('aria-selected',String(selected));element(id).tabIndex=selected?0:-1;}
  const needsBuild=!connectMode&&!['sif','cached'].includes(element('remoteImageSource').value);
  const s=status.session||{},busy=!!status.running||!!requirements.installing||requesting||cacheClearing||!!cache?.clearing;
  window.remoteBusy=busy;
  document.body.dataset.running=busy||(typeof active!=='undefined'&&active)||(typeof installingPrerequisites!=='undefined'&&installingPrerequisites)?'true':'false';
  const localBusy=typeof active!=='undefined'&&active;
  const cacheBusy=!!status.running||!!requirements.installing||cacheClearing||!!cache?.clearing||localBusy;
  element('remoteClearCache').disabled=cacheBusy||cacheReading||!!cacheError||!cache||cache.inUse||!cache.measured||!(cache.bytes||cache.entries);
  element('remoteClearCache').textContent=cacheClearing||cache?.clearing?'Clearing cache…':'Clear cache';
  element('remoteCacheUsage').textContent=cacheClearing||cache?.clearing?'Clearing local cached build files…':cacheError?'Local cache size unavailable.':cache?.measured?formatCacheBytes(cache.bytes)+' on this laptop · '+cache.entries+' cache entr'+(cache.entries===1?'y':'ies')+(cache.inUse?' · in use':''):cache?.inUse?'Cache is in use by another launcher.':'Checking local cache size…';
  element('remoteCacheMessage').textContent=cacheMessage;
  element('remoteCacheError').textContent=cacheActionError||cacheError;
  element('remoteFields').disabled=busy||!!s.active;
  element('remoteEndpointFields').disabled=busy;
  const endpointReady=!!element('remoteEndpointURL').value.trim()&&!!element('remoteEndpointKey').value.trim();
  element('remotePrivateAgent').disabled=busy||!endpointReady;
  element('remotePrivateAgentHint').textContent=element('remotePrivateAgent').value==='codex'?'Continue and Codex use this private endpoint. Codex requires the Responses API.':'Continue uses the private endpoint; Codex keeps its original connection.';

  element('remoteConnectionFields').disabled=busy||(!connectMode&&!!s.active);
  element('remoteStart').disabled=connectMode||busy||!!s.active||!requirements.ssh||!requirements.vscode||(element('remoteImageSource').value==='cached'&&!element('remoteCachedImage').value);
  element('remoteCurrentActions').hidden=connectMode||!s.active;
  element('remoteReconnect').disabled=busy||!s.active;
  const preparing=status.running&&['Preparing remote image and project files','Uploading image archive and reusable project recipe','Uploading project template for cached server image'].includes(status.phase);
  element('remoteCancel').disabled=requesting||!s.active||(!s.jobID&&!preparing);
  element('remoteCancel').textContent=preparing?'Cancel preparation':'Cancel allocation';
  element('remoteInstallEditor').hidden=!!requirements.vscode&&(!needsBuild||!!requirements.docker);
  element('remoteInstallEditor').textContent=needsBuild?'Install missing local tools':'Install Visual Studio Code';
  element('remoteInstallEditor').disabled=busy||!requirements.supported;
  element('remoteListSandboxes').disabled=busy||!requirements.ssh;
  const selected=sandboxes.find(item=>item.token===element('remoteSandboxList').value);
  element('remoteAttach').disabled=busy||!selected||!requirements.vscode;
  element('remoteStopSelected').disabled=busy||!selected;
  element('remoteNew').disabled=busy||!s.active;
  element('remoteRequirements').textContent=(requirements.ssh?'SSH ready. ':'OpenSSH (ssh and ssh-keygen) is required on this laptop. ')+(requirements.vscode?'VS Code detected.':'Visual Studio Code is required.')+(needsBuild?(requirements.docker?' Local image builder ready.':' Docker Sandboxes is needed only if this image is absent from the server cache.'):'');
  element('remotePhase').textContent=requirements.installing?'Installing missing local tools…':status.phase||(connectMode?'Ready to connect':'Ready to submit');
  element('remoteStorageNotice').hidden=!s.active||!s.jobID||!!s.projectPath;
  element('remoteJob').textContent=s.active?'Allocation '+(s.jobID||'submission pending')+(s.node?' · '+s.node+':'+s.port:'')+(s.projectPath?' · '+s.projectPath:''):'No active allocation';
  element('remoteError').textContent=clientError||status.error||requirements.error||'';
  const text=(status.text||'')+(requirements.installing&&requirements.log?'\n'+requirements.log:'');
  const log=element('remoteLog'),atBottom=log.scrollHeight-log.scrollTop-log.clientHeight<70;
  log.textContent=text||'Submission, queue state, container startup and connection details will appear here.';if(atBottom)log.scrollTop=log.scrollHeight;
  if(typeof updateConnectionControls==='function')updateConnectionControls();
 }
 function formatCacheBytes(bytes){
  if(!bytes)return '0 B';
  const units=['B','KiB','MiB','GiB','TiB'];let value=bytes,unit=0;
  while(value>=1024&&unit<units.length-1){value/=1024;unit++;}
  return (unit?value.toFixed(1):String(value))+' '+units[unit];
 }
 async function refreshCache(force=false){
  if(cacheReading||cacheClearing||(!force&&Date.now()<cacheNextCheck))return;
  cacheReading=true;render();
  try{cache=await(await request('remote/cache')).json();cacheError='';}
  catch(error){cacheError=error.message;}
  finally{cacheReading=false;cacheNextCheck=Date.now()+15000;render();}
 }
 element('remoteClearCache').addEventListener('click',async()=>{
  if(element('remoteClearCache').disabled)return;
  cacheClearing=true;cacheActionError='';cacheMessage='';render();
  try{
   cache=await(await request('remote/cache/clear',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})).json();
   cacheMessage='Removed '+formatCacheBytes(cache.removedBytes||0)+' of local cached files. Images already cached on the HPC remain available.';
  }catch(error){cacheActionError=error.message;}
  finally{cacheClearing=false;cacheNextCheck=0;render();await refreshCache(true);}
 });
 async function refresh(){
  try{const wasRunning=!!status.running;const data=await(await request('remote/status')).json();status=data.status;requirements=data.prerequisites;render();if(visible)await refreshCache(wasRunning&&!status.running);}
  catch(error){clientError=error.message;element('remoteError').textContent=clientError;}
 }
 async function initialize(){
  const data=await(await request('remote/defaults')).json();status=data.status;requirements=data.prerequisites;sitePresets=data.sitePresets||{};
  for(const item of data.sandboxes||[])presetVersions[item.slug.replace('-sandbox','')]=item.version;
  const profile=status.session?.active?status.session.profile:data.profile;
  for(const [key,id]of Object.entries(fields))element(id).value=profile[key]??'';
  element('remoteNetwork').value=profile.network||'open';
  element('remoteImagesDir').value=profile.imagesDir||'~/.local/share/vscode-sandboxes/images';
  element('remoteProject').value=profile.project||'~/sandboxes';element('remotePartition').value=profile.partition||'cluster';element('remoteCPUs').value=profile.cpus??1;element('remoteFolders').value=profile.folders||profile.data||'';
  element('remoteImageSource').value=profile.imageSource||(profile.image?'sif':'python');element('remoteArchitecture').value=profile.architecture||'amd64';element('remoteVersion').value=profile.version||presetVersions[element('remoteImageSource').value]||'';lastSource=element('remoteImageSource').value;
  lastDefaultSites=defaultSites();element('remoteSites').value=profile.sites??lastDefaultSites;
  dockerfile=profile.dockerfile||'';element('remoteDockerfilePreview').value=dockerfile;element('remoteDockerfilePreview').hidden=!dockerfile;
  if(profile.cachedImage){setOptions('remoteCachedImage',[{value:profile.cachedImage,label:'Saved image '+profile.cachedImage.slice(0,12)}],'Select a server image');element('remoteCachedImage').value=profile.cachedImage;}
  loaded=true;render();await refreshCache(true);
  if(status.session?.active)selectRemoteMode(true);else if(element('remoteImageSource').value==='cached')await listImages();
  request('remote/recipe').then(r=>r.text()).then(text=>element('remoteRecipe').textContent=text).catch(error=>element('remoteRecipe').textContent=error.message);
 }
 async function action(route,profile){
  requesting=true;clientError='';render();
  try{await request(route,{method:'POST',headers:{'Content-Type':'application/json'},body:profile?JSON.stringify(profile):undefined});if(['remote/start','remote/attach','remote/reconnect'].includes(route)){element('remoteEndpointURL').value='';element('remoteEndpointKey').value='';}await refresh();}
  catch(error){clientError=error.message;}
  finally{requesting=false;render();}
 }
 element('remoteForm').addEventListener('submit',event=>{
  event.preventDefault();if(connectMode||element('remoteStart').disabled)return;
  if(element('remoteImageSource').value==='cached'&&!element('remoteCachedImage').value){clientError='List images and select a server image first.';render();return;}
  if(element('remoteImageSource').value==='dockerfile'&&!dockerfile){clientError='Choose a Dockerfile first.';render();return;}
  const endpoint=endpointSelection();if(endpoint===null)return;
  if(element('remoteNetwork').value==='none'&&endpoint.baseURL){clientError='No internet cannot be used with a private endpoint. Choose Selected sites or Open.';render();return;}
  action('remote/start',{...profile(),endpoint});
 });
 element('remoteNetwork').addEventListener('change',render);
 for(const [id,mode]of [['remoteNetworkNone','none'],['remoteNetworkRestricted','restricted'],['remoteNetworkOpen','open']])element(id).addEventListener('change',()=>{if(element(id).checked){element('remoteNetwork').value=mode;render();}});
 element('remoteResetSites').addEventListener('click',()=>{lastDefaultSites=defaultSites();element('remoteSites').value=lastDefaultSites;render();});
 element('remoteImageSource').addEventListener('change',()=>{
  const source=element('remoteImageSource').value;
  if(source==='python'&&lastSource!=='python')element('remoteVersion').value=presetVersions.python;
  if(source==='r'&&lastSource!=='r')element('remoteVersion').value=presetVersions.r;
  if((source==='sif'||source==='registry')&&lastSource!==source)element('remoteImage').value='';
  if(element('remoteSites').value===lastDefaultSites){lastDefaultSites=defaultSites();element('remoteSites').value=lastDefaultSites;}
  lastSource=source;render();
  if(source==='cached')listImages();
 });
 element('remoteDockerfileFile').addEventListener('change',async()=>{
  clientError='';dockerfile='';const file=element('remoteDockerfileFile').files?.[0];
  try{if(file){if(file.size>512*1024)throw Error('Dockerfile exceeds 512 KiB.');dockerfile=await file.text();element('remoteDockerfileName').textContent=file.name+' · '+file.size+' bytes';}}
  catch(error){clientError=error.message;}
  element('remoteDockerfilePreview').value=dockerfile;element('remoteDockerfilePreview').hidden=!dockerfile;render();
 });
 element('remoteDownloadDockerfile').addEventListener('click',async()=>{
  try{const response=await request('remote/dockerfile',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(profile())});const url=URL.createObjectURL(await response.blob()),link=document.createElement('a');link.href=url;link.download='Dockerfile';link.click();URL.revokeObjectURL(url);}
  catch(error){clientError=error.message;render();}
 });

 function setOptions(id,items,placeholder){
  const select=element(id);select.textContent='';
  for(const item of [{value:'',label:placeholder},...items]){const option=document.createElement('option');option.value=item.value;option.textContent=item.label;select.appendChild(option);}
 }
 async function catalogRequest(route,payload){return (await request(route,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile:profile(),...payload})}));}
 async function listImages(){
  const selected=element('remoteCachedImage').value;
  if(!element('remoteLogin').value.trim()){images=[];setOptions('remoteCachedImage',[],'Enter an SSH login above');element('remoteCachedImage').value='';element('remoteImagesStatus').textContent='Enter your SSH login to load available images automatically.';render();return;}
  requesting=true;clientError='';element('remoteCachedImage').value='';element('remoteImagesStatus').textContent='Loading available images…';render();
  try{images=await(await catalogRequest('remote/images')).json();setOptions('remoteCachedImage',images.map(item=>({value:item.id,label:[item.source,item.version,item.architecture,item.id.slice(0,12),Math.round(item.size/1048576)+' MiB'].filter(Boolean).join(' · ')})),images.length?'Select a server image':'No cached images available');element('remoteCachedImage').value=images.some(item=>item.id===selected)?selected:'';element('remoteImagesStatus').textContent=images.length+' cached image(s) available. Selecting one skips image build, transfer and conversion.';}
  catch(error){images=[];setOptions('remoteCachedImage',[],'Could not load images — retry Refresh images');element('remoteCachedImage').value='';clientError=error.message;}finally{requesting=false;render();}
 }
 async function listSandboxes(){
  requesting=true;clientError='';element('remoteSandboxesStatus').textContent='Reading running allocations…';render();
  try{sandboxes=await(await catalogRequest('remote/sandboxes')).json();setOptions('remoteSandboxList',sandboxes.map(item=>({value:item.token,label:[item.jobID,item.queueState,item.profile.project,'Internet: '+(item.profile.network||'open')].join(' · ')})),'Select an allocation');element('remoteSandboxesStatus').textContent=sandboxes.length+' running or queued sandbox allocation(s). Attach opens the existing job; Stop cancels only the selected allocation.';}
  catch(error){clientError=error.message;}finally{requesting=false;render();}
 }
 element('remoteListImages').addEventListener('click',listImages);
 for(const id of ['remoteLogin','remoteImagesDir','remoteStateDir','remoteModules','remoteModuleSetup'])element(id).addEventListener('change',()=>{if(connectMode){sandboxes=[];setOptions('remoteSandboxList',[],'Select an allocation');if(element('remoteLogin').value.trim())listSandboxes();else{element('remoteSandboxesStatus').textContent='Enter your SSH login above to find running sandboxes.';render();}}else if(element('remoteImageSource').value==='cached'){element('remoteCachedImage').value='';listImages();}});
 element('remoteListSandboxes').addEventListener('click',listSandboxes);
 element('remoteCachedImage').addEventListener('change',()=>{if(element('remoteCachedImage').value){element('remoteImageSource').value='cached';lastSource='cached';}render();});
 element('remoteSandboxList').addEventListener('change',render);
 element('remoteAttach').addEventListener('click',async()=>{
  const selected=sandboxes.find(item=>item.token===element('remoteSandboxList').value);if(!selected)return;
  const endpoint=endpointSelection();if(endpoint===null)return;
  await action('remote/attach',{profile:profile(),token:selected.token,jobID:selected.jobID,endpoint});
  if(!clientError){const attached=status.session?.profile;if(attached){for(const [key,id]of Object.entries(fields))element(id).value=attached[key]??'';element('remoteImagesDir').value=attached.imagesDir||'~/.local/share/vscode-sandboxes/images';if(attached.cachedImage){setOptions('remoteCachedImage',[{value:attached.cachedImage,label:'Attached image '+attached.cachedImage.slice(0,12)}],'Select an image');element('remoteCachedImage').value=attached.cachedImage;}lastSource=element('remoteImageSource').value;dockerfile=attached.dockerfile||'';render();}}

 });
 element('remoteStopSelected').addEventListener('click',async()=>{
  const selected=sandboxes.find(item=>item.token===element('remoteSandboxList').value);if(!selected)return;
  await action('remote/stop-selected',{profile:profile(),token:selected.token,jobID:selected.jobID});await listSandboxes();
 });
 element('remoteNew').addEventListener('click',async()=>{await action('remote/detach',{});if(!clientError)selectRemoteMode(false);});
 element('remoteReconnect').addEventListener('click',()=>{const endpoint=endpointSelection();if(endpoint!==null)action('remote/reconnect',{endpoint});});
 for(const id of ['remoteEndpointURL','remoteEndpointKey'])element(id).addEventListener('input',render);
 element('remotePrivateAgent').addEventListener('change',render);
 element('remoteCancel').addEventListener('click',()=>action('remote/cancel'));
 element('remoteInstallEditor').addEventListener('click',async()=>{
  requesting=true;clientError='';render();
  try{await request('install-prerequisites',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({components:connectMode||['sif','cached'].includes(element('remoteImageSource').value)?['vscode']:['docker','vscode']})});await refresh();}
  catch(error){clientError=error.message;}
  finally{requesting=false;render();}
 });

 function selectRemoteMode(connect){
  connectMode=connect;
  render();
  if(!loaded||requesting||status.running)return;
  if(connect){if(element('remoteLogin').value.trim())listSandboxes();else element('remoteSandboxesStatus').textContent='Enter your SSH login above to find running sandboxes.';}
  else if(element('remoteImageSource').value==='cached'&&!status.session?.active)listImages();
 }
 element('remoteNewTab').addEventListener('click',()=>selectRemoteMode(false));
 element('remoteConnectTab').addEventListener('click',()=>selectRemoteMode(true));
 for(const id of ['remoteNewTab','remoteConnectTab'])element(id).addEventListener('keydown',event=>{
  if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;
  event.preventDefault();const connect=event.key==='End'||(event.key!=='Home'&&id==='remoteNewTab');selectRemoteMode(connect);element(connect?'remoteConnectTab':'remoteNewTab').focus();
 });
 function selectTab(remote){
  visible=remote;
  element('localPanel').hidden=remote;element('remotePanel').hidden=!remote;
  for(const [id,selected]of [['localTab',!remote],['remoteTab',remote]]){element(id).setAttribute('aria-selected',String(selected));element(id).tabIndex=selected?0:-1;}
  if(remote){(loaded?refresh():initialize()).catch(error=>{clientError=error.message;element('remoteError').textContent=clientError;});}
 }
 element('localTab').addEventListener('click',()=>selectTab(false));
 element('remoteTab').addEventListener('click',()=>selectTab(true));
 for(const id of ['localTab','remoteTab'])element(id).addEventListener('keydown',event=>{
  if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;
  event.preventDefault();const remote=event.key==='End'||(event.key!=='Home'&&id==='localTab');selectTab(remote);element(remote?'remoteTab':'localTab').focus();
 });
 async function pollRemote(){if(loaded&&(visible||status.running||status.session?.active))await refresh();setTimeout(pollRemote,2000);}
 pollRemote();
})();
