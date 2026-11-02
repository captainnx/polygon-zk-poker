import {readFile,writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
import {createPublicClient,createWalletClient,defineChain,http,decodeEventLog,type Abi,type Hex,type Address} from 'viem';
import {privateKeyToAccount} from 'viem/accounts';
import {runSignedNine} from './signed-nine.ts';
import {resultCommit,type Audit} from '../apps/prover/src/hand-input.ts';

const root=fileURLToPath(new URL('../',import.meta.url));
async function run(command:string,args:string[]) {await new Promise<void>((resolve,reject)=>{const p=spawn(command,args,{cwd:root,stdio:'inherit'});p.once('error',reject);p.once('exit',code=>code===0?resolve():reject(new Error(`${command} failed: ${code}`)));});}
const require=createRequire(import.meta.url);const solc=require('solc') as {compile(s:string):string};
const sources:Record<string,{content:string}>={};
for(const [name,path]of [['HonkVerifier.sol','apps/prover/generated/hand/HonkVerifier.sol'],['PokerHandRegistry.sol','contracts/src/PokerHandRegistry.sol'],['IHandVerifier.sol','contracts/src/IHandVerifier.sol']])sources[name!]={content:await readFile(`${root}${path}`,'utf8')};
type Artifact={abi:Abi;evm:{bytecode:{object:string;linkReferences:Record<string,Record<string,{start:number;length:number}[]>>};deployedBytecode:{object:string}}};
const compiled=JSON.parse(solc.compile(JSON.stringify({language:'Solidity',sources,settings:{optimizer:{enabled:true,runs:1},evmVersion:'paris',outputSelection:{'*':{'*':['abi','evm.bytecode.object','evm.bytecode.linkReferences','evm.deployedBytecode.object']}}}}))) as {errors?:{severity:string;formattedMessage:string}[];contracts:Record<string,Record<string,Artifact>>};
const errors=compiled.errors?.filter(x=>x.severity==='error')??[];assert.equal(errors.length,0,errors.map(x=>x.formattedMessage).join('\n'));
const verifier=compiled.contracts['HonkVerifier.sol']?.HonkVerifier;const registry=compiled.contracts['PokerHandRegistry.sol']?.PokerHandRegistry;assert.ok(verifier);assert.ok(registry);assert.ok(verifier.evm.deployedBytecode.object.length/2<=24576,'generated verifier exceeds EIP-170');
const anvil=spawn(`${root}node_modules/.bin/anvil`,['--host','127.0.0.1','--port','18545','--silent'],{stdio:['ignore','pipe','pipe']});let startError:Error|undefined;anvil.on('error',e=>{startError=e;});
const chain=defineChain({id:31337,name:'Full signed poker test',nativeCurrency:{name:'Ether',symbol:'ETH',decimals:18},rpcUrls:{default:{http:['http://127.0.0.1:18545']}}});
const account=privateKeyToAccount('0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80');
const client=createPublicClient({chain,transport:http(undefined,{retryCount:0}),pollingInterval:100});const wallet=createWalletClient({account,chain,transport:http()});
const deployedLibraries=new Map<string,Address>();let deploymentGas=0n;
async function deploy(artifact:Artifact,args:unknown[]=[]):Promise<Address> {
 let bytecode=artifact.evm.bytecode.object;
 for(const [file,libraries]of Object.entries(artifact.evm.bytecode.linkReferences))for(const [name,slots]of Object.entries(libraries)) {
  const label=`${file}:${name}`;let address=deployedLibraries.get(label);if(!address){const lib=compiled.contracts[file]?.[name];assert.ok(lib);address=await deploy(lib);deployedLibraries.set(label,address);}
  for(const slot of slots){assert.equal(slot.length,20);bytecode=bytecode.slice(0,slot.start*2)+address.slice(2).toLowerCase()+bytecode.slice((slot.start+20)*2);}
 }
 assert.match(bytecode,/^[0-9a-fA-F]+$/);const hash=await wallet.deployContract({abi:artifact.abi,bytecode:`0x${bytecode}`,args});const receipt=await client.waitForTransactionReceipt({hash,timeout:30000});assert.equal(receipt.status,'success');assert.ok(receipt.contractAddress);deploymentGas+=receipt.gasUsed;return receipt.contractAddress;
}
try {
 let ready=false;for(let i=0;i<100;i++){if(startError)throw startError;if(anvil.exitCode!==null)throw new Error('Anvil failed to start; port 18545 must be free');try{await client.getBlockNumber();ready=true;break;}catch{await new Promise(r=>setTimeout(r,100));}}assert.ok(ready);
 const base=process.env.API_URL??'http://127.0.0.1:8080';const caps=await(await fetch(`${base}/api/v3/capabilities`)).json() as {server:Address};
 const verifierAddress=await deploy(verifier);const registryAddress=await deploy(registry,[verifierAddress,caps.server]);const beforeHand=await client.getBlockNumber({cacheTime:0});
 console.log(`Full verifier deployed; registry=${registryAddress}`);
 let restarted=false;
 const result=await runSignedNine(base,registryAddress,31337,async moves=>{
  if(moves===3&&process.env.E2E_RESTART_SERVER!=='0') {
   await run('docker',['compose','restart','server']);let available=false;
   for(let i=0;i<60;i++){try{const r=await fetch(`${base}/api/v3/capabilities`);if(r.ok){available=true;break;}}catch{}await new Promise(r=>setTimeout(r,500));}
   assert.ok(available,'service did not recover');restarted=true;
  }
 });
 await writeFile(`${root}reports/signed-hand.json`,JSON.stringify(result.audit,null,2));await writeFile(`${root}reports/signed-receipts.json`,JSON.stringify(result.receipts,null,2));
 await run('go',['run','./apps/server/cmd/verify-signed','reports/signed-hand.json']);
 await run('node',['apps/prover/src/hand.ts','check','reports/signed-hand.json']);
 await run('node',['apps/prover/src/hand.ts','prove','reports/signed-hand.json']);
 const proof=JSON.parse(await readFile(`${root}apps/prover/generated/hand/proof.json`,'utf8')) as {scope:string;proof:Hex;publicInputs:Hex[]};assert.equal(proof.scope,'signed-hand-v3');
 const audit=result.audit as unknown as Audit;const commitment=resultCommit(audit);const spec=audit.spec;
 const args=[spec.handId,spec.players,spec.config.stacks,audit.finalStacks,commitment,BigInt(spec.validUntil)] as const;
 const expected=await client.readContract({address:registryAddress,abi:registry.abi,functionName:'statement',args}) as Hex[];assert.deepEqual(expected,proof.publicInputs);
 assert.equal(await client.getBlockNumber({cacheTime:0}),beforeHand,'game unexpectedly made an on-chain transaction');
 // Balance-preserving output tamper must reach and fail the real verifier.
 const changed=[...audit.finalStacks];const donor=changed.findIndex(x=>x>0);const receiver=(donor+1)%9;changed[donor]=changed[donor]!-1;changed[receiver]=changed[receiver]!+1;
 await assert.rejects(client.simulateContract({account,address:registryAddress,abi:registry.abi,functionName:'verifyHand',args:[spec.handId,spec.players,spec.config.stacks,changed,commitment,BigInt(spec.validUntil),proof.proof]}));
 const tx=await wallet.writeContract({address:registryAddress,abi:registry.abi,functionName:'verifyHand',args:[...args,proof.proof]});
 const receipt=await client.waitForTransactionReceipt({hash:tx,timeout:30000});assert.equal(receipt.status,'success');
 const event=receipt.logs.filter(x=>x.address.toLowerCase()===registryAddress.toLowerCase()).map(log=>decodeEventLog({abi:registry.abi,data:log.data,topics:log.topics}));assert.equal(event.length,1);assert.equal(event[0]!.eventName,'HandVerified');
 const recorded=await client.readContract({address:registryAddress,abi:registry.abi,functionName:'getHand',args:[spec.handId]}) as {exists:boolean;finalStacks:bigint[]};assert.equal(recorded.exists,true);assert.deepEqual(recorded.finalStacks,audit.finalStacks.map(BigInt));
 await assert.rejects(client.simulateContract({account,address:registryAddress,abi:registry.abi,functionName:'verifyHand',args:[...args,proof.proof]}));
 assert.equal((await client.getBlockNumber({cacheTime:0}))-beforeHand,1n);
 const report={scope:'signed-hand-v3',chainId:31337,verifier:verifierAddress,registry:registryAddress,handId:spec.handId,players:9,actions:9,totalChips:6950,serverRestartRecovered:restarted,oneHandTransaction:true,transactionHash:tx,verifyAndRecordGas:receipt.gasUsed.toString(),deploymentGas:deploymentGas.toString(),verifierRuntimeBytes:verifier.evm.deployedBytecode.object.length/2,balancedOutputTamperRejected:true,duplicateHandRejected:true,handVerifiedEvent:true};
 await writeFile(`${root}reports/full-e2e.json`,JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
} catch(error) {const e=error as Error&{shortMessage?:string};console.error(e.shortMessage??e.message.split('\n')[0]);process.exitCode=1;}finally{anvil.kill('SIGTERM');}
