# gb-533 执行与验证记录

## 基本信息

| 项目 | 实测值 |
| --- | --- |
| 项目编号 | `gb-533` |
| 项目名 | `robot-cell-safety-envelope-validator` |
| 验证完成时间 | 2026-08-22 06:10 CST |
| 实现提交 | `52b7965e10ca5782169c03b0d1a75423e3a9d6a7` |
| 前端端口 | `18533` |
| 后端端口 | `19533` |
| PostgreSQL 端口 | `57533` |
| SQLite runtime smoke 端口 | `20533` |
| Go 功能规模 | 3486 行 / 42 个 `.go` 文件 |

本记录只陈述本项目的真实执行结果。系统是离线工程决策支持工具，不连接控制器、不下发程序、不控制安全设备，也不构成现场运行许可或认证安全结论。

## 构建、静态检查与测试

下列命令均在本项目中真实执行并以退出码 0 完成：

| 范围 | 命令 | 结果 |
| --- | --- | --- |
| 根工作区 | `go work sync` | 通过 |
| 根工作区 | `go build ./backend/...` | 通过 |
| 根工作区 | `go vet ./backend/...` | 通过 |
| 根工作区 | `go test ./backend/...` | 通过 |
| 根工作区 | `go test -race ./backend/...` | 通过 |
| 后端目录 | `go build ./...` | 通过 |
| 后端目录 | `go vet ./...` | 通过 |
| 后端目录 | `go test ./...` | 通过 |
| 前端依赖 | `npm --prefix frontend ci` | 通过，安装 871 个包 |
| 前端类型 | `npm --prefix frontend run typecheck` | 通过 |
| 前端生产构建 | `npm --prefix frontend run build` | 通过，初始包 478.06 kB，预计传输 113.59 kB |
| 官方规模脚本 | `project_scale.py .` | 通过，3486 行 / 42 个功能 Go 文件，带前端页面 |
| 官方运行脚本 | `runtime_smoke.py .` | 通过，SQLite 服务在 `http://127.0.0.1:20533/healthz` 返回 HTTP 200 |

Go 测试覆盖包络几何、联锁依赖、状态机、RBAC/中间件等普通项目逻辑；race 检查未报告数据竞争。

本机 npm registry 的 audit 端点曾返回 `404 NOT_IMPLEMENTED`，因此没有可用的在线 audit 结论。这不影响本次 `npm ci`、TypeScript 类型检查和 Angular 生产构建均真实通过的结果。

## Compose 与健康检查

真实执行：

```text
docker compose config --quiet       PASS
docker compose up -d --build        PASS
docker compose ps                   db/backend/frontend 均 healthy
GET :19533/healthz                  HTTP 200, status=ok
GET :19533/readyz                   HTTP 200, database=available
GET :18533/api/healthz              HTTP 200（经 Nginx 原样代理）
```

Compose 使用 PostgreSQL 16；三个服务实际监听 `18533/19533/57533`。Browser 验收完成后，对后端结构化日志精确筛选 `level=error/fatal`、HTTP 5xx、panic，对 Nginx 日志精确筛选 HTTP 5xx、panic、fatal 和 `[error]`，两组结果均为空。

## API 冒烟

真实运行 `scripts/api_smoke.sh`，共 `38/38` 条 HTTP 检查通过。响应体断言还校验了状态、checksum、错误码、证据、幂等复用、风险保留、自审隔离和四实体审计投影。

