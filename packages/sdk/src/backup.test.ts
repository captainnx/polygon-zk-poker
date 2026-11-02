import test from 'node:test';
import assert from 'node:assert/strict';
import {encryptBackup,decryptBackup} from './backup.ts';
test('encrypted recovery backup rejects wrong password and ciphertext tampering',async()=>{
 const value={privateKey:`0x${'23'.repeat(32)}`,seed:`0x${'42'.repeat(32)}`,cursor:'12'};
 const envelope=await encryptBackup(value,'a-test-password-with-entropy');assert.deepEqual(await decryptBackup(envelope,'a-test-password-with-entropy'),value);
 await assert.rejects(decryptBackup(envelope,'wrong-password'));
 const changed={...envelope,ciphertext:`0x${(parseInt(envelope.ciphertext.slice(2,4),16)^1).toString(16).padStart(2,'0')}${envelope.ciphertext.slice(4)}` as const};await assert.rejects(decryptBackup(changed,'a-test-password-with-entropy'));
});
