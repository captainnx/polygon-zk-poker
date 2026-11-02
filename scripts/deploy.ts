import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {spawn} from 'node:child_process';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
import {createPublicClient,createWalletClient,defineChain,http,encodeDeployData,encodeFunctionData,keccak256,type Abi,type Hex,type Address} from 'viem';
import {privateKeyToAccount} from 'viem/accounts';
import {verifyManifest} from '../apps/prover/src/manifest.ts';
import {resultCommit,type Audit} from '../apps/prover/src/hand-input.ts';
import {runSignedNine} from './signed-nine.ts';

const root=new URL('../',import.meta.url);const cwd=fileURLToPath(root);
const network=process.argv[2]??'amoy';if(!['local','amoy','polygon'].includes(network))throw new Error('network must be local|amoy|polygon');
const chainId=network==='local'?31337:network==='amoy'?80002:137;
const rpc=network==='local'?(process.env.LOCAL_RPC_URL??'http://127.0.0.1:8545'):process.env[network==='amoy'?'AMOY_RPC_URL':'POLYGON_RPC_URL'];
if(!rpc)throw new Error(`${network.toUpperCase()}_RPC_URL is required; no deployment attempted`);
const manifest=await verifyManifest(root);
if(network==='polygon') {
 if(process.env.ALLOW_MAINNET_DEPLOYMENT!=='true')throw new Error('mainnet deployment requires explicit ALLOW_MAINNET_DEPLOYMENT=true');
 const amoy=JSON.parse(await readFile(new URL('deployments/v3/amoy.json',root),'utf8')) as {verified:boolean;verificationKeySha256:string};
 if(!amoy.verified||amoy.verificationKeySha256!==manifest.verificationKeySha256)throw new Error('matching real Amoy proof verification is required before mainnet');
}
const chain=defineChain({id:chainId,name:`Polygon ZK Poker ${network}`,nativeCurrency:{name:'POL',symbol:'POL',decimals:18},rpcUrls:{default:{http:[rpc]}}});
const client=createPublicClient({chain,transport:http(undefined,{retryCount:1}),pollingInterval:1000});assert.equal(await client.getChainId(),chainId,'RPC points to wrong chain');
const localAccount=privateKeyToAccount('0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80');
const localWallet=createWalletClient({account:localAccount,chain,transport:http()});
const keystore=process.env.FOUNDRY_ACCOUNT;const passwordFile=process.env.FOUNDRY_PASSWORD_FILE;
if(network!=='local'&&!keystore)throw new Error('FOUNDRY_ACCOUNT must name an existing encrypted Foundry keystore; private keys are not accepted');
async function cast(args:string[]):Promise<Record<string,unknown>> {
 const options=['--rpc-url',rpc!,'--account',keystore!,'--json'];if(passwordFile)options.push('--password-file',passwordFile);
 let stdout='';await new Promise<void>((resolve,reject)=>{const p=spawn(`${cwd}node_modules/.bin/cast`,['send',...args,...options],{cwd,stdio:['inherit','pipe','inherit']});p.stdout.on('data',b=>{stdout+=String(b);});p.once('error',reject);p.once('exit',code=>code===0?resolve():reject(new Error('Cast transaction failed; inspect the local terminal.')));});
 return JSON.parse(stdout) as Record<string,unknown>;
}
async function run(args:string[]) {await new Promise<void>((resolve,reject)=>{const p=spawn(process.execPath,args,{cwd,stdio:'inherit'});p.once('error',reject);p.once('exit',code=>code===0?resolve():reject(new Error('prover command failed')));});}
const require=createRequire(import.meta.url);const solc=require('solc') as {compile(input:string):string};
const sources:Record<string,{content:string}>={};
for(const [name,path]of [['HonkVerifier.sol','apps/prover/generated/hand/HonkVerifier.sol'],['PokerHandRegistry.sol','contracts/src/PokerHandRegistry.sol'],['IHandVerifier.sol','contracts/src/IHandVerifier.sol']])sources[name!]={content:await readFile(new URL(path!,root),'utf8')};
type Artifact={abi:Abi;evm:{bytecode:{object:string;linkReferences:Record<string,Record<string,{start:number;length:number}[]>>};deployedBytecode:{object:string}}};
const compiled=JSON.parse(solc.compile(JSON.stringify({language:'Solidity',sources,settings:{optimizer:{enabled:true,runs:1},evmVersion:'paris',outputSelection:{'*':{'*':['abi','evm.bytecode.object','evm.bytecode.linkReferences','evm.deployedBytecode.object']}}}}))) as {errors?:{severity:string;formattedMessage:string}[];contracts:Record<string,Record<string,Artifact>>};
const errors=compiled.errors?.filter(x=>x.severity==='error')??[];assert.equal(errors.length,0,errors.map(x=>x.formattedMessage).join('\n'));
await mkdir(new URL('deployments/v3/',root),{recursive:true});const reportFile=new URL(`deployments/v3/${network}.json`,root);
type Deployment={network:string;chainId:number;verificationKeySha256:string;bytecodeSha256:string;addresses:Record<string,Address>;codeHashes:Record<string,Hex>;transactions:Hex[];verified:boolean;productionReady:false;handId?:Hex;handTransaction?:Hex;gas?:string;pendingAudit?:string;expiredAudits?:string[]};
let report:Deployment;
try {report=JSON.parse(await readFile(reportFile,'utf8')) as Deployment;if(report.chainId!==chainId||report.verificationKeySha256!==manifest.verificationKeySha256)throw new Error('deployment record belongs to another circuit; preserve it and use a new release directory');}
catch(e){if((e as NodeJS.ErrnoException).code!=='ENOENT')throw e;report={network,chainId,verificationKeySha256:manifest.verificationKeySha256,bytecodeSha256:manifest.bytecodeSha256,addresses:{},codeHashes:{},transactions:[],verified:false,productionReady:false};}
const save=()=>writeFile(reportFile,JSON.stringify(report,null,2));
async function deploy(file:string,name:string,args:unknown[]=[]):Promise<Address> {
 const label=`${file}:${name}`;const known=report.addresses[label];
 if(known){const code=await client.getCode({address:known});if(!code||keccak256(code)!==report.codeHashes[label])throw new Error('saved deployment code mismatch');return known;}
 const a=compiled.contracts[file]?.[name];assert.ok(a);assert.ok(a.evm.deployedBytecode.object.length/2<=24576,'EIP-170 code size');let bytecode=a.evm.bytecode.object;
 for(const [source,libraries]of Object.entries(a.evm.bytecode.linkReferences))for(const [library,slots]of Object.entries(libraries)){const address=await deploy(source,library);for(const slot of slots){assert.equal(slot.length,20);bytecode=bytecode.slice(0,slot.start*2)+address.slice(2).toLowerCase()+bytecode.slice((slot.start+20)*2);}}
 assert.match(bytecode,/^[0-9a-fA-F]+$/);const creation=encodeDeployData({abi:a.abi,bytecode:`0x${bytecode}`,args});
 const tx=network==='local'?await localWallet.sendTransaction({data:creation}):((await cast(['--create',creation])).transactionHash as Hex);
 const receipt=await client.waitForTransactionReceipt({hash:tx,timeout:120000});assert.equal(receipt.status,'success');assert.ok(receipt.contractAddress);const code=await client.getCode({address:receipt.contractAddress});assert.ok(code);
 report.addresses[label]=receipt.contractAddress;report.codeHashes[label]=keccak256(code);report.transactions.push(tx);await save();console.log(`${name}: ${receipt.contractAddress}`);return receipt.contractAddress;
}
const capabilities=await(await fetch(`${process.env.API_URL??'http://127.0.0.1:8080'}/api/v3/capabilities`)).json() as {server:Address};
const verifier=await deploy('HonkVerifier.sol','HonkVerifier');const registry=await deploy('PokerHandRegistry.sol','PokerHandRegistry',[verifier,capabilities.server]);
console.log(`Registry ready on chain ${chainId}: ${registry}`);
if(network==='amoy'&&!process.argv.includes('--skip-demo')&&!report.verified) {
 const artifact=compiled.contracts['PokerHandRegistry.sol']!.PokerHandRegistry!;
 let audit:Audit|undefined;
 if(report.pendingAudit) {
  audit=JSON.parse(await readFile(new URL(report.pendingAudit,root),'utf8')) as Audit;
  const stored=await client.readContract({address:registry,abi:artifact.abi,functionName:'getHand',args:[audit.spec.handId]}) as {exists:boolean;finalStacks:bigint[]};
  if(stored.exists){assert.deepEqual(stored.finalStacks,audit.finalStacks.map(BigInt));report.verified=true;report.handId=audit.spec.handId;await save();}
  else if(BigInt(audit.spec.validUntil)<=BigInt(Math.floor(Date.now()/1000))){(report.expiredAudits??=[]).push(report.pendingAudit);delete report.pendingAudit;await save();audit=undefined;}
 }
 if(!report.verified) {
  if(!audit) {
   const result=await runSignedNine(process.env.API_URL??'http://127.0.0.1:8080',registry,80002);audit=result.audit as unknown as Audit;
   report.pendingAudit=`deployments/v3/amoy-${audit.spec.handId}.json`;
   await writeFile(new URL(report.pendingAudit,root),JSON.stringify(audit,null,2));
   await writeFile(new URL(`deployments/v3/amoy-${audit.spec.handId}-receipts.json`,root),JSON.stringify(result.receipts,null,2));await save();
  }
  await run(['apps/prover/src/hand.ts','check',report.pendingAudit!]);await run(['apps/prover/src/hand.ts','prove',report.pendingAudit!]);
  const proof=JSON.parse(await readFile(new URL('apps/prover/generated/hand/proof.json',root),'utf8')) as {proof:Hex};
  const args=[audit.spec.handId,audit.spec.players,audit.spec.config.stacks,audit.finalStacks,resultCommit(audit),BigInt(audit.spec.validUntil),proof.proof];
  const data=encodeFunctionData({abi:artifact.abi,functionName:'verifyHand',args});const tx=(await cast([registry,'--data',data])).transactionHash as Hex;
  report.handTransaction=tx;await save();
  const receipt=await client.waitForTransactionReceipt({hash:tx,timeout:120000});assert.equal(receipt.status,'success');report.transactions.push(tx);report.gas=receipt.gasUsed.toString();
  const stored=await client.readContract({address:registry,abi:artifact.abi,functionName:'getHand',args:[audit.spec.handId]}) as {exists:boolean;finalStacks:bigint[]};assert.equal(stored.exists,true);assert.deepEqual(stored.finalStacks,audit.finalStacks.map(BigInt));report.verified=true;report.handId=audit.spec.handId;await save();
 }
}
console.log(JSON.stringify({network,chainId,verifier,registry,amoyHandVerified:network==='amoy'&&report.verified,productionReady:false},null,2));
