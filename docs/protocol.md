# signed-hand-v3：无钱包玩家身份

v3 不使用玩家钱包或 EIP-712。所有数字按指定字节数大端编码，Hash/Address 的 JSON 带 0x，uint64 JSON 为十进制字符串。所有 SHA 均为 SHA256。

## 登录和自动密钥

`POST /api/v3/auth/{guest,register,login,logout}`、`GET /api/v3/auth/me`。账号名规范化为小写；密码 Argon2id：64 MiB、3 次、并行度 1、16-byte salt、32-byte hash，最多 2 个并发派生。游客随机生成 32-byte identityId，注册可将游客升级为账号并保留 ID。

登录凭证为 32-byte 随机 token，数据库只存 `SHA("ZKPOKER_LOGIN_V3"||token.ascii)`。Cookie 有效七天，HttpOnly、SameSite=Strict；HTTPS 或 COOKIE_SECURE=true 时 Secure。账号注册／登录／游客入口按实际 TCP 来源限制 60 次/分钟，不信任任意 X-Forwarded-For；生产反代的用户级限流需另外配置。未知账号和错误密码给相同错误，均执行密码派生。身份服务是账号真实性的信任源。

客户端登录后自动生成 secp256k1 key，私钥不上传。链上玩家 ID 为：

```text
playerId = SHA("ZKPOKER_PLAYER_V3" || sessionPublicKey.x[32] || .y[32])
```

`POST /api/v3/sessions/issue` 需要登录 Cookie，接收 domain、sessionKey address、完整 65-byte publicKey、tableId、nonce、expiresAt。服务验证公钥与地址相符，并从已认证 Cookie 取得 accountId/kind；不能由请求指定他人的 accountId。Guest kind=0，Account kind=1。

```text
sessionId = SHA("ZKPOKER_SESSION_V3" || chainId.u64 || registry[20] || issuer[20]
 || playerId[32] || accountId[32] || sessionKeyAddress[20] || tableId[32]
 || nonce.u64 || expiresAt.u64 || kind.u8)
```

服务器签发 issuerSignature(sessionId)，客户端自动签自己的 possession signature(sessionId)，再 `POST /api/v3/sessions` 注册。服务验证两份签名、公钥承诺、当前登录 accountId、签发者和最长七天期限。nonce 唯一性以 accountId、chainId、Registry、nonce 为键。撤销后重放注册不能复活。

`POST /api/v3/sessions/revoke` 支持 Session Key 对 `SHA("ZKPOKER_REVOKE_V3"||chainId.u64||registry||sessionId)` 签名；也可由登录身份撤销该身份拥有的会话。撤销与接受动作通过数据库锁串行。链上不获知未上链的撤销历史。

## 承诺与随机性

Spec 包含 domain、authIssuer、tableId、handId、players[9] 公钥承诺、sessionIDs[9]、规则与初始筹码、validUntil、actionLimit。截止最多一小时，动作数 1..64，所有 Session 覆盖截止；空座 identity/session/stacks 全为 0。同一登录身份不能占多个座位；公钥身份也必须唯一。

```text
context = SHA("ZKPOKER_CONTEXT_V3" || chainId.u64 || registry[20] || authIssuer[20]
 || tableId || handId || initial[9].u64 || button.u8 || small.u64 || big.u64 || ante.u64
 || players[9][32] || sessionIDs[9][32] || validUntil.u64 || actionLimit.u16)
seedCommit = SHA("ZKPOKER_SEED_V3" || context || seat.u8 || seed[32])
manifest = SHA("ZKPOKER_MANIFEST_V3" || context || serverCommit || playerCommits[9][32])
consent = SHA("ZKPOKER_CONSENT_V3" || sessionId || handId || manifest)
```

server seat=255，玩家 seat=0..8，按座位升序。服务器先承诺，再收玩家 Commit，冻结集合后收齐所有 Consent；客户端核对公钥身份/签发者/自有承诺/规则并验全部 Consent 后才 Reveal。每手新种子，缺席不得用 0 补齐。Commit/Reveal 分别签 `SHA("ZKPOKER_COMMIT_V3"||sid||handId||context||commit)` 和 `SHA("ZKPOKER_REVEAL_V3"||sid||handId||manifest||seed)`。

