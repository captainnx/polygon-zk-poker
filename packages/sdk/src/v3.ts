import { concatHex, hexToBytes, sha256, stringToHex, toHex, recoverAddress, type Address, type Hex } from 'viem';
import { type PrivateKeyAccount } from 'viem/accounts';


export const ZERO = `0x${'00'.repeat(32)}` as Hex;
export const ZERO_ADDRESS = `0x${'00'.repeat(20)}` as Address;
export type Config = {stacks:number[];button:number;smallBlind:number;bigBlind:number;ante:number};
export type V3Session = {domain:{chainId:number;registry:Address};authorization:{playerId:Hex;accountId:Hex;issuer:Address;kind:0|1;sessionKey:Address;tableId:Hex;nonce:string;expiresAt:string};signature:Hex;issuerSignature:Hex;id:Hex;revoked:boolean};
export type Spec = {domain:{chainId:number;registry:Address};authIssuer:Address;tableId:Hex;handId:Hex;config:Config;players:Hex[];sessions:Hex[];validUntil:string;actionLimit:number};
export type Action = {seat:number;kind:'fold'|'check'|'call'|'bet'|'raise'|'all-in';amount:number};
export type Move = {action:Action;sequence:string;previousState:Hex;signature:Hex;state:Hex;publicView:Hex;holeCommit:Hex};
export type Receipt = {handId:Hex;index:string;kind:string;payload:string;previous:Hex;at:string;server:Address;digest:Hex;signature:Hex};
export type Public = {spec:Spec;server:Address;serverCommit:Hex;commits:Hex[];consented:boolean[];revealed:boolean[];manifest:Hex;deckCommit:Hex;state:Hex;stage:string;abortReason?:string;version:string;lastReceipt:Hex;toCall?:number;canRaise?:boolean;minTarget?:number;hand?:{actor:number;street:string;currentBet:number;minRaise:number;board:number[];players:{stack:number;streetBet:number;folded:boolean;occupied:boolean}[];sequence:number}};
export type Result = {public:Public;receipt:Receipt};
const u64=(v:number|string|bigint)=>toHex(BigInt(v),{size:8});
const hash=(...v:Hex[])=>sha256(concatHex(v));
const label=(value:string)=>stringToHex(`ZKPOKER_${value}_V3`);
const code={fold:0,check:1,call:2,bet:3,raise:4,'all-in':5};
export function contextHash(s:Spec):Hex {
  if(s.config.stacks.length!==9||s.players.length!==9||s.sessions.length!==9)throw new Error('nine-slot arrays required');
  return hash(label('CONTEXT'),u64(s.domain.chainId),s.domain.registry,s.authIssuer,s.tableId,s.handId,...s.config.stacks.map(u64),toHex(s.config.button,{size:1}),u64(s.config.smallBlind),u64(s.config.bigBlind),u64(s.config.ante),...s.players,...s.sessions,u64(s.validUntil),toHex(s.actionLimit,{size:2}));
}
export const seedCommit=(context:Hex,seat:number,seed:Hex)=>hash(label('SEED'),context,toHex(seat,{size:1}),seed);
export const manifestHash=(context:Hex,server:Hex,commits:Hex[])=>hash(label('MANIFEST'),context,server,...commits);
export const consentDigest=(session:Hex,hand:Hex,manifest:Hex)=>hash(label('CONSENT'),session,hand,manifest);
export const valueDigest=(kind:'COMMIT'|'REVEAL'|'READ',session:Hex,hand:Hex,context:Hex,value:Hex)=>hash(label(kind),session,hand,context,value);
export const initialState=(context:Hex,manifest:Hex,deck:Hex)=>hash(label('INITIAL'),context,manifest,deck);
export function actionDigest(session:Hex,hand:Hex,previous:Hex,sequence:string,action:Action,publicView:Hex,holeCommit:Hex):Hex {
  if(!Number.isInteger(action.seat)||action.seat<0||action.seat>8||BigInt(sequence)<1n||!(action.kind in code)||!Number.isSafeInteger(action.amount)||action.amount<0||action.amount>1e12||(action.kind!=='bet'&&action.kind!=='raise'&&action.amount!==0))throw new Error('invalid v3 action');
  return hash(label('ACTION'),session,hand,previous,u64(sequence),toHex(action.seat,{size:1}),toHex(code[action.kind],{size:1}),u64(action.amount),publicView,holeCommit);
}
export function publicView(p:Public):Hex {
 const h=p.hand;if(!h||p.stage!=='playing')throw new Error('no active public view');
 const counts:Record<string,number>={preflop:0,flop:3,turn:4,river:5};
 if(!Array.isArray(h.players)||!Array.isArray(h.board)||h.players.length!==9||h.board.length!==counts[h.street]||new Set(h.board).size!==h.board.length||h.board.some(x=>!Number.isInteger(x)||x<0||x>51)||!Number.isInteger(h.actor)||h.actor<0||h.actor>8||!Number.isInteger(h.sequence)||h.sequence<0||h.sequence>=64)throw new Error('invalid public observation shape');
 if([...h.players.flatMap(v=>[v.stack,v.streetBet]),h.currentBet,h.minRaise].some(x=>!Number.isSafeInteger(x)||x<0||x>1e12)||h.minRaise===0||h.players.some(v=>typeof v.folded!=='boolean'))throw new Error('invalid public observation amounts');
 const words:Hex[]=[];let folded=0;h.players.forEach((v,i)=>{words.push(toHex(BigInt(v.stack),{size:5}),toHex(BigInt(v.streetBet),{size:5}));if(v.folded)folded|=1<<i;});
 const streets:Record<string,number>={preflop:0,flop:1,turn:2,river:3};const street=streets[h.street];if(street===undefined)throw new Error('invalid street');
 const board=Array.from({length:5},(_,i)=>h.board?.[i]??0);
 return hash(stringToHex('ZKPVIEW3'),...words,toHex(folded,{size:2}),toHex(BigInt(h.currentBet),{size:5}),toHex(BigInt(h.minRaise),{size:5}),toHex(Uint8Array.from(board)),toHex(street,{size:1}),toHex(h.actor,{size:1}),toHex(h.sequence,{size:1}));
}
export const privateView=(ctx:Hex,seat:number,seed:Hex,hole:number[])=>hash(label('HOLE'),ctx,toHex(seat,{size:1}),seed,toHex(Uint8Array.from(hole)));
export type Principal={identityId:Hex;kind:0|1;username?:string};
export type Request=<T>(path:string,body:unknown)=>Promise<T>;
export function sessionId(s:V3Session):Hex {
 const a=s.authorization;
 return hash(label('SESSION'),u64(s.domain.chainId),s.domain.registry,a.issuer,a.playerId,a.accountId,a.sessionKey,a.tableId,u64(a.nonce),u64(a.expiresAt),toHex(a.kind,{size:1}));
}
export const playerKeyId=(key:Hex)=>hash(label('PLAYER'),`0x${key.slice(4)}`);
export async function verifySession(s:V3Session,issuer:Address,now=BigInt(Math.floor(Date.now()/1000))):Promise<boolean>{
 try {const a=s.authorization;if(a.kind!==0&&a.kind!==1)return false;
 if(![31337,80002,137].includes(s.domain.chainId)||a.playerId===ZERO||a.accountId===ZERO||a.issuer.toLowerCase()!==issuer.toLowerCase()||BigInt(a.expiresAt)<=now||sessionId(s)!==s.id)return false;
 const {recoverPublicKey}=await import('viem');const pub=await recoverPublicKey({hash:s.id,signature:s.signature});if(playerKeyId(pub)!==a.playerId)return false;
 return (await recoverAddress({hash:s.id,signature:s.issuerSignature})).toLowerCase()===issuer.toLowerCase()&&(await recoverAddress({hash:s.id,signature:s.signature})).toLowerCase()===a.sessionKey.toLowerCase();
 }catch{return false;}
}
export async function createSession(key:PrivateKeyAccount,who:Principal,domain:V3Session['domain'],issuer:Address,tableId:Hex,nonce:bigint,expiresAt:bigint,request:Request):Promise<V3Session>{
 const issued=await request<V3Session>('/api/v3/sessions/issue',{domain,sessionKey:key.address,publicKey:key.publicKey,tableId,nonce:nonce.toString(),expiresAt:expiresAt.toString()});
 const a=issued.authorization;
 if(a.accountId!==who.identityId||a.kind!==who.kind||a.sessionKey.toLowerCase()!==key.address.toLowerCase()||a.tableId!==tableId||a.nonce!==nonce.toString()||a.expiresAt!==expiresAt.toString()||issued.domain.chainId!==domain.chainId||issued.domain.registry.toLowerCase()!==domain.registry.toLowerCase())throw new Error('issued certificate context mismatch');
 issued.signature=await key.sign({hash:sessionId(issued)});
 if(!await verifySession(issued,issuer))throw new Error('invalid identity certificate');
 const saved=await request<V3Session>('/api/v3/sessions',issued);if(saved.id!==issued.id||saved.revoked||!await verifySession(saved,issuer))throw new Error('session registration mismatch');return saved;
}
export const revokeDigest=(s:V3Session)=>hash(label('REVOKE'),u64(s.domain.chainId),s.domain.registry,s.id);
export async function signedValue(key:PrivateKeyAccount,kind:'COMMIT'|'REVEAL',sid:Hex,hand:Hex,context:Hex,value:Hex) {return {value,signature:await key.sign({hash:valueDigest(kind,sid,hand,context,value)})};}
export async function signedConsent(key:PrivateKeyAccount,sid:Hex,p:Public,expected:Spec,ownSeed:Hex) {
  const ctx=contextHash(expected);const seat=expected.sessions.findIndex(x=>x===sid);
  if(seat<0||contextHash(p.spec)!==ctx||p.commits[seat]!==seedCommit(ctx,seat,ownSeed))throw new Error('roster, rules or own commitment changed');
  const expectedManifest=manifestHash(contextHash(p.spec),p.serverCommit,p.commits);
  if(expectedManifest!==p.manifest||p.stage!=='consenting')throw new Error('unfrozen or inconsistent manifest');
  return {value:p.manifest,signature:await key.sign({hash:consentDigest(sid,p.spec.handId,p.manifest)})};
}
export async function signedMove(key:PrivateKeyAccount,p:Public,action:Action,seed:Hex,hole:number[]):Promise<Move> {
  if(p.stage!=='playing'||p.hand?.actor!==action.seat)throw new Error('not this seat’s turn');
  const sequence=String(p.hand.sequence+1);const sid=p.spec.sessions[action.seat];if(!sid)throw new Error('missing session');
  if(hole.length!==2||hole[0]===hole[1]||hole.some(x=>!Number.isInteger(x)||x<0||x>51))throw new Error('invalid private cards');
  const view=publicView(p);const holeCommit=privateView(contextHash(p.spec),action.seat,seed,hole);
  const state=actionDigest(sid,p.spec.handId,p.state,sequence,action,view,holeCommit);
  return {action,sequence,previousState:p.state,state,publicView:view,holeCommit,signature:await key.sign({hash:state})};
}
export async function readAuthorization(key:PrivateKeyAccount,p:Public,sid:Hex) {
  const timestamp=BigInt(Math.floor(Date.now()/1000));
  return {sessionId:sid,timestamp:timestamp.toString(),signature:await key.sign({hash:valueDigest('READ',sid,p.spec.handId,contextHash(p.spec),toHex(timestamp,{size:32}))})};
}
export async function verifyReceipt(receipt:Receipt,server:Address,previous:Hex,index:bigint):Promise<void> {
  const payload=toHex(Uint8Array.from(atob(receipt.payload),c=>c.charCodeAt(0)));
  const digest=hash(label('RECEIPT'),receipt.handId,u64(receipt.index),u64(receipt.at),receipt.server,receipt.previous,sha256(stringToHex(receipt.kind)),sha256(payload));
  if(receipt.server.toLowerCase()!==server.toLowerCase()||receipt.previous!==previous||BigInt(receipt.index)!==index||digest!==receipt.digest||(await recoverAddress({hash:digest,signature:receipt.signature})).toLowerCase()!==server.toLowerCase())throw new Error('receipt authentication, ordering or fork failure');
}
export const receiptPayload=(r:Receipt):unknown=>JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(r.payload),c=>c.charCodeAt(0))));
export async function verifyFrozenConsents(p:Public,receipts:Receipt[],sessions:V3Session[]) {
  for(let seat=0;seat<9;seat++) {
    if(p.spec.config.stacks[seat]===0)continue;
    const s=sessions[seat];if(!s||s.id!==p.spec.sessions[seat])throw new Error('missing peer identity certificate');
    const a=s.authorization;
    if(a.playerId!==p.spec.players[seat]||a.tableId!==p.spec.tableId||s.domain.chainId!==p.spec.domain.chainId||s.domain.registry.toLowerCase()!==p.spec.domain.registry.toLowerCase()||BigInt(a.expiresAt)<BigInt(p.spec.validUntil)||!await verifySession(s,p.spec.authIssuer))throw new Error('invalid peer identity certificate');
    let signature:Hex|undefined;
    for(const r of receipts)if(r.kind==='consent'&&r.handId===p.spec.handId) {
      const v=receiptPayload(r) as {seat:number;value:Hex;sessionId:Hex;signature:Hex};
      if(v.seat===seat&&v.value===p.manifest&&v.sessionId===s.id)signature=v.signature;
    }
    if(!signature||(await recoverAddress({hash:consentDigest(s.id,p.spec.handId,p.manifest),signature})).toLowerCase()!==a.sessionKey.toLowerCase())throw new Error('missing or forged player consent');
  }
}
export async function post<T>(base:string,path:string,body:unknown):Promise<T> {
  const response=await fetch(`${base}${path}`,{method:'POST',credentials:'include',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const result=await response.json();if(!response.ok)throw new Error(result.error??`HTTP ${response.status}`);return result as T;
}
export const rawBytes=(hex:Hex)=>hexToBytes(hex);
