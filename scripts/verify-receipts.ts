import {readFile} from 'node:fs/promises';
import {verifyReceipt,ZERO,type Receipt} from '../packages/sdk/src/v3.ts';
import type {Address,Hex} from 'viem';
const [file,server,handId]=process.argv.slice(2);
if(!file||!server||!handId)throw new Error('usage: verify-receipts.ts FILE SERVER_ADDRESS HAND_ID');
const receipts=JSON.parse(await readFile(file,'utf8')) as Receipt[];let previous=ZERO;
for(let i=0;i<receipts.length;i++){const r=receipts[i]!;if(r.handId!==handId)throw new Error('wrong hand');await verifyReceipt(r,server as Address,previous,BigInt(i));previous=r.digest;}
console.log(JSON.stringify({verified:true,receipts:receipts.length,server,handId,head:previous,scope:'signed receipt order and integrity; server timestamps and availability are not independently proven'},null,2));
