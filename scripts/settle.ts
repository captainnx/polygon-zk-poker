import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
import {resolve as resolvePath} from 'node:path';
import {createPublicClient,createWalletClient,defineChain,http,parseAbi,encodeFunctionData,decodeEventLog,type Hex,type Address} from 'viem';
import {privateKeyToAccount} from 'viem/accounts';
import {verifyManifest} from '../apps/prover/src/manifest.ts';
import {fields,resultCommit,type Audit} from '../apps/prover/src/hand-input.ts';
const root=new URL('../',import.meta.url);const cwd=fileURLToPath(root);
const network=process.argv[2];const auditPath=process.argv[3];
if(!network||!['local','amoy','polygon'].includes(network)||!auditPath)throw new Error('usage: settle.ts local|amoy|polygon AUDIT.json [PROOF.json] [--check-tampering]');
const manifest=await verifyManifest(root);
if(network==='polygon'){
 if(process.env.ALLOW_MAINNET_DEPLOYMENT!=='true')throw new Error('mainnet explicitly disabled');
 const amoy=JSON.parse(await readFile(new URL('deployments/v3/amoy.json',root),'utf8')) as {verified:boolean;verificationKeySha256:string};
 assert.ok(amoy.verified&&amoy.verificationKeySha256===manifest.verificationKeySha256,'matching Amoy verification required');
}
const chainId=network==='local'?31337:network==='amoy'?80002:137;
const rpc=network==='local'?(process.env.LOCAL_RPC_URL??'http://127.0.0.1:8545'):process.env[network==='amoy'?'AMOY_RPC_URL':'POLYGON_RPC_URL'];if(!rpc)throw new Error('RPC configuration missing');
const deployment=JSON.parse(await readFile(new URL(`deployments/v3/${network}.json`,root),'utf8')) as {chainId:number;verificationKeySha256:string;addresses:Record<string,Address>};assert.equal(deployment.chainId,chainId);assert.equal(deployment.verificationKeySha256,manifest.verificationKeySha256);
const registry=deployment.addresses['PokerHandRegistry.sol:PokerHandRegistry'];assert.ok(registry);
const audit:Audit=JSON.parse(await readFile(auditPath,'utf8'));assert.equal(audit.spec.domain.chainId,chainId);assert.equal(audit.spec.domain.registry.toLowerCase(),registry.toLowerCase());
const proofPath=process.argv[4]&&!process.argv[4].startsWith('--')?process.argv[4]:fileURLToPath(new URL('apps/prover/generated/hand/proof.json',root));
const proof=JSON.parse(await readFile(proofPath,'utf8')) as {scope:string;proof:Hex;publicInputs:Hex[]};assert.equal(proof.scope,'signed-hand-v3');assert.deepEqual(proof.publicInputs,fields(audit));
// An independent executable verifies all signatures and replay before any send.
await new Promise<void>((resolve,reject)=>{const p=spawn('go',['run','./apps/server/cmd/verify-signed',resolvePath(auditPath)],{cwd,stdio:'inherit'});p.once('error',reject);p.once('exit',c=>c===0?resolve():reject(new Error('independent audit rejected')));});
const chain=defineChain({id:chainId,name:`Poker ${network}`,nativeCurrency:{name:'POL',symbol:'POL',decimals:18},rpcUrls:{default:{http:[rpc]}}});
const client=createPublicClient({chain,transport:http(),pollingInterval:500});assert.equal(await client.getChainId(),chainId);
const abi=parseAbi([
 'function verifyHand(bytes32 handId,bytes32[9] players,uint64[9] initialStacks,uint64[9] finalStacks,bytes32 protocolCommitment,uint64 validUntil,bytes proof)',
 'function statement(bytes32 handId,bytes32[9] players,uint64[9] initialStacks,uint64[9] finalStacks,bytes32 protocolCommitment,uint64 validUntil) view returns (bytes32[])',
 'function getHand(bytes32 handId) view returns ((bytes32[9] players,uint64[9] initialStacks,uint64[9] finalStacks,bytes32 protocolCommitment,bytes32 resultDigest,uint64 validUntil,bool exists))',
 'event HandVerified(bytes32 indexed handId,bytes32 indexed resultDigest,bytes32 protocolCommitment)',
]);
const spec=audit.spec;const initial=spec.config.stacks.map(BigInt) as unknown as readonly [bigint,bigint,bigint,bigint,bigint,bigint,bigint,bigint,bigint];const final=audit.finalStacks.map(BigInt) as unknown as typeof initial;const players=spec.players as unknown as readonly [Hex,Hex,Hex,Hex,Hex,Hex,Hex,Hex,Hex];
const args=[spec.handId,players,initial,final,resultCommit(audit),BigInt(spec.validUntil)] as const;
const statement=await client.readContract({address:registry,abi,functionName:'statement',args});assert.deepEqual(statement,proof.publicInputs);
const read=()=>client.readContract({address:registry,abi,functionName:'getHand',args:[spec.handId]});
function checkStored(stored:Awaited<ReturnType<typeof read>>){assert.ok(stored.exists);assert.deepEqual(stored.initialStacks,initial);assert.deepEqual(stored.finalStacks,final);assert.deepEqual(stored.players.map(x=>x.toLowerCase()),players.map(x=>x.toLowerCase()));assert.equal(stored.protocolCommitment,resultCommit(audit));assert.equal(stored.validUntil,BigInt(spec.validUntil));}
const stored=await read();if(stored.exists){checkStored(stored);console.log(JSON.stringify({verified:true,alreadyRecorded:true,chainId,registry,handId:spec.handId}));process.exit(0);}
assert.ok(BigInt(spec.validUntil)>BigInt(Math.floor(Date.now()/1000)),'hand deadline expired; cannot settle it under a new ID');
await client.simulateContract({address:registry,abi,functionName:'verifyHand',args:[...args,proof.proof]});
if(process.argv.includes('--check-tampering')){
 const bytes=Buffer.from(proof.proof.slice(2),'hex');bytes[Math.floor(bytes.length/2)]!^=1;
 await assert.rejects(client.simulateContract({address:registry,abi,functionName:'verifyHand',args:[...args,`0x${bytes.toString('hex')}`]}));
 const altered=[...final];const donor=altered.findIndex(x=>x>0n);altered[donor]=altered[donor]!-1n;const receiver=(donor+1)%9;altered[receiver]=altered[receiver]!+1n;
 await assert.rejects(client.simulateContract({address:registry,abi,functionName:'verifyHand',args:[spec.handId,players,initial,altered as unknown as typeof final,args[4],args[5],proof.proof]}));
}
let tx:Hex;
if(network==='local'){
 // Public Anvil development key; never used for remote networks.
 const account=privateKeyToAccount('0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80');
 tx=await createWalletClient({account,chain,transport:http()}).writeContract({address:registry,abi,functionName:'verifyHand',args:[...args,proof.proof]});
}else{
 const account=process.env.FOUNDRY_ACCOUNT;if(!account)throw new Error('FOUNDRY_ACCOUNT must name an encrypted keystore');const data=encodeFunctionData({abi,functionName:'verifyHand',args:[...args,proof.proof]});
 const options=['send',registry,'--data',data,'--rpc-url',rpc,'--account',account,'--json'];if(process.env.FOUNDRY_PASSWORD_FILE)options.push('--password-file',process.env.FOUNDRY_PASSWORD_FILE);
 let stdout='';await new Promise<void>((resolve,reject)=>{const p=spawn(`${cwd}node_modules/.bin/cast`,options,{stdio:['inherit','pipe','inherit']});p.stdout.on('data',b=>{stdout+=String(b);});p.once('error',reject);p.once('exit',c=>c===0?resolve():reject(new Error('keystore transaction failed')));});tx=(JSON.parse(stdout) as {transactionHash:Hex}).transactionHash;
}
await mkdir(new URL('deployments/v3/',root),{recursive:true});const reportFile=new URL(`deployments/v3/${network}-hand-${spec.handId}.json`,root);
await writeFile(reportFile,JSON.stringify({chainId,registry,handId:spec.handId,transactionHash:tx,pending:true},null,2));
const receipt=await client.waitForTransactionReceipt({hash:tx,timeout:120000});assert.equal(receipt.status,'success');checkStored(await read());
const events=receipt.logs.filter(x=>x.address.toLowerCase()===registry.toLowerCase()).map(x=>decodeEventLog({abi,data:x.data,topics:x.topics}));assert.equal(events.length,1);assert.equal(events[0]!.eventName,'HandVerified');
const report={verified:true,chainId,registry,handId:spec.handId,transactionHash:tx,gas:receipt.gasUsed.toString(),handVerifiedEvent:true,tamperedProofAndBalancedOutputRejected:process.argv.includes('--check-tampering'),productionReady:false};
await writeFile(reportFile,JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