```text
mixed = SHA("ZKPOKER_MIX_V3" || context || serverSeed || concat(seat.u8||playerSeed))
priority[c] = SHA("ZKPOKER_CARD_V3" || mixed || c.u8)
```

52 张牌按 256-bit priority 升序排序，碰撞时按 cardId。随机预言机模型下除碰撞外等概率，碰撞界 `1326/2^256`，不声称数学上绝对无偏。`deckCommit=SHA("ZKPOKER_DECK_V3"||context||deck[52])`。

## 动作、观察和恢复

```text
initialState = SHA("ZKPOKER_INITIAL_V3" || context || manifest || deckCommit)
publicView = SHA("ZKPVIEW3" || concat(stack.u40||streetBet.u40) for 9 seats
 || foldedMask.u16 || currentBet.u40 || minRaise.u40 || board[5]
 || street.u8 || actor.u8 || previousSequence.u8)
holeCommit = SHA("ZKPOKER_HOLE_V3" || context || seat.u8 || playerSeed || hole[2])
state = SHA("ZKPOKER_ACTION_V3" || sid || handId || previousState || sequence.u64
 || seat.u8 || kind.u8 || amount.u64 || publicView || holeCommit)
resultCommit = SHA("ZKPOKER_RESULT_V3" || manifest || deckCommit || terminalState)
```

sequence 从 1 开始。kind：Fold0/Check1/Call2/Bet3/Raise4/All-in5；Bet/Raise amount 为本街总目标，其余为 0。street=Preflop0/Flop1/Turn2/River3。未发牌槽补 0；客户端先严格校验各街牌数 0/3/4/5、不同整数牌、整数筹码和合法 actor，避免补零歧义。底牌承诺以玩家种子遮蔽，不能只枚举两张牌反推。

服务器和电路从实际引擎状态独立回算视图与动作摘要。错误展示的合法签名仍不能用于正确实际状态；密码学不能证明恶意客户端 UI 确实向人展示了什么。

收据摘要为 `SHA("ZKPOKER_RECEIPT_V3"||handId||index.u64||at.u64||server[20]||previous||SHA(kind.utf8)||SHA(payload.rawBytes))`，服务器签名，payload 以 Base64 保留原始 JSON 字节。服务器自报时间不是可信时间源。

恢复授权 `SHA("ZKPOKER_READ_V3"||sid||handId||context||timestamp.u256)`，接受 ±30 秒，带 after/knownReceipt 验日志头、连续链和光标。自己的有效 Session 可读取同一 accountId 的历史底牌；动作必须用原 roster Session。HTTP 和 `/api/v3/ws` 都支持签名恢复与动作。完成后可下载 Audit，运行独立 `verify-signed`；运行中不得泄露种子。

## 完整证明与 Registry

Registry 构造 44 个字段：chainId、Registry、handId 两个 128-bit limbs、9 个公钥玩家 ID 各两个 limbs、9 初始筹码、9 最终筹码、resultCommit 两个 limbs、validUntil、不可变 authIssuer 地址。

Noir 约束：公钥→playerId；每个身份凭证的 accountId/kind/期限/域/SessionKey；不可变签发者的 ECDSA；客户端持钥签名；共同 Consent；种子与 hash-sort；签名动作及视图；发牌/Burn/完整下注/7 张牌/边池/筹码。accountId 是 witness 字段，不是链上公开输入；完成后的完整 Audit 会披露它。

`PokerHandRegistry(verifier,authIssuer)` 只验证 Proof 并记录 `bytes32[9] players`、初末筹码、协议承诺、截止和索引摘要，发 HandVerified。不要求玩家账户发送交易，不处理代币。过期、重复 HandID、非法 roster 或错误证明拒绝。正常完成一笔交易，不含部署。

这是新协议，API/哈希域/数据库表/VK/Registry 与 v2 钱包版隔离，不能复用旧 Proof。网络：本地 31337、Amoy 80002、Polygon 137；运营端主网工具要求同 VK 的 Amoy 成功记录。尚未完成真实测试网验收。
