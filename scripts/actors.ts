import {randomBytes} from 'node:crypto';
import {generatePrivateKey,privateKeyToAccount} from 'viem/accounts';
import type {Principal,Request} from '../packages/sdk/src/v3.ts';
// Synthetic clients keep cookies, passwords and signing keys in memory only.
export function actor(base:string){
 let cookie='';
 const request:Request=async<T>(path:string,body:unknown)=>{
  const r=await fetch(`${base}${path}`,{method:'POST',headers:{'Content-Type':'application/json',...(cookie?{Cookie:cookie}:{})},body:JSON.stringify(body)});
  const cookies=r.headers.getSetCookie();if(cookies.length)cookie=cookies.at(-1)!.split(';')[0]!;
  const value=await r.json();if(!r.ok)throw new Error(value.error??`HTTP ${r.status}`);return value as T;
 };
 return {request,key:privateKeyToAccount(generatePrivateKey()),guest:()=>request<Principal>('/api/v3/auth/guest',{}),account:()=>request<Principal>('/api/v3/auth/register',{username:`sim_${randomBytes(8).toString('hex')}`,password:randomBytes(24).toString('hex')})};
}
