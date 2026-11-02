import {writeFile,readFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
import {Noir,type CompiledCircuit,type InputMap} from '@noir-lang/noir_js';
import {recoverAddress,hexToBytes} from 'viem';
import {runSignedNine} from './signed-nine.ts';
import {buildInput,type Audit} from '../apps/prover/src/hand-input.ts';
const replay=process.argv.includes('--fixture');
const saved=replay?JSON.parse(await readFile('fixtures/adversarial-signed-hand-v3.json','utf8')):undefined;
const result=replay?{audit:saved.audit as Audit,adversarialMoves:saved.cases as Awaited<ReturnType<typeof runSignedNine>>['adversarialMoves']}:await runSignedNine(process.env.API_URL??'http://127.0.0.1:8080','0x1234567890123456789012345678901234567890',31337,undefined,'streets');
const audit=result.audit as unknown as Audit;
const noir=new Noir(JSON.parse(await readFile('apps/prover/hand/target/poker_hand_v3.json','utf8')) as CompiledCircuit);
const input=await buildInput(audit);await noir.execute(input as unknown as InputMap);
for(const candidate of result.adversarialMoves){
 const m=candidate.move;
 assert.equal((await recoverAddress({hash:m.state,signature:m.signature})).toLowerCase(),audit.authorizations[m.action.seat]!.authorization.sessionKey.toLowerCase());
 const changed=structuredClone(input);changed.moves[Number(m.sequence)-1]={seat:m.action.seat,kind:m.action.kind==='check'?1:m.action.kind==='raise'?4:m.action.kind==='call'?2:5,amount:String(m.action.amount),signature:Array.from(hexToBytes(m.signature)).slice(0,64)};
 await assert.rejects(noir.execute(changed as unknown as InputMap),new RegExp(candidate.expected),candidate.name);
 console.log(`Correct session signature; Noir rejects: ${candidate.name} (${candidate.expected})`);
}
if(!replay)await writeFile('fixtures/adversarial-signed-hand-v3.json',JSON.stringify({audit,cases:result.adversarialMoves},null,2));
await writeFile(replay?'reports/signed-adversarial-fixture.json':'reports/signed-adversarial.json',JSON.stringify({validSessionSignatures:true,liveServerTest:!replay,serverRejectedAll:!replay,noirRejectedAtExpectedConstraint:true,cases:result.adversarialMoves.map(x=>x.name)},null,2));

await writeFile("reports/streets-signed-hand.json",JSON.stringify(audit,null,2));
