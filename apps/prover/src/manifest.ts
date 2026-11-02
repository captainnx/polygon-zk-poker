import {createHash} from 'node:crypto';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
const hash=(v:Uint8Array|string)=>createHash('sha256').update(v).digest('hex');
export type Manifest={protocol:'signed-hand-v3';actionCapacity:64;noir:string;barretenberg:string;bytecodeSha256:string;verificationKeySha256:string;soliditySha256:string;zeroKnowledge:true;oracle:'keccak';productionReady:false};
export async function writeManifest(root:URL):Promise<Manifest> {
 const circuit=JSON.parse(await readFile(new URL('apps/prover/hand/target/poker_hand_v3.json',root),'utf8')) as {bytecode:string;noir_version?:string};
 const vk=await readFile(new URL('apps/prover/generated/hand/vk',root));const source=await readFile(new URL('apps/prover/generated/hand/HonkVerifier.sol',root));
 const manifest:Manifest={protocol:'signed-hand-v3',actionCapacity:64,noir:circuit.noir_version??'1.0.0-beta.18',barretenberg:'3.0.0-nightly.20251104',bytecodeSha256:hash(Buffer.from(circuit.bytecode,'base64')),verificationKeySha256:hash(vk),soliditySha256:hash(source),zeroKnowledge:true,oracle:'keccak',productionReady:false};
 await writeFile(new URL('apps/prover/generated/hand/manifest.json',root),JSON.stringify(manifest,null,2));
 await mkdir(new URL('contracts/src/generated/',root),{recursive:true});await writeFile(new URL('contracts/src/generated/PokerHandVerifier.sol',root),source);
 await writeFile(new URL('contracts/verification-key.json',root),JSON.stringify(manifest,null,2));return manifest;
}
export async function verifyManifest(root:URL):Promise<Manifest> {
 const m=JSON.parse(await readFile(new URL('apps/prover/generated/hand/manifest.json',root),'utf8')) as Manifest;
 const c=JSON.parse(await readFile(new URL('apps/prover/hand/target/poker_hand_v3.json',root),'utf8')) as {bytecode:string};
 const vk=await readFile(new URL('apps/prover/generated/hand/vk',root));
 const source=await readFile(new URL('apps/prover/generated/hand/HonkVerifier.sol',root));
 if(m.protocol!=='signed-hand-v3'||m.actionCapacity!==64||!m.zeroKnowledge||m.bytecodeSha256!==hash(Buffer.from(c.bytecode,'base64'))||m.verificationKeySha256!==hash(vk)||m.soliditySha256!==hash(source))throw new Error('circuit/VK/verifier mismatch: run prepare-hand again');
 return m;
}
