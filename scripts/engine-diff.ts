import {readFile,writeFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
import {Noir,type CompiledCircuit,type InputMap} from '@noir-lang/noir_js';
const compiled:CompiledCircuit=JSON.parse(await readFile(new URL('../apps/prover/engine-diff/target/engine_diff.json',import.meta.url),'utf8'));
const vectors=JSON.parse(await readFile(new URL('../fixtures/engine-vectors.json',import.meta.url),'utf8')) as InputMap[];
const noir=new Noir(compiled);let passed=0;
for(const vector of vectors){await noir.execute(vector);passed++;if(passed%10===0)console.log(`Go/Noir differential vectors passed: ${passed}/${vectors.length}`);}
const changed=structuredClone(vectors[0]!);(changed.expected as string[])[0]=String(BigInt((changed.expected as string[])[0]!)+1n);await assert.rejects(noir.execute(changed));
const report={cases:passed,players:'2–9',seed:912342,checks:'blinds, ante, folds, full raises, all-in, runout, ranking, refunds, side pots and final chips',tamperedResultRejected:true};
await writeFile(new URL('../reports/engine-differential.json',import.meta.url),JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
