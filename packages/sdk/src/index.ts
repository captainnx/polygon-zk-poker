export * from './v3.ts';
export {encryptBackup,decryptBackup} from './backup.ts';
export {generatePrivateKey as generateSessionKey} from 'viem/accounts';
import {generatePrivateKey,privateKeyToAccount} from 'viem/accounts';
export const createSessionKey=()=>privateKeyToAccount(generatePrivateKey());
