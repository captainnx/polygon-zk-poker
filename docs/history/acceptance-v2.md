# Phase 2–5 验收记录

日期：2026-10-21，Asia/Shanghai。本机 macOS arm64 / Apple M1 Pro，工具版本见 `reports/toolchain.json`。以下为实际执行结果；远端 CI 尚未运行，Amoy 尚未部署。本地链结果不能代替测试网验收。

## 阶段结果

| 阶段 | 当前完成内容 | 验收边界 |
|---|---|---|
| Phase 1 | 2–9 人完整 Go 状态机，全部下注/短加注/累计重开/All-in/边池/平局/守恒 | 第一轮已有穷举与模糊测试，本轮继续通过竞态回归 |
| Phase 2 | 钱包 EIP-712 → Session Key；Go 验签、nonce/撤销/期限；冻结承诺共同签名后 Reveal；签名所见状态；Postgres 原子记录、AES-GCM 快照、Redis 公开缓存；签名收据、幂等重发、HTTP/WS 恢复 | 本地验收通过。选择性中止仅可保留证据，不能由 ZK 强制防止 |
| Phase 3 | 完整静态 64 动作 Noir 关系：身份/授权/Consent/动作签名、种子→洗牌、公开状态/底牌承诺、完整引擎/摊牌/边池/最终筹码；真实 UltraHonk ZK Proof | 本地 witness、差分及完整 Proof 验证通过；独立约束审计待完成 |
| Phase 4 | 实际生成 Solidity Verifier/VK，Registry 记录全部公开结果并发事件，正常一手一笔；部署/结算工具与主网前置门禁 | 本地真实 EVM 通过；Amoy 实际部署待 RPC 和已充值 keystore |
| Phase 5 | 9 个独立模拟钱包客户端 → 自动签名 → 第 3 动作重启服务 → Postgres 恢复 → 独立 Audit → 完整 Proof → 本地登记；资源/Gas 测量 | 本地完整链路通过；测试网确认/重组/公网资源测量未执行 |

## 本轮测试

| 检查 | 执行方式 | 实际结果 |
|---|---|---|
| Go 单元/竞态/覆盖率 | `go test -race -coverprofile=reports/coverage-v2.out ./apps/server/...` | 全通过；engine 94.9%、fairness 88.3%、httpapi 64.7%、table 71.6% |
| Go 静态检查 | `go vet ./apps/server/...` | 通过 |
| 协议规则与攻击 | table 测试 | 9 人完整签名、重新创建 Service 恢复、原 ACK 重发、错误光标、签名/种子/Manifest/结果篡改、过早 Reveal、冻结后改 Commit、过期/nonce 冲突、8 并发相同动作只执行一次、撤销不被授权重试复活、截止中止、动作上限中止、原 Session 动作权限均通过 |
| 所见状态绑定 | 合法签名签署错误 PublicView / HoleCommit | 两种请求均拒绝，之后合法动作正常执行；SDK 同样确认摘要不可替换 |
| HTTP/WS | httptest + 实际 Docker API | 翻前空牌面统一为空数组并有回归测试；HTTP 钱包授权/建桌/恢复、WS 认证恢复、错误签名与跨源拒绝；九客户端走实际 Postgres/Redis 服务 |
| SDK / 前端 | `npm run build`、`npm test` | 类型及 Web 构建通过，9 项测试通过；含 EIP-712、v2 Go/Noir 向量、签名收据、口令错误/密文篡改拒绝 |
| Noir 库及演示 | `make test-noir` | 6 个库共 26 项 + 牌型演示 3 项，总 29 项正负测试通过 |
| Go/Noir 差分 | `make differential` | 固定种子 912342，40 组 2–9 人完整轨迹，普通 Raise/Fold/All-in/Runout/牌型/边池/最终输出一致；错误输出拒绝 |
| 完整电路 | `make test-hand` | 完整签名样例成功；7 类恶意 witness 全拒绝：动作/钱包/Consent 签名、牌序、非法加注、玩家种子、守恒但错误边池输出 |
| 有效签名的恶意动作 | `node scripts/signed-adversarial.ts` 及 `--fixture` | 另跑九人四街 36 动作，正确 Session Key 签署非法 Check、Short Raise、翻牌后错误公共牌与错误底牌；Go 服务均拒绝，完整 Noir 分别在预期规则/观察约束拒绝，不依赖损坏签名 |
| Foundry Registry | `make test-contracts` | 12 项通过，含 256 次筹码 fuzz；这些单元测试使用明确命名的 InputBindingDouble，不视为 ZK 测试 |
| 真正的完整手牌 Proof | `node scripts/full-e2e.ts` | 使用本轮生成的完整 VK/Verifier；原生证明生成、自验及独立原生验证通过，34 个公开输入与 Registry.statement 完全一致 |
| 正常一手一笔 | 同上，临时 Anvil :18545 | 9 玩家、9 动作、6,950 总筹码；游戏期间无链上交易，最终仅一笔 verifyHand；HandVerified 与存储结果正确，重复 HandID/守恒但错误输出拒绝 |
| 服务重启 | 同上 | 第 3 个动作后 `docker compose restart server`，继续签名和恢复成功，未重发扣筹码 |
| 独立部署和结算 | `deploy.ts local`、`settle.ts local … --check-tampering` | 正常大小限制下部署，真正验签回放后登记；损坏 Proof/错误公开输出模拟拒绝，重复运行返回已有结果；伪造动作 Audit 在发送前退出 |
| 工具链指纹 | `verifyManifest` | 整理后重编译仍匹配已验证 VK/ACIR/生成 Solidity；无重新解释旧证明的情况 |
| Docker | `docker compose up -d --build` | PostgreSQL/Redis/server 健康，Web 可访问；协议表/持久卷/签名服务器密钥接通 |
| 浏览器 | 实际点击引擎运行与 v2 加载 | 九人回放、真实 v2 手牌公共牌/最终筹码显示正常；未授权不显示 Hole；无扩展时提示安装钱包。未在浏览器中签署真实钱包授权，自动签名由九客户端覆盖 |
| npm 审计 | `npm audit --audit-level=low` | 当次报告 0 vulnerabilities；不代表独立安全审计 |

