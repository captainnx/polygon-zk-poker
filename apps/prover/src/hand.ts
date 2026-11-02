import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {performance} from 'node:perf_hooks';
import {fileURLToPath} from 'node:url';
import {spawn} from 'node:child_process';
import assert from 'node:assert/strict';
import {Noir,type CompiledCircuit,type InputMap} from '@noir-lang/noir_js';
import {buildInput,fields,type Audit} from './hand-input.ts';
import {monitorMemory} from './resources.ts';
import {writeManifest,verifyManifest} from './manifest.ts';

const circuitPath=fileURLToPath(new URL('../hand/target/poker_hand_v3.json',import.meta.url));
const compiled:CompiledCircuit=JSON.parse(await readFile(circuitPath,'utf8'));
const command=process.argv[2]??'check';const out=new URL('../generated/hand/',import.meta.url);await mkdir(out,{recursive:true});
const outputPath=fileURLToPath(out);const binary=fileURLToPath(new URL('../../../.tools/bb',import.meta.url));
// Native CLI explicitly selects UltraHonk+Keccak. --disable_zk is NEVER passed;
// the pinned backend enables ZK by default. CLI avoids broken IPC in bb.js 3.0.
async function native(args:string[]) {
 await new Promise<void>((resolve,reject)=>{const p=spawn(binary,args,{stdio:'inherit',env:{...process.env,HARDWARE_CONCURRENCY:process.env.PROVER_THREADS??'2'}});p.once('error',reject);p.once('exit',(code,signal)=>code===0?resolve():reject(new Error(`Barretenberg ${args[0]} failed: ${code??signal}`)));});
}
const common=['-s','ultra_honk'];const oracle=['--oracle_hash','keccak'];
async function solidity(){await native(['write_solidity_verifier',...common,'-k',`${outputPath}/vk`,'-o',`${outputPath}/HonkVerifier.sol`,'--optimized']);}
if(command==='stats')await native(['gates',...common,...oracle,'-b',circuitPath]);
else if(command==='prepare') {
 const memory=monitorMemory();try{await native(['write_vk',...common,...oracle,'-b',circuitPath,'-o',outputPath,'-v']);await solidity();await writeManifest(new URL('../../../',import.meta.url));console.log('Full-hand verification key and Solidity verifier generated.');}finally{console.log(JSON.stringify(await memory.stop()));}
} else {
 const path=process.argv[3]??new URL('../../../reports/signed-hand.json',import.meta.url);
 const audit:Audit=JSON.parse(await readFile(path,'utf8'));const input=await buildInput(audit);const noir=new Noir(compiled);const started=performance.now();
 console.log('Executing complete signed-hand witness...');const {witness}=await noir.execute(input as unknown as InputMap);
 await writeFile(new URL('input.json',out),JSON.stringify(input));await writeFile(new URL('witness.gz',out),witness);
 if(command==='check') {
  const mutations:Record<string,(x:typeof input)=>void>={
   actionSignature:x=>{x.moves[0]!.signature[0]=x.moves[0]!.signature[0]!^1;},
   sessionPossessionSignature:x=>{x.authorizations[0]!.signature[0]=x.authorizations[0]!.signature[0]!^1;},
   identityIssuerSignature:x=>{x.authorizations[0]!.issuer_signature[0]=x.authorizations[0]!.issuer_signature[0]!^1;},
   identityClaim:x=>{x.statement[4]=String(BigInt(x.statement[4]!)+1n);},
   consentSignature:x=>{x.consent_signatures[0]![0]=x.consent_signatures[0]![0]!^1;},
   deck:x=>{[x.deck[0],x.deck[1]]=[x.deck[1]!,x.deck[0]!];},
   illegalBet:x=>{x.moves[0]!.kind=4;x.moves[0]!.amount='1';},
   playerSeed:x=>{x.player_seeds[0]![0]=x.player_seeds[0]![0]!^1;},
   balancedSidePotOutput:x=>{const donor=x.statement.findIndex((v,i)=>i>=31&&i<=39&&BigInt(v)>0n);const recipient=31+(donor-31+1)%9;x.statement[donor]=String(BigInt(x.statement[donor]!)-1n);x.statement[recipient]=String(BigInt(x.statement[recipient]!)+1n);},
  };
  for(const [label,mutate]of Object.entries(mutations)){const changed=structuredClone(input);mutate(changed);await assert.rejects(noir.execute(changed as unknown as InputMap),`${label} accepted`);}
  console.log(JSON.stringify({completeWitness:true,negativeCases:Object.keys(mutations),allTamperingRejected:true,witnessMs:performance.now()-started},null,2));
 } else if(command==='prove') {
  await verifyManifest(new URL('../../../',import.meta.url));
  const memory=monitorMemory();try {
   console.log('Generating complete UltraHonk ZK proof...');const before=performance.now();
   await native(['prove',...common,...oracle,'-b',circuitPath,'-w',`${outputPath}/witness.gz`,'-o',outputPath,'-k',`${outputPath}/vk`,'--slow_low_memory','--storage_budget','8g','--verify','--bench_out',fileURLToPath(new URL('../../../reports/full-hand-operations.json',import.meta.url))]);
   const proverMs=performance.now()-before;
   await native(['verify',...common,...oracle,'-k',`${outputPath}/vk`,'-p',`${outputPath}/proof`,'-i',`${outputPath}/public_inputs`]);
   const raw=await readFile(new URL('proof',out));const pub=await readFile(new URL('public_inputs',out));assert.equal(pub.length,44*32);
   const inputs=Array.from({length:44},(_,i)=>`0x${pub.subarray(i*32,(i+1)*32).toString('hex')}`);assert.deepEqual(inputs,fields(audit));
   await writeFile(new URL('proof.json',out),JSON.stringify({scope:'signed-hand-v3',proof:`0x${raw.toString('hex')}`,publicInputs:inputs},null,2));await solidity();
   const report={scope:'signed-hand-v3',scheme:'UltraHonk',zeroKnowledge:true,oracle:'keccak',backend:'native-cli',capacity:64,players:audit.spec.config.stacks.filter(x=>x>0).length,actions:audit.moves.length,proofBytes:raw.length,proverMs,totalMs:performance.now()-started,nodePeakRSSBytes:process.resourceUsage().maxRSS*1024,...await memory.stop(),verified:true};
   await writeFile(new URL('../../../reports/full-hand-proof.json',import.meta.url),JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
  }finally{await memory.stop();}
 } else throw new Error('usage: hand.ts stats|prepare|check|prove [AUDIT.json]');
}
