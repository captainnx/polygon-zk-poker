# signed-hand-v2 协议

此文描述当前可执行的开发协议。旧 `engine-audit-v1` 仅作引擎回归，与 v2 不兼容。v2 尚无稳定版本兼容承诺，正式上线前必须冻结格式并审计。

所有整数按指定宽度大端编码，哈希为 SHA256，`||` 为拼接；Hash/Address 的 JSON 使用带 `0x` 十六进制；uint64 JSON 使用十进制字符串。seat=0..8，服务器随机性角色=255。空座 initial=0、wallet/session/seed/commit=0。9 槽数组不省略空位。

## 授权和开局

钱包签署 EIP-712，domain=`PolygonZKPoker`，version=`1`，chainId 与 verifyingContract=Registry。类型为：

```text
SessionAuthorization(address wallet,address sessionKey,bytes32 tableId,uint64 nonce,uint64 expiresAt)
```

当前支持 secp256k1 EOA 钱包，尚未实现 EIP-1271 智能合约钱包。SessionID 是该 typed-data digest。服务器验签、限制授权最长七天，持久化域内 wallet/nonce 唯一性与撤销状态。完全相同的授权重试不会重新激活已撤销 Session。撤销由钱包签 `RevokeSession(bytes32 sessionId)`。钱包不需要每手弹窗；后续由 Session Key 直接签 32 字节 digest（不加 personal_sign 前缀）。

Spec 包含 domain、tableId、handId、9 座身份与 sessionId、初始筹码/规则、validUntil、actionLimit。截止最多一小时，且所有 Session 覆盖截止时间；动作上限 1..64。Context：

```text
H("ZKPOKER_CONTEXT_V2" || chainId.u64 || registry[20] || tableId[32] || handId[32]
 || initial[9].u64 || button.u8 || smallBlind.u64 || bigBlind.u64 || ante.u64
 || wallets[9][20] || sessionIDs[9][32] || validUntil.u64 || actionLimit.u16)
```

服务器先生成随机 serverSeed[32] 并返回承诺；每个玩家在钉住 Spec 和该承诺后生成新的独立 seed。不得跨手复用 seed。

```text
seedCommit = H("ZKPOKER_SEED_V2" || context || seat.u8 || seed[32])
manifest = H("ZKPOKER_MANIFEST_V2" || context || serverCommit || playerCommits[9][32])
consentDigest = H("ZKPOKER_CONSENT_V2" || sessionID || handId || manifest)
```

状态依次 committing → consenting → revealing → playing → complete；超时/动作上限可转 aborted。收齐所有 Commit 后集合冻结，再收齐所有玩家对同一 Manifest 的 Consent。客户端核对自己承诺、预期 Context，并验证所有同伴的钱包授权和 Consent 签名后才发送 Reveal。服务器不得加入新承诺、改变 roster、替换缺席随机数或在未同意前索取种子。

Commit 和 Reveal 请求分别由 Session Key 签：

```text
H("ZKPOKER_COMMIT_V2" || sessionID || handId || context || commit)
H("ZKPOKER_REVEAL_V2" || sessionID || handId || manifest || seed)
```

服务端验证这些传输签名。电路验证种子预像和全部 Consent 签名，借此绑定共同认可的随机性；Commit/Reveal 的传输签名和服务器收据不是电路的外部时间证明。

## 确定性洗牌

```text
mixed = H("ZKPOKER_MIX_V2" || context || serverSeed
          || concat(occupiedSeat.u8 || playerSeed[32]) ascending seat)
priority[card] = H("ZKPOKER_CARD_V2" || mixed || card.u8), card=0..51
```

按 priority 的无符号 256 位值升序排列，碰撞时按 cardID 排序，得到 deck[52]。此 hash-sort 算法在随机预言机模型下除碰撞外等概率，碰撞界为 `1326 / 2^256`；不声称数学上绝对无偏。它避免有限电路内拒绝采样的无界问题。Go/SDK/Noir 使用同一算法，旧 Fisher–Yates 演示不用于 v2。

```text
deckCommit = H("ZKPOKER_DECK_V2" || context || deck[52])
initialState = H("ZKPOKER_INITIAL_V2" || context || manifest || deckCommit)
```

