import { readFile, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
import { createPublicClient, createWalletClient, defineChain, http, type Abi, type Hex } from 'viem';
import { privateKeyToAccount } from 'viem/accounts';

const require = createRequire(import.meta.url);
const solc = require('solc') as {compile(input:string):string};
const source = await readFile(new URL('../apps/prover/generated/ShowdownVerifier.sol',import.meta.url),'utf8');
const proof = JSON.parse(await readFile(new URL('../apps/prover/generated/proof.json',import.meta.url),'utf8')) as {scope:string;proof:Hex;publicInputs:Hex[]};
assert.equal(proof.scope,'seven-card-ranking-demo-only');
type Contract = {abi:Abi;evm:{deployedBytecode:{object:string};bytecode:{object:string;linkReferences:Record<string,Record<string,{start:number;length:number}[]>>}}};
const compiled = JSON.parse(solc.compile(JSON.stringify({language:'Solidity',sources:{'Verifier.sol':{content:source}},settings:{optimizer:{enabled:true,runs:1},evmVersion:'paris',outputSelection:{'*':{'*':['abi','evm.bytecode.object','evm.bytecode.linkReferences','evm.deployedBytecode.object']}}}}))) as {errors?:{severity:string;formattedMessage:string}[];contracts:Record<string,Record<string,Contract>>};
const errors = compiled.errors?.filter(e=>e.severity==='error')??[];
assert.equal(errors.length,0,errors.map(e=>e.formattedMessage).join('\n'));
const contract = compiled.contracts['Verifier.sol']?.HonkVerifier;
assert.ok(contract,'generated HonkVerifier missing');
assert.ok(contract.evm.deployedBytecode.object.length/2 <= 24576,'EIP-170 runtime code size exceeded');
const anvil = spawn(fileURLToPath(new URL('../node_modules/.bin/anvil',import.meta.url)),['--host','127.0.0.1','--port','18545','--silent'],{stdio:['ignore','pipe','pipe']});
let startupError:Error|undefined;anvil.on('error',e=>{startupError=e;});
const chain = defineChain({id:31337,name:'Ephemeral primitive proof test',nativeCurrency:{name:'Ether',symbol:'ETH',decimals:18},rpcUrls:{default:{http:['http://127.0.0.1:18545']}}});
const publicClient = createPublicClient({chain,transport:http(undefined,{retryCount:0}),pollingInterval:100});
// Public Anvil development account. Never use it on a public network.
const account = privateKeyToAccount('0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80');
const wallet = createWalletClient({account,chain,transport:http()});
try {
  let ready=false;
  for(let i=0;i<100;i++) {
    if(startupError)throw startupError;
    if(anvil.exitCode!==null)throw new Error('Anvil failed to start; port 18545 must be free');
    try {await publicClient.getBlockNumber();ready=true;break;} catch {await new Promise(r=>setTimeout(r,100));}
  }
  assert.ok(ready,'Anvil startup timeout');
  // bb's generated verifier uses an external transcript library. Link every
  // referenced library explicitly instead of submitting placeholder bytecode.
  let bytecode = contract.evm.bytecode.object;
  let libraryDeployGas = 0n;
  for (const [file, libraries] of Object.entries(contract.evm.bytecode.linkReferences)) {
    for (const [name, slots] of Object.entries(libraries)) {
      const library = compiled.contracts[file]?.[name];assert.ok(library);
      assert.equal(Object.keys(library.evm.bytecode.linkReferences).length,0,'nested library linking not supported');
      const libraryHash = await wallet.deployContract({abi:library.abi,bytecode:`0x${library.evm.bytecode.object}`});
      const receipt = await publicClient.waitForTransactionReceipt({hash:libraryHash,timeout:20000});
      assert.equal(receipt.status,'success');assert.ok(receipt.contractAddress);libraryDeployGas += receipt.gasUsed;
      for(const slot of slots) {
        assert.equal(slot.length,20);
        bytecode = bytecode.slice(0,slot.start*2)+receipt.contractAddress.slice(2).toLowerCase()+bytecode.slice((slot.start+slot.length)*2);
      }
    }
  }
  assert.match(bytecode,/^[0-9a-fA-F]+$/,'unlinked verifier bytecode');
  const hash = await wallet.deployContract({abi:contract.abi,bytecode:`0x${bytecode}`});
  const deployed = await publicClient.waitForTransactionReceipt({hash,timeout:20000});
  const address = deployed.contractAddress;assert.ok(address);assert.equal(deployed.status,'success');
  assert.equal(await publicClient.readContract({address,abi:contract.abi,functionName:'verify',args:[proof.proof,proof.publicInputs]}),true);
  let tamperedAccepted=false;
  try {tamperedAccepted=await publicClient.readContract({address,abi:contract.abi,functionName:'verify',args:[proof.proof,[`0x${BigInt(6783749).toString(16).padStart(64,'0')}`]]}) as boolean;} catch {/* invalid proof may revert */}
  assert.equal(tamperedAccepted,false);
  const tx = await wallet.writeContract({address,abi:contract.abi,functionName:'verify',args:[proof.proof,proof.publicInputs]});
  const receipt = await publicClient.waitForTransactionReceipt({hash:tx,timeout:20000});assert.equal(receipt.status,'success');
  const report={scope:'seven-card-ranking-demo-only',chainId:31337,verifierRuntimeBytes:contract.evm.deployedBytecode.object.length/2,optimizerRuns:1,libraryDeployGas:libraryDeployGas.toString(),deployGas:deployed.gasUsed.toString(),verifyGas:receipt.gasUsed.toString(),verified:true,tamperedPublicInputRejected:true};
  await writeFile(new URL('../reports/primitive-evm.json',import.meta.url),JSON.stringify(report,null,2));
  console.log(JSON.stringify(report,null,2));
} catch(error) {
  const e=error as Error & {shortMessage?:string};
  console.error(e.shortMessage ?? e.message.split('\n')[0]);process.exitCode=1;
} finally {anvil.kill('SIGTERM');}
