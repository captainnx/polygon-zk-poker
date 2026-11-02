// Executable UltraHonk ZK smoke test. Output cannot be used by PokerHandRegistry.
import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { performance } from 'node:perf_hooks';
import assert from 'node:assert/strict';
import { Noir, type CompiledCircuit } from '@noir-lang/noir_js';
import { UltraHonkBackend } from '@aztec/bb.js';

const circuitPath = new URL('../circuit/target/showdown_demo.json', import.meta.url);
const output = new URL('../generated/', import.meta.url);
const circuit:CompiledCircuit = JSON.parse(await readFile(circuitPath, 'utf8'));
const noir = new Noir(circuit);
const backend = new UltraHonkBackend(circuit.bytecode, {threads:1});
// keccak:true disables ZK in this pinned backend. Explicitly use keccakZK.
const options = {keccakZK:true};
const start = performance.now();
try {
  const {witness} = await noir.execute({cards:[32,36,40,44,48,1,6],expected_rank:6783750});
  await assert.rejects(noir.execute({cards:[32,36,40,44,48,1,6],expected_rank:6783749}));
  const beforeProve = performance.now();
  const proof = await backend.generateProof(witness, options);
  const afterProve = performance.now();
  assert.equal(await backend.verifyProof(proof, options), true);
  async function accepted(value:typeof proof) {
    try {return await backend.verifyProof(value, options);} catch {return false;}
  }
  const changedInputs = {...proof,publicInputs:[`0x${BigInt(6783749).toString(16).padStart(64,'0')}`]};
  assert.equal(await accepted(changedInputs), false, 'tampered public rank accepted');
  const broken = {...proof,proof:proof.proof.slice()};broken.proof[0] = broken.proof[0]! ^ 1;
  assert.equal(await accepted(broken), false, 'tampered proof accepted');
  const vk = await backend.getVerificationKey(options);
  const solidity = await backend.getSolidityVerifier(vk, options);
  await mkdir(output,{recursive:true});
  await writeFile(new URL('ShowdownVerifier.sol',output),solidity);
  await writeFile(new URL('proof.json',output),JSON.stringify({scope:'seven-card-ranking-demo-only',proof:`0x${Buffer.from(proof.proof).toString('hex')}`,publicInputs:proof.publicInputs},null,2));
  await writeFile(new URL('vk.bin',output),vk);
  const report = {scope:'seven-card-ranking-demo-only',scheme:'UltraHonk',zeroKnowledge:true,oracle:'keccak',proofBytes:proof.proof.length,publicInputs:proof.publicInputs,proverMs:afterProve-beforeProve,totalMs:performance.now()-start,nodePeakRSSBytes:process.resourceUsage().maxRSS*1024,verified:true,tamperedProofRejected:true,tamperedPublicInputRejected:true,output:fileURLToPath(output)};
  await writeFile(new URL('../../../reports/primitive-proof.json',import.meta.url),JSON.stringify(report,null,2));
  console.log(JSON.stringify(report,null,2));
} finally {await backend.destroy();}
