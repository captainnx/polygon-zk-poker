import {existsSync} from 'node:fs';
export function getReadiness(root=new URL('../../../',import.meta.url)) {
 return {mode:'development',protocol:'signed-hand-v3',actionCapacity:64,fullHandRelation:true,
  compiled:existsSync(new URL('apps/prover/hand/target/poker_hand_v3.json',root)),
  nativeBackend:existsSync(new URL('.tools/bb',root)),
  generatedHandVerifier:existsSync(new URL('apps/prover/generated/hand/manifest.json',root)),
  productionReady:false};
}
export const readiness=getReadiness();
export function requireHandCircuit(state=getReadiness()):void {
 if(!state.compiled||!state.nativeBackend||!state.generatedHandVerifier)throw new Error('Compile the hand circuit and run prepare-hand with the native backend first. No proof was produced.');
}
