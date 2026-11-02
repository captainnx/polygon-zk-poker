# Polygon ZK Poker

2–9 人 No-Limit Texas Hold’em 链下游戏与 Polygon 结果验证系统。**玩家使用账号或游客登录，无需钱包、Approve 或手工签名。** 浏览器自动生成 Session Key；游戏结束后的链上交易由运营端账户提交。仅记录整数筹码，不转移真实代币。开发版本，未经独立安全审计。

## 玩家入口

1. 打开本地页面，点击「游客进入」，或注册／登录账号。
2. 客户端自动生成本地 Session Key，并取得身份凭证和入桌编号。
3. 分享入桌编号，由建桌方创建 2–9 人牌局，再分享 Hand ID。玩家加载并确认规则后，客户端自动 Commit、Consent、Reveal；游戏动作自动签名。
4. 手牌完成后可以下载 Audit 与签名收据。运营端生成 Proof 并用一笔交易登记结果。

账号名为 3–32 位字母、数字或下划线，密码至少 12 字节。游客可以注册为账号并保留原登录身份。密钥仅在客户端内存，刷新前可导出加密备份；恢复进行中牌局必须导入原密钥。账号重新登录可以读取该账号历史牌局，不能替代旧密钥为进行中的牌局签名。

登录身份与链上玩家 ID 分开：登录身份由服务器认证，**链上玩家 ID 是客户端公钥的 SHA256 承诺**。凭证绑定二者；电路同时验证公钥承诺、客户端持钥证明和签发者签名。因此认证服务器不能用另一把密钥冒充已经入桌的公钥身份。账号名称归属仍依赖认证服务，不是去中心化身份系统。

## 本地运行

需要 Go 1.25+、Node 24+、Docker Compose。完整证明建议至少 16 GiB 可用内存，首次工具与 SRS 下载需要联网。

```bash
make bootstrap
make test
make differential
docker compose up -d --build
# 页面 http://127.0.0.1:5173
# API  http://127.0.0.1:8080/healthz
make signed-demo
make full-e2e
```

`full-e2e` 使用 5 名游客、4 个账号自动生成各自的密钥，部署真实 Verifier/Registry，在第 3 个动作后重启服务并恢复，再生成完整 UltraHonk ZK Proof、验证、登记和检查 HandVerified。合约部署不计入每手一笔。

默认页面连接本地开发桌；未配置 `DEFAULT_REGISTRY_ADDRESS` 时仅为链下演示域，不能据此认为已有链上部署。运营端应在部署后配置正确的 Registry、chainId 与认证签发者；玩家无需了解钱包或提供私钥。HTTP 服务不直接接受高内存证明任务，独立 prover/relayer 由运营端运行。

## 验证与部署

```bash
# 完整身份凭证、动作签名、随机性、下注和筹码独立回放
go run ./apps/server/cmd/verify-signed fixtures/signed-hand-v3.json
# 正负 witness + 36 动作完整多街轨迹
make test-hand
# 证明某份已完成 Audit
node apps/prover/src/cli.ts prove-hand /absolute/path/audit.json
```

Audit 含完成后的全部种子和牌序，发布即披露该手全部牌；运行中不能下载。外部保留的签名收据用于检查承诺与日志分叉，不能证明服务器自报时间。

复制 `.env.example` 到本地 `.env`，由运营端配置 RPC 和加密 Foundry keystore：

```bash
make prepare-hand
node --env-file=.env scripts/deploy.ts amoy
node --env-file=.env scripts/settle.ts amoy /absolute/path/audit.json
```

远程交易用 `cast --account`，不要求玩家持有钱包。部署记录在 `deployments/v3/`。主网要求显式开关和当前 VK 的 Amoy 成功记录。**当前尚无 Amoy 部署验收，生产状态始终 false。**

## 工程边界

- Go 完整规则：大小盲、每人 Ante、Check/Call/Bet/Raise/Fold/All-in、短加注与累计重开、未跟注返还、边池、平局/余筹和筹码守恒。
- v3 每手 1–64 动作上限、最多一小时；超过上限或超时则中止，不伪造 Fold。每座初始筹码不超过 `10^12`。
- `bet/raise.amount` 为本街累计目标，其余动作 amount=0。Card=`rankIndex×4+suitIndex`，rank=`23456789TJQKA`，suit=`cdhs`。
- 登录使用 Argon2id 密码哈希、随机 HttpOnly/SameSite Cookie；数据库只存登录 token 哈希。生产反向代理需 HTTPS 并启用 `COOKIE_SECURE=true`。
- Postgres 保存认证身份、会话、AES-GCM 快照和原子追加签名日志；Redis 仅缓存公开状态。
- 完整 Noir 约束公钥身份、服务端身份凭证、随机洗牌、所见牌面、逐动作转移、7 张牌比较、边池与初末筹码。Registry 只验证和记录，不托管资金。
- 服务器泄牌、串通和选择性中止不能由此 ZK 关系排除。跨手资金账本、经济惩罚、自动续桌、第三方账号平台和密码找回未实现。

SDK：`@zkpoker/sdk/v3`，加密备份：`@zkpoker/sdk/backup`。主工程已迁移到 `signed-hand-v3`：新的 44 个公开字段与 v2 钱包版不兼容，须重新生成 VK 和部署 Registry。旧数据库表未删除；旧版验收仅保留在 `docs/history/`，不能当成 v3 的测试结果。

[架构](docs/architecture.md) · [协议](docs/protocol.md) · [威胁模型](docs/threat-model.md) · [本轮验收](docs/acceptance.md)
