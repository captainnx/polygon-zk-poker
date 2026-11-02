# v3 无钱包协议验收

日期：2026-10-23，Asia/Shanghai。本机 macOS arm64 / Apple M1 Pro。以下为实际本地执行结果；远端 CI、Polygon Amoy 和主网均未验收。旧钱包版记录保存在 `docs/history/acceptance-v2.md` 和 `reports/acceptance-v2.json`，不能替代本次结果。

## 本次变更

玩家以账号或游客登录，浏览器自动生成 secp256k1 Session Key，无需钱包扩展、EIP-712 或逐手人工签名。HTTP 认证、SDK、前端、Go 独立验证、Noir、Registry 和部署脚本都已迁移到 `signed-hand-v3`，数据库保留旧表并新增 v3 表。

认证服务为登录身份与客户端公钥签发凭证，客户端自动签署持钥证明。链上玩家 ID 是客户端公钥的 SHA256 承诺，电路同时约束它与持钥签名。即使签发者签署了凭证，也不能把已有玩家 ID 换绑到另一把密钥。登录账号与用户名归属仍依赖认证服务，ZK 不证明外部真实身份。

## 阶段状态

| 阶段 | 可运行内容 | 状态 |
|---|---|---|
| Phase 1 | 2–9 人下注、短加注、累计重开、All-in、边池、平局和筹码守恒 | Go 竞态回归、Go/Noir 差分通过 |
| Phase 2 | 账号/游客、密码哈希、Cookie、自动客户端密钥、身份凭证、签名动作、Commit-Reveal、冻结共同签名、状态哈希链、收据、撤销与恢复 | 本地服务及九客户端通过 |
| Phase 3 | 公钥身份、双方凭证签名、完整洗牌/下注/摊牌/边池/筹码关系；真实 UltraHonk ZK Proof | 正负 witness、原生证明及独立验证通过；未独立审计 |
| Phase 4 | 新生成的 Solidity Verifier、44 字段 Registry、HandVerified 事件、正常一手一笔 | 本地 EVM 真实 Proof 通过；Amoy 待部署 |
| Phase 5 | 5 游客 + 4 账号，自动签名，第 3 动作后重启恢复，独立回放、Proof、单笔登记和资源测量 | 本地端到端通过；公网测试网未验收 |

## 测试证据

| 检查 | 命令或证据 | 结果 |
|---|---|---|
| Go | `make test-go`；`reports/go-v3-test.log` | 单元/竞态与 vet 通过；新增登录、升级账号、过期、登出、错误密码、Cookie、跨源拒绝、IP 限流、身份替换攻击测试 |
| TypeScript/Web | `npm run build`、`npm test` | 构建通过；10 项测试通过，包括客户端自动凭证、恶意签发者换绑、公钥身份、签名向量、收据与加密备份 |
| Noir 库与演示 | `make test-noir` | 库 26 项、演示 3 项正负测试通过 |
| 游戏差分 | `make differential` | 固定种子 912342 的 40 组 2–9 人完整轨迹一致；包含边池、返还、牌型和最终筹码，错误输出拒绝 |
| 完整手牌 witness | `make test-hand` | 正常九人样例通过；9 类恶意 witness 全拒绝：动作签名、客户端持钥签名、签发者签名、身份声明、Consent 签名、牌序、非法下注、种子和守恒但错误边池输出 |
| 有效密钥的恶意动作 | `scripts/signed-adversarial.ts`，本地真实服务及 fixture 重放 | 九人四街 36 动作通过；非法 Check、Short Raise、错误底牌/公共牌观察分别被服务与电路拒绝，错误观察触发签名观察绑定 |
| Foundry | `make test-contracts` | 12 项通过，含 256 次筹码 fuzz。Registry 单元测试使用明确命名的 InputBindingDouble，真实 ZK 由端到端测试另行覆盖 |
| 完整 Proof/EVM | `scripts/full-e2e.ts`；`reports/full-e2e-v3.log` | 使用本次 v3 VK，原生生成、自验、独立验证通过；44 个应用字段与 Registry.statement 相同；事件与存储正确 |
| 篡改/重试 | `settle.ts local … --check-tampering` 及再次运行 | 独立 Go 回放通过后登记；损坏 Proof 与守恒但错误输出拒绝；再次运行读取已有结果，不重复发送 |
| 服务恢复 | 完整端到端脚本 | 第 3 动作重启 Docker server 后继续，Postgres 恢复成功；仅最终一笔登记交易 |
| Docker | `docker compose up -d --build` | PostgreSQL、Redis、server、Web 正常；v3 数据、服务端签名密钥持久化 |
| 浏览器 | 实际点击游客进入并刷新 | 无钱包直接显示“已自动创建会话”；Cookie 恢复相同游客身份并创建新会话；进行中旧牌局需原密钥备份的说明可见 |

