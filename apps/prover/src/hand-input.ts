import { concatHex, hexToBytes, recoverPublicKey, sha256, stringToHex, toHex, type Address, type Hex } from 'viem';
import { contextHash, initialState, seedCommit, manifestHash, consentDigest, actionDigest, type Spec, type V3Session, type Move } from '../../../packages/sdk/src/v3.ts';
export type Audit={version:string;spec:Spec;authorizations:V3Session[];serverSeed:Hex;serverCommit:Hex;commits:{value:Hex;signature:Hex}[];consents:Hex[];reveals:{value:Hex;signature:Hex}[];manifest:Hex;deckCommit:Hex;state:Hex;moves:Move[];finalStacks:number[];stage:string};
const zero=(n:number)=>Array(n).fill(0) as number[];
const bytes=(h:Hex)=>Array.from(hexToBytes(h));
const sig=(h:Hex)=>{const b=bytes(h);if(b.length!==65||(b[64]!==27&&b[64]!==28&&b[64]!==0&&b[64]!==1))throw new Error('invalid 65-byte signature');return b.slice(0,64);};
async function key(hash:Hex,signature:Hex) {const publicKey=bytes(await recoverPublicKey({hash,signature}));return {x:publicKey.slice(1,33),y:publicKey.slice(33,65)};}
const hash=(...parts:Hex[])=>sha256(concatHex(parts));
export const resultCommit=(a:Audit)=>hash(stringToHex('ZKPOKER_RESULT_V3'),a.manifest,a.deckCommit,a.state);
export function shuffle(a:Audit) {
 const context=contextHash(a.spec);const parts:Hex[]=[stringToHex('ZKPOKER_MIX_V3'),context,a.serverSeed];
 for(let i=0;i<9;i++)if(a.spec.config.stacks[i]!>0)parts.push(toHex(i,{size:1}),a.reveals[i]!.value);
 const mixed=hash(...parts);
 const cards=Array.from({length:52},(_,id)=>({id,priority:hash(stringToHex('ZKPOKER_CARD_V3'),mixed,toHex(id,{size:1}))}));
 cards.sort((a,b)=>a.priority<b.priority?-1:a.priority>b.priority?1:a.id-b.id);return cards.map(x=>x.id);
}
export function statement(a:Audit) {
 const hand=BigInt(a.spec.handId);const commit=BigInt(resultCommit(a));const mask=(1n<<128n)-1n;
 return [BigInt(a.spec.domain.chainId),BigInt(a.spec.domain.registry),hand>>128n,hand&mask,...a.spec.players.flatMap(x=>[BigInt(x)>>128n,BigInt(x)&mask]),...a.spec.config.stacks.map(x=>BigInt(x)),...a.finalStacks.map(x=>BigInt(x)),commit>>128n,commit&mask,BigInt(a.spec.validUntil),BigInt(a.spec.authIssuer)].map(x=>x.toString());
}
export async function buildInput(a:Audit) {
 if(a.version!=='signed-hand-v3'||a.stage!=='complete'||a.moves.length>64)throw new Error('unsupported or incomplete hand');
 const ctx=contextHash(a.spec);if(seedCommit(ctx,255,a.serverSeed)!==a.serverCommit)throw new Error('server commitment mismatch');
 for(let i=0;i<9;i++)if(a.spec.config.stacks[i]!>0&&seedCommit(ctx,i,a.reveals[i]!.value)!==a.commits[i]!.value)throw new Error('player commitment mismatch');
 if(manifestHash(ctx,a.serverCommit,a.commits.map(x=>x.value))!==a.manifest)throw new Error('manifest mismatch');
 const deck=shuffle(a);const computed=hash(stringToHex('ZKPOKER_DECK_V3'),ctx,toHex(Uint8Array.from(deck)));if(computed!==a.deckCommit)throw new Error('deck commitment mismatch');
 const authorizations=[];const consents=[];const playerSeeds=[];
 for(let i=0;i<9;i++) {
  if(a.spec.config.stacks[i]===0) {authorizations.push({account_id:zero(32),session:{x:zero(32),y:zero(32)},signature:zero(64),issuer_signature:zero(64),kind:0,nonce:'0',expiry:'0'});consents.push(zero(64));playerSeeds.push(zero(32));continue;}
  const s=a.authorizations[i]!;const auth=s.authorization;
  authorizations.push({account_id:bytes(auth.accountId),session:await key(consentDigest(s.id,a.spec.handId,a.manifest),a.consents[i]!),signature:sig(s.signature),issuer_signature:sig(s.issuerSignature),kind:auth.kind,nonce:auth.nonce,expiry:auth.expiresAt});
  consents.push(sig(a.consents[i]!));playerSeeds.push(bytes(a.reveals[i]!.value));
 }
 let chain=initialState(ctx,a.manifest,a.deckCommit);const moves=[];const kinds={fold:0,check:1,call:2,bet:3,raise:4,'all-in':5};
 for(let i=0;i<a.moves.length;i++) {
  const m=a.moves[i]!;if(m.previousState!==chain||BigInt(m.sequence)!==BigInt(i+1))throw new Error('action chain mismatch');
  chain=actionDigest(a.spec.sessions[m.action.seat]!,a.spec.handId,chain,m.sequence,m.action,m.publicView,m.holeCommit);if(chain!==m.state)throw new Error('action digest mismatch');
  moves.push({seat:m.action.seat,kind:kinds[m.action.kind],amount:String(m.action.amount),signature:sig(m.signature)});
 }
 if(chain!==a.state)throw new Error('terminal state mismatch');while(moves.length<64)moves.push({seat:0,kind:0,amount:'0',signature:zero(64)});
 const occupiedSeat=a.spec.config.stacks.findIndex(v=>v>0);const first=a.authorizations[occupiedSeat]!;
 return {statement:statement(a),issuer_key:await key(first.id,first.issuerSignature),table:bytes(a.spec.tableId),button:a.spec.config.button,small:String(a.spec.config.smallBlind),big:String(a.spec.config.bigBlind),ante:String(a.spec.config.ante),action_limit:a.spec.actionLimit,authorizations,server_seed:bytes(a.serverSeed),player_seeds:playerSeeds,consent_signatures:consents,deck,action_count:a.moves.length,moves};
}
export function fields(a:Audit):Hex[]{return statement(a).map(x=>toHex(BigInt(x),{size:32}));}