| # | 检查 | 预期/实测 |
| ---: | --- | --- |
| 1 | 后端健康检查 | 200 / 200 |
| 2 | 未认证读取工作单元 | 401 / 401 |
| 3 | `safety_engineer` 登录 | 200 / 200 |
| 4 | `robot_programmer` 登录 | 200 / 200 |
| 5 | `reviewer` 登录 | 200 / 200 |
| 6 | `auditor` 登录 | 200 / 200 |
| 7 | `admin` 登录 | 200 / 200 |
| 8 | 工作单元列表 | 200 / 200 |
| 9 | 安全区域列表 | 200 / 200 |
| 10 | 运动程序列表 | 200 / 200 |
| 11 | 校验运行列表 | 200 / 200 |
| 12 | programmer 越权创建工作单元 | 403 / 403 |
| 13 | engineer 创建 `QA-CELL-533` | 201 / 201 |
| 14 | 工作单元详情 | 200 / 200 |
| 15 | 自交 Polygon 被拒绝 | 422 / 422，`invalid_geometry` |
| 16 | 创建 `QA restricted gate` | 201 / 201 |
| 17 | 启用安全区域 | 200 / 200，版本递增 |
| 18 | 旧版本重复启用 | 409 / 409 |
| 19 | engineer 越权导入程序 | 403 / 403 |
| 20 | programmer 导入 `QA-MOVE-533` | 201 / 201，SHA-256 checksum |
| 21 | 运动程序详情 | 200 / 200 |
| 22 | `uploaded -> active` 非法跳转 | 409 / 409 |
| 23 | `uploaded -> parsed` | 200 / 200 |
| 24 | `parsed -> ready` | 200 / 200 |
| 25 | `ready -> active` | 200 / 200 |
| 26 | 运行包络仿真 | 201 / 201，碰撞逐条证据 |
| 27 | 相同 Idempotency-Key 重放 | 200 / 200，复用原 run ID |
| 28 | 校验运行详情 | 200 / 200 |
| 29 | programmer 越权复核 | 403 / 403 |
| 30 | reviewer 写入独立复核 | 200 / 200，状态 `reviewed` |
| 31 | reviewer 接受证据 | 200 / 200，客观风险未被覆盖 |
| 32 | admin 导入自审隔离样本 | 201 / 201 |
| 33 | admin 解析自己的程序 | 200 / 200 |
| 34 | admin 标记自己的程序就绪 | 200 / 200 |
| 35 | engineer 校验 admin 程序 | 201 / 201 |
| 36 | admin 记录复核步骤 | 200 / 200 |
| 37 | 上传者尝试接受自己的结果 | 403 / 403，`forbidden` |
| 38 | auditor 读取审计流 | 200 / 200，四实体投影齐全 |

这组检查真实贯穿四个实体的 handler、service、repository 和数据库链路，同时覆盖 401、403、409、422 与成功分支。

## Codex 内置 Browser 验证

浏览器验证仅使用 Codex 内置 Browser；没有启动或使用外部 Chrome，也没有使用独立 Playwright。

1. 以 `engineer` 登录 `/cells`，通过真实表单创建 `CELL-B21`；列表刷新后显示 3 条记录。
2. 进入 `/zones`，选择 `QA-CELL-533`，确认 `QA restricted gate` 的 Polygon、高度和限速几何来自真实 API。
3. 以 `programmer` 导入 `UI-PICK-533` v1 到 `QA-CELL-533`，依次真实执行 `uploaded -> parsed -> ready -> active`。
4. 以 `engineer` 在 `/validation` 对 `UI-PICK-533` 运行 attempt 1；结果为 `failed`、风险 `22/100`，包含 1 个 restricted zone collision，实际/允许速度为 `280/100 mm/s`，并显示 `Acceptance is not permission to operate`。
5. 以 `reviewer` 写入独立复核意见，并真实推进 `reviewed -> accepted`；碰撞和风险证据仍保留。
6. 在 `/audit` 验证共 23 个事件，包含 `robot_cell.created`、`motion_program.uploaded`、三次 `motion_program.state_changed`、`validation_run.completed`、`validation_run.reviewed` 和 `validation_run.accepted`。Event #23 的 request ID 为 `1731b2664e26fc5e8c40b858d64ca591`，before/after 不可变投影正确。
7. 在 390 x 844 视口检查 `/audit` 与 `/validation`：`clientWidth = scrollWidth = bodyScrollWidth = 390`，无横向溢出，文字、按钮和内容区无阻断遮挡。
8. 内置 Browser 的 `tab.dev.logs()` 返回空数组；实际交互请求均取得预期 2xx/201，容器日志无 5xx。