密码使用 Argon2id，随机 HttpOnly/SameSite Cookie 仅存 token 哈希，签名 nonce/期限/域/撤销均受验证。客户端密钥在内存，网络断线重连继续使用原密钥；刷新后自动生成的新密钥不能替代进行中旧手牌的密钥，必须导入加密备份。账号找回、第三方登录和透明无交互的跨设备密钥同步未实现。

## 真实完整证明基准

本次容量 64、实际 9 人 9 动作、总筹码 6,950。原生 Barretenberg 两线程，ZK 开启，Keccak oracle，`--slow_low_memory --storage_budget 8g`。

| 指标 | 本机单次实测 |
|---|---|
| Proof | 11,712 bytes，44 个应用公开字段 |
| Prover 含自验 | 115,592 ms，约 115.6 秒 |
| Witness + prove + 独立 verify | 126,948 ms，约 126.9 秒 |
| Node 峰值 RSS | 1,303,150,592 bytes |
| 进程树采样峰值 RSS | 12,328,075,264 bytes，约 11.48 GiB，每秒采样 |
| 验证并登记 | 5,059,183 gas，Anvil chainId 31337 |
| 库 + Verifier + Registry 部署 | 7,663,182 gas |
| Verifier runtime | 24,089 bytes，未放宽 24,576 bytes 限制 |
| 电路域 | 8,388,608；生成 Verifier 含 16 个证明系统辅助字段，与 44 个应用字段有别 |

首次下载、编译、VK 生成、合约部署不计入 126.9 秒。数据不是最坏情况、吞吐或 Amoy 基准。标准 CI 只配置 witness/差分；高内存完整证明需自托管 runner，尚未实际运行远端 CI。

HandID：`0x58b171eb9f83087dd52a248a9958e2ea0c0967258573a0ef0796b38513f0ce08`。

本地交易：`0x4fcb33687dd3adab45ac7f73c341e17a0171fc8be0a117e688ab644fecfba68e`。这不是测试网交易。

ACIR SHA256：`19e70aab14f8bda4e605fd748419a56cf687a4c6f9b725335db6d18e054116ff`。

VK SHA256：`9935a88959095748f2270f89128dbccfb3e65936d6ffc54e2fe13b465b73c4c6`。

可复现输入：`fixtures/signed-hand-v3.json`、`signed-receipts-v3.json`、`adversarial-signed-hand-v3.json`、`engine-vectors.json`。机器可读摘要与证据指纹在 `reports/acceptance-v3.json`；完整 VK 指纹在 `contracts/verification-key.json`。原始随机 Proof、完整日志与私有工作目录忽略于 Git。

## 未完成验收

未发送 Amoy 或主网交易；尚缺测试网 RPC、已充值的运营端 keystore 与实际部署验收。v3 使用新 VK/Registry，不能沿用 v2 证明或测试网记录。主网部署仍要求相同 VK 的成功 Amoy 记录和显式开关。

仍需独立协议/电路/合约审计、测试网确认与重组/网络故障验证、生产认证/运维加固。服务器泄牌、串通、选择性中止和历史发布时刻不能由本电路排除。当前始终 `productionReady=false`。
