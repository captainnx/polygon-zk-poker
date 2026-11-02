import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {recoverAddress,type Hex} from 'viem';
import {contextHash,initialState,actionDigest,verifyReceipt,ZERO,type Receipt} from './v3.ts';
import {type Audit} from '../../../apps/prover/src/hand-input.ts';
const audit:Audit=JSON.parse(await readFile(new URL('../../../fixtures/signed-hand-v3.json',import.meta.url),'utf8'));
const receipts:Receipt[]=JSON.parse(await readFile(new URL('../../../fixtures/signed-receipts-v3.json',import.meta.url),'utf8'));
test('v3 Go/Noir fixture matches every SDK action digest and session signature',async()=>{
 let previous=initialState(contextHash(audit.spec),audit.manifest,audit.deckCommit);
 for(const m of audit.moves){assert.equal(m.previousState,previous);const digest=actionDigest(audit.spec.sessions[m.action.seat]!,audit.spec.handId,previous,m.sequence,m.action,m.publicView,m.holeCommit);assert.equal(digest,m.state);const signer=await recoverAddress({hash:digest,signature:m.signature});assert.equal(signer.toLowerCase(),audit.authorizations[m.action.seat]!.authorization.sessionKey.toLowerCase());previous=digest;}
 assert.equal(previous,audit.state);
});
test('v3 observation commitments cannot be substituted under a valid action signature',async()=>{
 const m=audit.moves[0]!;const badView=`0x${'ff'.repeat(32)}` as Hex;
 for(const [view,hole]of [[badView,m.holeCommit],[m.publicView,badView]] as const){const digest=actionDigest(audit.spec.sessions[m.action.seat]!,audit.spec.handId,m.previousState,m.sequence,m.action,view,hole);assert.notEqual(digest,m.state);assert.notEqual((await recoverAddress({hash:digest,signature:m.signature})).toLowerCase(),audit.authorizations[m.action.seat]!.authorization.sessionKey.toLowerCase());}
});
test('v3 receipt chain retains byte identity and rejects forks and payload tampering',async()=>{
 let previous=ZERO;const server=receipts[0]!.server;
 for(let i=0;i<receipts.length;i++){await verifyReceipt(receipts[i]!,server,previous,BigInt(i));previous=receipts[i]!.digest;}
 const r=receipts[1]!;await assert.rejects(verifyReceipt({...r,payload:btoa('{}')},server,r.previous,BigInt(r.index)));
 await assert.rejects(verifyReceipt(r,server,ZERO,BigInt(r.index)));
});

test('observation signing rejects padding aliases and fractional card IDs',async()=>{
 const {publicView,signedMove}=await import('./v3.ts');const {createSessionKey}=await import('./index.ts');
 const p={spec:audit.spec,stage:'playing',state:ZERO,hand:{actor:0,street:'preflop',currentBet:10,minRaise:10,board:[] as number[],sequence:0,players:Array.from({length:9},()=>({stack:100,streetBet:0,folded:false,occupied:true}))}} as unknown as import('./v3.ts').Public;
 assert.match(publicView(p),/^0x[0-9a-f]{64}$/);
 const alias=structuredClone(p);alias.hand!.board=[0,0,0];assert.throws(()=>publicView(alias),/observation shape/);
 const fractional=structuredClone(p);fractional.hand!.street='flop';fractional.hand!.board=[1.5,2,3];assert.throws(()=>publicView(fractional),/observation shape/);
 const key=createSessionKey();await assert.rejects(signedMove(key,p,{seat:0,kind:'fold',amount:0},ZERO,[1.5,2]),/private cards/);
});