Go protocol/store 由 table 与真实 Docker 集成测试间接调用；上述默认逐包覆盖率不计跨包调用，因此其 0% 不是未执行，但也不能当作完整覆盖。CLI main 不计有效单元覆盖。没有用虚构覆盖率替代实测。

第一轮引擎证据保留：全部 2,598,960 个五张组合分类频数为 `[1302540,1098240,123552,54912,10200,5108,3744,624,40]`。普通 Bet/Raise 版本守恒 fuzz 在 30 秒执行 1,361,818 次，非法动作原子性 fuzz 在 15 秒执行 1,010,870 次，均无失败；本轮引擎逻辑没有变化，未重复这两项长时间 fuzz。

## 完整证明基准

本轮测量对象为完整 signed-hand-v2、容量 64、实际九人九动作手牌；不是第一轮局部牌型演示。原生 Barretenberg 两个线程，ZK 开启，Keccak oracle，`--slow_low_memory --storage_budget 8g`。

| 指标 | 本机实际值 |
|---|---|
| Proof | 11,712 bytes；34 个应用公开字段 |
| Prover 时间 | 119,138 ms（约 119.1 秒），native prove 含自验 |
| Witness + prove + 独立 verify 总时长 | 127,869 ms（约 127.9 秒） |
| Node 峰值 RSS | 1,289,338,880 bytes |
| 进程树采样峰值 RSS | 10,797,744,128 bytes（约 10.06 GiB），每 1 秒采样，含 Node/原生子进程，不含整个系统 |
| 验证并登记交易 | 5,058,535 gas，Anvil chainId 31337 |
| Transcript 库 + Verifier + Registry 部署总计 | 7,654,435 gas |
| Verifier Runtime | 24,089 bytes，solc 0.8.30、paris、optimizer runs=1，未放宽 24,576 bytes 限制 |
| 电路域 | 8,388,608；生成代码内部还包含证明系统辅助输入，不应与 34 个应用字段混淆 |

这是单机器单次观测，不是最坏情况、吞吐或 Amoy 基准。首次 SRS 下载、编译/VK 生成、合约部署不计入 prover 127.9 秒。高内存 full-proof CI 仅配置给自托管 runner；标准 CI 运行 witness 与差分，不假称普通 runner 已跑完整证明。

本轮完整测试 HandID：`0x4f0e7d79eeae5c2bf027b79eed6c3b1dc0080f50b65f237b0d6ed9c43783cf76`。本地登记交易：`0x6a1b37bf55322c8730bc89bf9d3abdb82d79924eb575967daa0d771a24d8bb3a`。这是本地链交易，没有测试网浏览器链接。

ACIR bytecode SHA256：`520c333c03123ee0bfa6b4c0ecca4806a52bed304e82deead2b02032a042b95f`。

VK SHA256：`b716438d6cf930657e7cf70d4b12b0804625f013afa4a5122d7a5f6133b8ed7f`。

完整指纹在 `contracts/verification-key.json`。可复现样例在 `fixtures/signed-hand-v2.json`、`fixtures/signed-receipts-v2.json` 、`fixtures/adversarial-signed-hand-v2.json` 和 `fixtures/engine-vectors.json`。本机原始证据在 `reports/full-e2e.log`、`full-e2e.json`、`full-hand-proof.json`、`full-hand-witness-test.log`、`go-v2-test.log`、`noir-v2-test.log`、`differential-v2.log`、`settle-local.log`、`signed-adversarial.log`。随机 Proof、私有工作目录与完整日志忽略于 Git；`reports/acceptance-v2.json` 保存实际验收摘要与证据指纹，可随固定版本纳入 Git。

## 未完成的外部验收

未配置 `AMOY_RPC_URL` 和已有加密 Foundry keystore 账户，未发送任何 Amoy 或主网交易。`deploy.ts amoy` 已实现部署、九人测试、证明、提交、保存/恢复进度；只有实际链上记录正确后才写 verified=true。主网要求该 VK 的成功 Amoy 记录和显式开关，当前不会满足测试网前置条件。

仍需：实际 Amoy 部署/确认/Gas 与 RPC 测量、重组/网络故障验证、独立协议/电路/合约审计、钱包扩展 UI 验收与生产运维加固。服务器泄牌、串通、选择性中止不能通过当前 ZK 关系排除。当前生产状态始终 false。
