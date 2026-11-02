import { readiness, requireHandCircuit } from './readiness.ts';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
const command = process.argv[2];
try {
  if (command === 'status') console.log(JSON.stringify(readiness, null, 2));
  else if (command === 'prove-hand'||command==='prepare-hand'||command==='test-hand') {
    if(command==='prove-hand')requireHandCircuit();
    const action=command==='prove-hand'?'prove':command==='prepare-hand'?'prepare':'check';
    await new Promise<void>((resolve,reject)=>{const p=spawn(process.execPath,[fileURLToPath(new URL('./hand.ts',import.meta.url)),action,...process.argv.slice(3)],{stdio:'inherit'});p.once('error',reject);p.once('exit',code=>code===0?resolve():reject(new Error('hand prover failed')));});
  } else throw new Error('usage: cli.ts status|prepare-hand|test-hand|prove-hand [AUDIT.json]');
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
