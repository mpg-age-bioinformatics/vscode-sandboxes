// Runs inside the sandbox. It never contacts a host Docker daemon.
const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');
const workspace = process.cwd();
const configPath = path.join(workspace, 'code', '.sandbox', 'devcontainer.json');
const lockPath = path.join(workspace, 'code', '.sandbox', 'container-image.lock.json');
const runtimePath = path.join(workspace, 'code', '.sandbox', 'sandbox-runtime.json');
const dockerEnvironment = {...process.env, DOCKER_HOST:'unix:///var/run/docker.sock', DOCKER_CONTEXT:''};
function docker(args, capture=false) {
  const result = spawnSync('docker', args, { env:dockerEnvironment, encoding:'utf8', stdio: capture ? ['ignore','pipe','inherit'] : 'inherit' });
  if (result.error) throw result.error;
  if (result.status !== 0) throw Error(`Sandbox Docker command failed (${result.status}): ${args[0]}`);
  return capture ? result.stdout.trim() : '';
}
function read(file) { return JSON.parse(fs.readFileSync(file, 'utf8')); }
function write(file, value) {
  const temporary = `${file}.tmp-${process.pid}`;
  fs.writeFileSync(temporary, JSON.stringify(value,null,2)+'\n');
  fs.renameSync(temporary,file);
}
function connection(sandbox) {
  if(!/^[a-zA-Z0-9][a-zA-Z0-9.-]*$/.test(sandbox||'')) throw Error('Invalid sandbox name.');
  const parent=`ssh-remote+${sandbox}.sbx`;
  const encodedConfig=configPath.split('/').map(encodeURIComponent).join('/');
  return {parent,configURI:`vscode-remote://${encodeURIComponent(parent)}${encodedConfig}`};
}
function pullPinned(image) {
  if (typeof image !== 'string' || !image || image.startsWith('-') || /\s/.test(image)) throw Error('Set a valid image in code/.sandbox/devcontainer.json.');
  let lock;
  if (fs.existsSync(lockPath)) lock=read(lockPath);
  const reference=lock?.requestedImage===image ? lock.resolvedImage : image;
  if (typeof reference!=='string'||!reference||reference.startsWith('-')) throw Error('Invalid image lock.');
  // Offline reuse is deliberate: do not pull a mutable tag again on every start.
  const cached=spawnSync('docker',['image','inspect',reference],{env:dockerEnvironment,stdio:'ignore'}).status===0;
  if (!cached) docker(['pull',reference]);
  const details=JSON.parse(docker(['image','inspect',reference],true))[0];
  const digest=(details.RepoDigests||[])[0];
  if (!digest || !digest.includes('@sha256:')) throw Error('The pulled image has no immutable repository digest.');
  if (reference!==image) docker(['tag',reference,image]);
  write(lockPath,{requestedImage:image,resolvedImage:digest,imageId:details.Id,architecture:details.Architecture});
  return digest;
}
try {
  switch(process.argv[2]) {
    case 'prepare': {
      const config=read(configPath);
      const runtime=read(runtimePath);
      const digest=pullPinned(runtime.baseImage);
      if (!config.build || config.build.dockerfile!=='../Dockerfile') throw Error('Use code/Dockerfile as the development container build recipe.');
      if (!fs.existsSync(path.join(workspace,'code','Dockerfile'))) throw Error('Missing code/Dockerfile.');
      config.build.args={...config.build.args,BASE_IMAGE:digest};
      write(configPath,config);
      break;
    }
    case 'record': {
      const lock=read(lockPath);
      const details=JSON.parse(docker(['image','inspect',lock.resolvedImage],true))[0];
      if(details.Id!==lock.imageId) throw Error('The image tag changed during startup. Retry before using this container.');
      console.log(`Pinned development base image: ${lock.resolvedImage}; recipe: code/Dockerfile`);
      break;
    }
    case 'uri': {
      const sandbox=process.argv[3];
      const {parent}=connection(sandbox);
      // Same nested authority format used by the installed Dev Containers extension.
      const payload={hostPath:workspace,localDocker:false,settings:{host:'unix:///var/run/docker.sock'},configFile:{scheme:'vscode-remote',authority:parent,path:configPath}};
      const authority=`dev-container+${Buffer.from(JSON.stringify(payload),'utf8').toString('hex')}@${parent}`;
      console.log(`vscode-remote://${authority}${workspace.split('/').map(encodeURIComponent).join('/')}`);
      break;
    }
    case 'config-uri': console.log(connection(process.argv[3]).configURI); break;
    default: throw Error('Expected prepare, record, uri, or config-uri.');
  }
} catch(error) { console.error(error.message); process.exit(1); }
