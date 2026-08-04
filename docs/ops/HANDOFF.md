# Stocker 部署与运维 Handoff

> 交接对象:在**任意一台新机器**上负责部署和日常运维的 agent/工程师。本文档自包含,按此操作无需读代码。
> 仓库:https://github.com/qiz029/stocker (分支 main)

## 〇、快速开始(新机器从零到跑起来)

```bash
git clone https://github.com/qiz029/stocker.git && cd stocker
# 1. 装依赖(见下表)  2. createdb stocker  3. 配 .env.local(可选,见一节)
./run-local.sh          # 后端 :8080 + 前端 :5173
```

## 一、运行需要什么

| 依赖 | 要求 | 备注 |
|------|------|------|
| Go | 1.26+(go.mod 声明 1.26.4) | 后端 `go run` 直跑,无需预编译 |
| Node + npm | Node 18+ | 前端 Vite;首次 `cd web && npm install` |
| Postgres | 已知可用版本 18(macOS:`brew install postgresql@18`;Linux:发行版对应包) | 唯一外部服务,本地即可 |
| 数据库 | `createdb stocker` | schema 由服务启动时自动迁移（内嵌 migration）,无需手动建表 |
| `.env.local` | 仓库根目录,gitignored,建议 chmod 600 | **可选**。缺失时服务照常运行,新闻退化为模板文案 |
| 外网 | 基本不需要 | 历史行情数据已 go:embed 进代码;只有 LLM 调用出网 |

### `.env.local`(启用 LLM 新闻文案才需要)

格式抄 `.env.local.example`。**key 绝不入 git、绝不写进任何文档/聊天记录**——从项目负责人处通过安全渠道单独获取,在目标机器上手工创建该文件:

```
LLM_BASE_URL=https://api.deepseek.com
LLM_API_KEY=<向负责人索取>
LLM_MODEL=deepseek-chat        # 当前实际使用 DeepSeek
LLM_DISABLE_THINKING=1         # 必须:deepseek-v4-flash 是推理模型,不禁用思考每批慢约 1 分钟
LLM_ROOM_BUDGET_SECS=360       # 必须:限流兜底,预算内尽量填充新闻正文
```

## 二、启动 / 停止

```bash
./run-local.sh          # 前台运行,Ctrl+C 同时停止前后端
```

- 打开 http://localhost:5173 即可玩;后端 API 在 :8080
- `DATABASE_URL` 默认 `postgres://localhost:5432/stocker?sslmode=disable`,可用环境变量覆盖

### ⚠️ 停止/重启后端的唯一正确姿势(踩过两次的坑)

```bash
kill $(lsof -t -i :8080)     # 对
# pkill -f 'cmd/server'      # 错!只杀 go run 父进程,编译出的子进程孤儿化
                             # 继续占 :8080,新服务 bind 失败但表面"正常"
```

前端同理:`kill $(lsof -t -i :5173)`。

## 三、部署后验证

```bash
# 1. 端口活着
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/api/scenarios   # 期望 401(未登录)——说明服务在
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:5173                 # 期望 200

# 2. 完整冒烟:浏览器打开 5173,注册→建房→启动时间轴走一遍
#    有 .env.local 时建房约 48 秒属正常(LLM 填充);无则秒回(模板文案)

# 3. 测试套件(可选,变更后跑)
cd web && npm test && npm run typecheck        # 前端:16 文件 54 用例(2026-07-27 时点)
createdb stocker_test 2>/dev/null || true      # 后端 DB 测试库(一次性)
cd ../server && STOCKER_TEST_DB='postgres://localhost:5432/stocker_test?sslmode=disable' go test ./...
# 不设 STOCKER_TEST_DB 时 DB 相关测试自动 skip,不算失败
```

## 四、已知行为与限流(不是故障)

- **建房 ~48 秒、新闻 LLM 填充率 ~90%**:DeepSeek key 账户级并发实测被限流到 ~5,`LLM_CONCURRENCY` 调高无效。正文在建房后异步升级(先模板后 LLM),属设计行为
- **游戏内时钟 2-4 小时一跳、有收盘和周末**:模拟市场时钟(PR #6,2026-07-27),纯前端展示,跳动不匀速是特性不是 bug
- **四个剧本**:dotcom-2000 / crash-1987 / nifty-1972 / gfc-2008,数据全部内嵌,运行时不拉外部行情
- **每房间 5 个 Agent 玩家**:迁移会给已有房间补齐，新房间自动加入；后端每 15 秒检查一次，每个 Agent 每个交易日最多下单一次，排行榜、房间动态与终局回放均有 Agent 标识
- Stooq 数据源有 JS 反爬已弃用;若要重抓数据用 `cmd/pipeline fetch`(Yahoo chart API,一次性操作,平时不用)

## 五、常用运维操作

```bash
# 重启后端(改了 server 代码后)
kill $(lsof -t -i :8080); (cd server && go run ./cmd/server &)

# 重置数据库(清空所有房间/用户;schema 下次启动自动重建)
kill $(lsof -t -i :8080); dropdb stocker && createdb stocker

# 看后端在不在 / 谁占着端口
lsof -i :8080
```

## 六、红线

- 永不提交 `.env.local` / key;本文档与聊天记录中也不得出现 key 明文
- 不改 `server/internal/engine/fidelity.go` 的保真门阈值(冻结,历史见 git log);剧本过不了门改数据不改门
- 运维操作不动 `docs/superpowers/` 下的 spec/plan 历史文档
