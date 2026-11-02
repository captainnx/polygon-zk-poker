import {actor} from './actors.ts';
import { randomBytes } from 'node:crypto';
import { writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
import { toHex, type Address, type Hex } from 'viem';
import { generatePrivateKey, privateKeyToAccount } from 'viem/accounts';
import { createSession, contextHash, seedCommit, signedValue, signedConsent, signedMove, readAuthorization, verifyReceipt, verifyFrozenConsents, post, ZERO, type Spec, type Public, type Result, type Receipt } from '../packages/sdk/src/v3.ts';

export async function runSignedNine(base:string,registry:Address,chainId=31337,afterMove?:(moves:number)=>Promise<void>,strategy:'all-in'|'streets'='all-in') {
  const capabilities = await (await fetch(`${base}/api/v3/capabilities`)).json() as {server:Address;durable:boolean};
  const actors=Array.from({length:9},()=>actor(base));
 const keys=actors.map(x=>x.key);const principals=[];
  const seeds=Array.from({length:9},()=>toHex(randomBytes(32)));
  const tableId=toHex(randomBytes(32));const handId=toHex(randomBytes(32));const now=BigInt(Math.floor(Date.now()/1000));
  const sessions=[];
  for(let i=0;i<9;i++) {
    const who=await (i<5?actors[i]!.guest():actors[i]!.account());principals.push(who);
    const session=await createSession(keys[i]!,who,{chainId,registry},capabilities.server,tableId,0n,now+86400n,actors[i]!.request);
    sessions.push(session);
  }
  const spec:Spec={domain:{chainId,registry},authIssuer:capabilities.server,tableId,handId,config:{stacks:[1000,700,500,1000,350,1000,800,1000,600],button:0,smallBlind:5,bigBlind:10,ante:1},players:sessions.map(x=>x.authorization.playerId),sessions:sessions.map(x=>x.id),validUntil:(now+1800n).toString(),actionLimit:64};
  const receipts:Receipt[]=[];let p:Public;let previous=ZERO;
  async function accept(result:Result) {
    assert.equal(result.public.spec.handId,handId);
    await verifyReceipt(result.receipt,capabilities.server,previous,BigInt(receipts.length));
    receipts.push(result.receipt);previous=result.receipt.digest;p=result.public;
  }
  await accept(await post<Result>(base,'/api/v3/hands',spec));
  const ctx=contextHash(spec);const serverCommit=p!.serverCommit;
  for(let i=0;i<9;i++) {
    const value=seedCommit(ctx,i,seeds[i]!);
    await accept(await post<Result>(base,`/api/v3/hands/${handId}/commit`,{seat:i,...await signedValue(keys[i]!,'COMMIT',sessions[i]!.id,handId,ctx,value)}));
  }
  assert.equal(p!.serverCommit,serverCommit);assert.equal(p!.stage,'consenting');
  for(let i=0;i<9;i++)assert.equal(p!.commits[i],seedCommit(ctx,i,seeds[i]!));
  for(let i=0;i<9;i++)await accept(await post<Result>(base,`/api/v3/hands/${handId}/consent`,{seat:i,...await signedConsent(keys[i]!,sessions[i]!.id,p!,spec,seeds[i]!)}));
  assert.equal(p!.stage,'revealing');
  await verifyFrozenConsents(p!,receipts,sessions);
  for(let i=0;i<9;i++)await accept(await post<Result>(base,`/api/v3/hands/${handId}/reveal`,{seat:i,...await signedValue(keys[i]!,'REVEAL',sessions[i]!.id,handId,p!.manifest,seeds[i]!)}));
  assert.equal(p!.stage,'playing');
  let moves=0;let retriesChecked=false;let recoveryChecked=false;const adversarialMoves:{name:string;move:Awaited<ReturnType<typeof signedMove>>;expected:string}[]=[];
  while(p!.stage==='playing') {
    const seat=p!.hand!.actor;const action={seat,kind:strategy==='all-in'&&moves===0?'all-in' as const:(p!.hand!.currentBet>p!.hand!.players[seat]!.streetBet?'call' as const:'check' as const),amount:0};
    const privateRead=await post<{hole:number[]}>(base,`/api/v3/hands/${handId}/recover`,{...await readAuthorization(keys[seat]!,p!,sessions[seat]!.id),after:String(receipts.length),knownReceipt:previous});
    if(moves===0) {
      for(const candidate of [
        {name:'validSignatureIllegalCheck',public:p!,action:{seat,kind:'check' as const,amount:0},hole:privateRead.hole,expected:'check facing bet'},
        {name:'validSignatureShortRaise',public:p!,action:{seat,kind:'raise' as const,amount:11},hole:privateRead.hole,expected:'short raise must be all-in'},
        {name:'validSignatureWrongHole',public:p!,action,hole:[privateRead.hole[1]!,privateRead.hole[0]!],expected:'invalid signature'},
      ]) {
        const invalid=await signedMove(keys[seat]!,candidate.public,candidate.action,seeds[seat]!,candidate.hole);
        await assert.rejects(post(base,`/api/v3/hands/${handId}/action`,invalid));
        adversarialMoves.push({name:candidate.name,move:invalid,expected:candidate.expected});
      }
    }
    if(p!.hand!.street==='flop'&&!adversarialMoves.some(x=>x.name==='validSignatureWrongBoard')) {
      const falseBoard=structuredClone(p!);const replacement=Array.from({length:52},(_,i)=>i).find(x=>!p!.hand!.board.includes(x)&&!privateRead.hole.includes(x))!;
      falseBoard.hand!.board[0]=replacement;
      const invalid=await signedMove(keys[seat]!,falseBoard,action,seeds[seat]!,privateRead.hole);
      await assert.rejects(post(base,`/api/v3/hands/${handId}/action`,invalid));
      adversarialMoves.push({name:'validSignatureWrongBoard',move:invalid,expected:'invalid signature'});
    }
    const move=await signedMove(keys[seat]!,p!,action,seeds[seat]!,privateRead.hole);
    await accept(await post<Result>(base,`/api/v3/hands/${handId}/action`,move));moves++;
    if(moves===1) {
      const retried=await post<Result>(base,`/api/v3/hands/${handId}/action`,move);assert.equal(retried.receipt.digest,previous);retriesChecked=true;
      await assert.rejects(post(base,`/api/v3/hands/${handId}/action`,{...move,signature:`0x${'00'.repeat(65)}`}));
    }
    if(afterMove)await afterMove(moves);
    if(moves===3) {
      const read=await readAuthorization(keys[0]!,p!,sessions[0]!.id);
      const cursor=receipts.length-1;const recovered=await post<{public:Public;events:Receipt[];hole:number[]}>(base,`/api/v3/hands/${handId}/recover`,{...read,after:String(cursor),knownReceipt:receipts[cursor-1]!.digest});
      assert.equal(recovered.events.length,1);assert.equal(recovered.events[0]!.digest,previous);assert.equal(recovered.hole.length,2);assert.equal(recovered.public.state,p!.state);recoveryChecked=true;
    }
  }
  assert.equal(p!.stage,'complete');assert.equal(moves,strategy==='all-in'?9:36);
  const audit=await post<Record<string,unknown>>(base,`/api/v3/hands/${handId}/audit`,await readAuthorization(keys[0]!,p!,sessions[0]!.id));
  const finalStacks=audit.finalStacks as number[];assert.equal(finalStacks.reduce((a,b)=>a+b,0),6950);
  return {audit,receipts,adversarialMoves,report:{handId,moves,totalChips:6950,guests:5,accounts:4,walletRequired:false,durable:capabilities.durable,retriesChecked,recoveryChecked},public:p!};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href) {
  const base=process.env.API_URL??'http://127.0.0.1:8080';
  const registry=(process.env.REGISTRY_ADDRESS??'0x1234567890123456789012345678901234567890') as Address;
  const result=await runSignedNine(base,registry);
  await writeFile(new URL('../reports/signed-hand.json',import.meta.url),JSON.stringify(result.audit,null,2));
  await writeFile(new URL('../reports/signed-receipts.json',import.meta.url),JSON.stringify(result.receipts,null,2));
  console.log(JSON.stringify(result.report,null,2));
}
