#!/usr/bin/env bash
# Runs only in a dedicated local Docker Sandbox, not on a host Docker engine.
set -euo pipefail
umask 077
arch="$1"; base="$2"
unset DOCKER_CONTEXT
export DOCKER_HOST=unix:///var/run/docker.sock
command -v docker >/dev/null || { echo 'The local shell sandbox needs its Docker-enabled template.' >&2; exit 1; }
check_storage() {
 node - "$1" "$2" <<'JS_SPACE'
const fs=require('fs');const path=process.argv[2],required=BigInt(process.argv[3])*1024n*1024n*1024n;
const stat=fs.statfsSync(path,{bigint:true}),available=stat.bavail*stat.bsize;
if(available<required){console.error(`Not enough free disk space for image build at ${path}: ${(Number(available)/2**30).toFixed(1)} GiB available; ${process.argv[3]} GiB required. Free host/builder storage before retrying.`);process.exit(1);}
JS_SPACE
}
docker_root="$(docker info --format '{{.DockerRootDir}}')"
check_storage "$docker_root" 20
check_storage . 20
worker_started=0
cleanup() {
 if (( worker_started )); then docker buildx rm --force remote-image-worker >/dev/null 2>&1 || true;fi
 rm -f image.tar.partial
}
trap cleanup EXIT
host_arch="$(docker info --format '{{.Architecture}}')"
case "$host_arch" in x86_64) host_arch=amd64;; aarch64) host_arch=arm64;; esac
if [[ "$host_arch" != "$arch" ]]; then
 echo "Enabling $arch emulation inside this temporary build sandbox..."
 docker run --privileged --rm tonistiigi/binfmt:qemu-v10.2.3-68 --install "$arch"
fi
# Record immutable base and Node images in the portable recipe. Custom Dockerfile
# stages are user supplied; Docker's build output records their resolved layers.
node_image=node:22.22.1-bookworm-slim
# Resolve registry descriptors without pulling/extracting base layers into the
# daemon; BuildKit downloads the pinned layers once into its own store.
pin_image() {
 docker buildx imagetools inspect "$1" --format '{{json .Manifest}}' | node -e '
const fs=require("fs");const ref=process.argv[1],manifest=JSON.parse(fs.readFileSync(0,"utf8"));
if(!/^sha256:[0-9a-f]{64}$/.test(manifest.digest))throw Error("Registry did not return an immutable digest");
console.log(ref.split("@")[0]+"@"+manifest.digest);' "$1"
}
node_digest="$(pin_image "$node_image")"
base_digest=''
if [[ -n "$base" ]]; then base_digest="$(pin_image "$base")";fi
node - "$base" "$base_digest" "$node_digest" <<'JS'
const fs=require('fs');const [base,digest,node]=process.argv.slice(2);
if(!node.includes('@sha256:')||(base&&!digest.includes('@sha256:')))throw Error('Missing immutable base image digest');
let text=fs.readFileSync('Dockerfile','utf8');
if(base)text=text.replace('FROM '+base+'\n','FROM '+digest+'\n');
text=text.replaceAll('--from=node:22.22.1-bookworm-slim','--from='+node);
fs.writeFileSync('Dockerfile',text);
JS
# Start a fresh BuildKit worker after emulation registration. The daemon's
# default worker can cache its supported architectures before binfmt is ready.
docker buildx create --name remote-image-worker --driver docker-container --driver-opt image=moby/buildkit:v0.27.0 --use
worker_started=1
docker buildx build --builder remote-image-worker --output type=docker,dest=image.tar.partial --platform "linux/$arch" --progress=plain --tag vscode-sandboxes-remote:workspace --file Dockerfile .
node - "$arch" "$base" "$base_digest" "$node_digest" <<'JS'
const fs=require('fs'),crypto=require('crypto'),cp=require('child_process');const [arch,base,digest,node]=process.argv.slice(2);
const manifest=JSON.parse(cp.execFileSync('tar',['-xOf','image.tar.partial','manifest.json'],{maxBuffer:4*1024*1024}));
if(manifest.length!==1)throw Error('Expected one Docker image in the exported archive');
const configName=manifest[0].Config;
if(!/^[a-zA-Z0-9_./-]+$/.test(configName)||configName.startsWith('/')||configName.split('/').includes('..'))throw Error('Invalid archive config path');
const config=cp.execFileSync('tar',['-xOf','image.tar.partial',configName],{maxBuffer:4*1024*1024});
const image=JSON.parse(config);
if(image.architecture!==arch||image.os!=='linux')throw Error('Built image architecture does not match compute-node architecture');
const imageId='sha256:'+crypto.createHash('sha256').update(config).digest('hex');
fs.writeFileSync('image-lock.json',JSON.stringify({requestedImage:base,resolvedImage:digest,nodeImage:node,imageId,architecture:arch,dockerfileSHA256:crypto.createHash('sha256').update(fs.readFileSync('Dockerfile')).digest('hex')},null,2)+'\n');
JS
mv image.tar.partial image.tar
chmod 600 image.tar image-lock.json Dockerfile
