import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const run=promisify(execFile);
export function monitorMemory() {
 let peak=0;let busy=false;const started=Date.now();
 async function sample(){if(busy)return;busy=true;try{const {stdout}=await run('ps',['-axo','pid=,ppid=,rss=']);const rows=stdout.trim().split('\n').map(line=>line.trim().split(/\s+/).map(Number));const pids=new Set([process.pid]);let changed=true;while(changed){changed=false;for(const [pid,parent]of rows)if(pid!==undefined&&parent!==undefined&&pids.has(parent)&&!pids.has(pid)){pids.add(pid);changed=true;}}let sum=0;for(const [pid,,rss]of rows)if(pid!==undefined&&rss!==undefined&&pids.has(pid))sum+=rss*1024;peak=Math.max(peak,sum);}catch{/* memory telemetry failure must not alter proof semantics */}finally{busy=false;}}
 const timer=setInterval(()=>{void sample();},1000);timer.unref();void sample();
 return {async stop(){clearInterval(timer);await sample();return {processTreePeakRSSBytes:peak,sampleIntervalMs:1000,elapsedMs:Date.now()-started};}};
}
