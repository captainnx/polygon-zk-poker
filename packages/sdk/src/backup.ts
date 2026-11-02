import {hexToBytes,toHex,type Hex} from 'viem';
type Envelope={version:1;salt:Hex;iv:Hex;ciphertext:Hex};
const buffer=(v:Uint8Array)=>new Uint8Array(v).buffer;
async function derive(password:string,salt:Uint8Array) {
 const material=await crypto.subtle.importKey('raw',buffer(new TextEncoder().encode(password)),'PBKDF2',false,['deriveKey']);
 return crypto.subtle.deriveKey({name:'PBKDF2',salt:buffer(salt),iterations:310000,hash:'SHA-256'},material,{name:'AES-GCM',length:256},false,['encrypt','decrypt']);
}
export async function encryptBackup(value:unknown,password:string):Promise<Envelope> {
 if(password.length<12)throw new Error('备份口令至少 12 个字符');
 const salt=crypto.getRandomValues(new Uint8Array(16));const iv=crypto.getRandomValues(new Uint8Array(12));const key=await derive(password,salt);
 const encrypted=await crypto.subtle.encrypt({name:'AES-GCM',iv,additionalData:new TextEncoder().encode('polygon-zk-poker/session-backup/v1')},key,buffer(new TextEncoder().encode(JSON.stringify(value))));
 return {version:1,salt:toHex(salt),iv:toHex(iv),ciphertext:toHex(new Uint8Array(encrypted))};
}
export async function decryptBackup(envelope:Envelope,password:string):Promise<unknown> {
 if(envelope.version!==1||envelope.ciphertext.length>2_000_000)throw new Error('unsupported backup');
 const salt=hexToBytes(envelope.salt);const iv=hexToBytes(envelope.iv);if(salt.length!==16||iv.length!==12)throw new Error('invalid backup');
 const key=await derive(password,salt);const decoded=await crypto.subtle.decrypt({name:'AES-GCM',iv:buffer(iv),additionalData:new TextEncoder().encode('polygon-zk-poker/session-backup/v1')},key,buffer(hexToBytes(envelope.ciphertext)));
 return JSON.parse(new TextDecoder().decode(decoded));
}