## 所见状态与动作链

Action 的 kind：Fold=0、Check=1、Call=2、Bet=3、Raise=4、All-in=5。Bet/Raise amount 为本街总目标，其他 amount 必须 0。sequence 从 1 开始。为了让签名也绑定玩家所见信息：

```text
publicView = H("ZKPVIEW2" || concat(stack.u40 || streetBet.u40) for seats0..8
 || foldedMask.u16 || currentBet.u40 || minRaise.u40 || board[5]
 || street.u8 || actor.u8 || previousSequence.u8)
holeCommit = H("ZKPOKER_HOLE_V2" || context || seat.u8 || playerSeed[32] || hole[2])
actionDigest = H("ZKPOKER_ACTION_V2" || sessionID || handId || previousState
 || sequence.u64 || seat.u8 || kind.u8 || amount.u64 || publicView || holeCommit)
```

board 未发位填 0；street=Preflop0/Flop1/Turn2/River3；玩家种子隐藏底牌承诺，其他玩家不能枚举 1,326 对底牌反推。客户端使用收到的公开状态和自己授权通道收到的底牌重算签名；服务器和电路使用实际引擎状态重算。伪造展示与实际执行不符会使动作验签/证明失败。签名并不证明 UI 没有遭到恶意代码篡改。

下一 state=actionDigest。服务端要求 previousState 和 sequence 正确、座位轮到本人、签名属于原 Session，再原子提交。Go 接口拒绝非规范高 S 签名。电路验证 ECDSA 数学有效性；签名的编码规范性不承担身份或重放安全边界。

```text
protocolCommitment = H("ZKPOKER_RESULT_V2" || manifest || deckCommit || terminalState)
```

## 收据、恢复与披露

服务器签名收据：

```text
receiptDigest = H("ZKPOKER_RECEIPT_V2" || handId || index.u64 || at.u64
 || serverAddress[20] || previousReceipt || H(kind.utf8) || H(payload.rawBytes))
```

payload 在 JSON 里为 Base64 原始字节，index 从 0 起。玩家外部保留共同 Manifest、完整收据链和服务器地址钉住信息，可识别日志分叉；服务器自报时间不能当可信时间。状态读取授权：

```text
H("ZKPOKER_READ_V2" || sessionID || handId || context || timestamp.u256)
```

仅接受当前时间 ±30 秒的签名，恢复时带 after 和 knownReceipt。只返回自己的底牌；公开缓存不含牌序/种子。完成后参与钱包可以下载 Audit 与收据，用 `verify-signed` 离线验证。Audit 包含所有牌与随机性，应只在完成后有意公开。

服务重启从持久化密钥和 Postgres 恢复；相同已接受动作重试返回原签名 ACK。WebSocket `/api/v2/ws` 接受 recover 与 action，HTTP 对应 `/api/v2/hands/{id}/{commit,consent,reveal,action,recover,audit,timeout}`。`/api/v2/sessions` 创建授权，`/api/v2/sessions/revoke` 撤销。完整 Spec 公共读取允许旁观，Hole/Audit 读取需认证。

## 完整证明与 Registry

```text
verifyHand(bytes32 handId,address[9] players,uint64[9] initialStacks,
 uint64[9] finalStacks,bytes32 protocolCommitment,uint64 validUntil,bytes proof)
```

34 个公开字段：chainId、Registry 地址、handId.high128、handId.low128、9 个钱包、9 个初始筹码、9 个最终筹码、protocolCommitment.high128、low128、validUntil。私有 witness 包含 Spec 剩余字段、钱包和 Session 公钥/签名、种子、牌序及 64 槽动作。字段均有范围约束；handId/承诺无取模别名。

Registry 拒绝重复 HandID、非法 roster、越界/不守恒筹码和过期截止；真实 Verifier 通过后记录结果并发出 HandVerified。正常一手一笔交易。chain31337 为本地，Amoy=80002，Polygon PoS=137。部署脚本核对 VK 指纹与网络，主网要求同 VK 的 Amoy 成功验证记录。

网络配置参考：[Polygon 官方文档](https://docs.polygon.technology/tools/dApp-development/common-tools/remix/)。生产使用前应重新核对提供商 RPC；脚本不硬编码公共测试网 RPC。