五个页面均消费真实 `/api/v1` 数据；创建、导入、状态迁移、仿真、复核、接受和审计不是静态 mock。

## 截图

| 页面 | 视口 | 证据 |
| --- | --- | --- |
| 工作单元 | 1280 x 720 | [cells-desktop.jpg](cells-desktop.jpg) |
| 运动程序 | 1280 x 720 | [programs-desktop.jpg](programs-desktop.jpg) |
| 校验运行 | 1440 x 1000 | [validation-desktop.jpg](validation-desktop.jpg) |
| 审计中心 | 1440 x 1000 | [audit-desktop.jpg](audit-desktop.jpg) |
| 校验运行 | 390 x 844 | [validation-mobile.jpg](validation-mobile.jpg) |
| 审计中心 | 390 x 844 | [audit-mobile.jpg](audit-mobile.jpg) |

## 需求覆盖

| 要求 | 实现与验证 |
| --- | --- |
| 四核心实体全链路 | `RobotCell`、`SafetyZone`、`MotionProgram`、`ValidationRun` 均有 PostgreSQL 表和独立 Go model/dto/repository/service/handler/router、Angular type/api/store/page；API 冒烟逐一穿透验证 |
| 五核心页面 | `/cells`、`/zones`、`/programs`、`/validation`、`/audit` 均加载真实 API；Browser 完成跨角色业务流 |
| 共享前端能力 | `CellStateBadge`、`SafetyCanvas`、`FindingDrawer` 及 `useAuth`、`useValidationRun` 已复用 |
| 包络算法 | 工具与负载半径扩张、二维 Polygon 与高度区间求交、首次相交时间/轨迹段/速度/证据均落库并在 UI 回放；几何测试与实际碰撞流通过 |
| 联锁算法 | 缺失前置、顺序反转、循环依赖均输出逐条证据并有单元测试 |
| 状态与历史 | 程序和校验状态机执行条件更新；非法跳转 409；幂等复用及 retry 链保留旧尝试；快照和审计 before/after 不覆盖历史 |
| JWT 与 RBAC | 五类角色、JWT claims、后端中间件、Angular guard 和操作显隐联动；401/403 与上传者自接受隔离真实通过 |
| 横切能力 | request ID、recovery、auth、RBAC、audit、统一错误、限流、幂等、结构化日志和优雅停机均实现 |
| 技术与规模 | Go 1.22、Gin、GORM、PostgreSQL 16、SQLite smoke、Angular 17、Material、Signals、RxJS、Lucide；3486 行 / 42 个功能 Go 文件 |
| 部署 | `db/backend/frontend` 三服务 healthcheck、健康依赖、命名卷、固定端口和 Nginx `/api` 代理均真实运行 |
| 安全边界 | README 与 UI 明示二维加高度近似、误差边界、禁止控制器连接/控制命令及“接受不等于运行许可” |

## 修复摘要

- 将初始化过程中误升到 1.25 的 Go directive 恢复为需求指定的 Go 1.22，并重新完成构建、测试和 race 检查。
- 扩充种子校验快照，使区域、程序、碰撞和联锁证据完整可回放；随后通过 API 与 Browser 真实验证。

## 停服与残留检查

验收后真实执行：

```text
docker compose down -v --remove-orphans     PASS
docker compose ps -a                        仅表头，无服务
docker ps -a（项目名过滤）                  空
docker volume ls（项目名过滤）              空
docker network ls（项目名过滤）             空
lsof :18533/:19533/:57533/:20533            均无监听进程
```

仅删除本项目的三个容器、默认网络和 `robot-cell-safety-envelope-validator-postgres-data` 命名卷，未执行全局 prune。
