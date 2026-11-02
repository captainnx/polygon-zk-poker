import './style.css';
import {mountSessions} from './session.ts';

type Audit = { handId:string; serverCommit:string; deckCommit:string; records:{action:{seat:number;kind:string;amount:number};chain:string}[];finalStacks:number[] };
type Demo = {hand:{board:number[];pots:{amount:number;winners:number[]}[];config:{stacks:number[]};payouts:number[]};audit:Audit;report:{actions:number;chain:string;scope:string}};
const root = document.querySelector<HTMLDivElement>('#app')!;
root.innerHTML = `<main><header><div class="brand">◈ POLYGON <span>/ ZK POKER</span></div><span class="tag">ENGINE LAB · v0.1</span></header><section class="intro"><p class="eyebrow">可复现的每一步</p><h1>九人同桌，<br><span>规则可检验。</span></h1><p>无需钱包连接，使用账号或游客身份进入牌桌。客户端自动签署游戏动作，结束后可验证牌局执行。</p><div class="controls"><button id="run">九人引擎演示 ↗</button><button id="download" disabled>下载回放 ↓</button></div><p class="notice">玩家无需钱包或代币授权。游戏动作自动签名；链上登记由运营端提交。当前为开发测试版本。</p></section><section class="workspace"><div><div class="section-head"><h2>THE TABLE</h2><span id="status" role="status" aria-live="polite">等待模拟</span></div><div class="table"><div class="felt"><div class="board" id="board"><span>♠</span><span>♥</span><span>♣</span><span>♦</span><span>♠</span></div><p>NO-LIMIT HOLD’EM · 9 MAX</p></div><div id="seats"></div></div><div class="stats" id="stats"><div>9<span>座位</span></div><div>5 / 10<span>大小盲</span></div><div>1<span>每人 Ante</span></div></div></div><aside><h2>执行记录</h2><p class="muted">承诺 → 揭示 → 确定性洗牌 → 动作重放</p><dl><dt>SERVER COMMIT</dt><dd id="server-commit">—</dd><dt>DECK COMMIT</dt><dd id="deck-commit">—</dd><dt>FINAL CHAIN</dt><dd id="chain">—</dd></dl><ol id="actions"><li>点击运行，生成 9 人牌局。</li></ol><div id="pots"></div></aside></section><footer>LOCAL DEVELOPMENT · V3 <span>无需钱包 · 无代币转账 · 非生产版本</span></footer></main>`;
const positions = [[50,3],[79,12],[94,37],[89,67],[67,84],[32,84],[10,67],[5,37],[21,12]];
const seats = document.querySelector<HTMLDivElement>('#seats')!;
positions.forEach(([x,y],i)=>{const s=document.createElement('div');s.className='seat';s.style.left=`${x}%`;s.style.top=`${y}%`;s.innerHTML=`<div class="avatar">${String(i+1).padStart(2,'0')}</div><strong>SEAT ${i+1}${i===0?' · D':''}</strong><span id="stack-${i}">—</span>`;seats.append(s);});
const run = document.querySelector<HTMLButtonElement>('#run')!;
const download = document.querySelector<HTMLButtonElement>('#download')!;
let current:Demo|undefined;
const setText=(selector:string,value:string)=>{document.querySelector(selector)!.textContent=value;};
run.onclick=async()=>{
  run.disabled=true;setText('#status','正在运行…');
  try {
    const response=await fetch('/api/demo',{method:'POST'});if(!response.ok) throw new Error(`HTTP ${response.status}`);
    const demo:Demo=await response.json();current=demo;
    demo.audit.finalStacks.forEach((s,i)=>setText(`#stack-${i}`,s.toLocaleString()));
    const board=document.querySelector('#board')!;board.replaceChildren();
    demo.hand.board.forEach(c=>{const card=document.createElement('span');card.textContent='23456789TJQKA'[Math.floor(c/4)]!+['♣','♦','♥','♠'][c%4]!;if(c%4===1||c%4===2)card.className='red';board.append(card);});
    setText('#server-commit',demo.audit.serverCommit);setText('#deck-commit',demo.audit.deckCommit);setText('#chain',demo.report.chain);
    const actions=document.querySelector('#actions')!;actions.replaceChildren();
    demo.audit.records.forEach(r=>{const li=document.createElement('li');li.textContent=`Seat ${r.action.seat+1} · ${r.action.kind}${r.action.amount?` → ${r.action.amount}`:''}`;actions.append(li);});
    const pots=document.querySelector('#pots')!;pots.replaceChildren();demo.hand.pots.forEach((p,i)=>{const row=document.createElement('p');row.textContent=`${i===0?'主池':`边池 ${i}`} · ${p.amount} → Seat ${p.winners.map(s=>s+1).join(', ')}`;pots.append(row);});
    const initial=demo.hand.config.stacks.reduce((a,b)=>a+b,0);const final=demo.audit.finalStacks.reduce((a,b)=>a+b,0);
    if(initial!==final)throw new Error('筹码守恒失败');
    setText('#status',`${demo.report.actions} 个动作 · ${final.toLocaleString()} 筹码守恒`);download.disabled=false;
  } catch(error) {setText('#status',`运行失败：${error instanceof Error?error.message:String(error)}`);} finally {run.disabled=false;}
};
download.onclick=()=>{if(!current)return;const u=URL.createObjectURL(new Blob([JSON.stringify(current.audit,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=u;a.download=`hand-${current.audit.handId}.json`;a.click();setTimeout(()=>URL.revokeObjectURL(u),1000);};
mountSessions(document.querySelector('main')!);
