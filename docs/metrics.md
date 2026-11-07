GOS 通过一个 `GET /metrics` 输出发布治理和应用业务指标，供 Prometheus 定时抓取。所有值来自数据库或配置的数据源，接口不包含效果图中的演示数据。

## 启用接口

源码运行时在 JSON 配置中添加：

```json
{
  "metrics": {
    "enabled": true,
    "bearer_token": "replace-with-a-separate-metrics-token",
    "refresh_interval_sec": 30,
    "collection_timeout_sec": 5,
    "business_sources": []
  }
}
```

此片段需要合并到现有配置中。接口默认关闭，仓库的 `config.local.json` 已开启本地预览。指标访问鉴权可在“系统设置 → 系统管理 → 指标访问”中控制。开启鉴权时使用独立的 `bearer_token`，可读取整个部署的指标，不使用浏览器会话或 MCP Token。

### 在系统设置中切换鉴权

- **开启“指标访问鉴权”**：请求必须带正确的采集 Token。
- **关闭“指标访问鉴权”**：无需登录或 Token，直接访问 `/metrics` 即可获得指标；已配置的 Token 可保留。
- 在系统管理页签点击“保存”后当前服务立即生效，设置持久化在数据库中，重启后继续保留。其他副本每 5 秒同步一次设置。
- 未配置 Token 时可以关闭鉴权；由关闭切换到开启前，需要先配置 `metrics.bearer_token` 或 `GOS_METRICS_BEARER_TOKEN` 并重启加载凭据。

没有保存过开关时，非 `local` 环境默认要求鉴权；`local` 环境在配置了 Token 时默认要求鉴权。要求鉴权但 Token 为空时 `/metrics` 返回 `503`，管理员仍可进入设置关闭鉴权。数据库读取失败时继续使用最近成功加载的访问策略。开关只控制指标访问鉴权，接口是否启用仍由部署配置的 `metrics.enabled` 决定。

Docker 使用下面的环境变量；Compose 文件已转发这些变量：

```dotenv
GOS_METRICS_ENABLED=true
GOS_METRICS_BEARER_TOKEN=replace-with-a-separate-metrics-token
GOS_METRICS_REFRESH_INTERVAL_SEC=30
GOS_METRICS_COLLECTION_TIMEOUT_SEC=5
GOS_METRICS_BUSINESS_SOURCES=[]
```

单容器和独立前端的 Nginx 均代理 `/metrics` 并转发 Authorization。可从后端 `8081` 或前端映射端口访问：

```bash
curl -H "Authorization: Bearer ${GOS_METRICS_BEARER_TOKEN}" http://127.0.0.1:8081/metrics
```

成功返回 Prometheus 文本或 OpenMetrics；开启鉴权且未授权返回 `401`，鉴权开启但未配置 Token 返回 `503`，接口禁用返回 `404`，写操作返回 `405`。关闭鉴权时可以直接运行 `curl http://127.0.0.1:8081/metrics`。配置文件由服务端读取；原生运行不会自动读取 Docker 的环境变量。

## 维度和元信息

| 标签 | 数据来源与语义 |
| --- | --- |
| `project_id` | 应用当前所属项目。应用被移动项目后，历史事件的项目标签跟随当前归属。 |
| `application_id` | 稳定的应用 ID。名称和应用 Key 从元信息指标查询。 |
| `env` | 发布单目标环境、Agent 环境或业务源绑定环境；与 Gos 自身运行环境分开。 |
| `status` | 业务阶段或 Agent 运行状态，具体由指标定义。 |
| `scope` | `ci` / `cd`。 |
| `provider` | 执行记录中的提供者，例如 `jenkins` / `argocd` / `gitops` / `agent`。 |
| `result` | `success` / `failed` / `cancelled` / `skipped`。 |
| `trigger_type` | 创建时的触发方式，`manual` / `webhook` / `schedule` / `automation`；旧 `scheduled` 统一为 `schedule`。 |
| `operation` | 创建时的 `deploy` / `rollback` / `replay`。 |
| `source_id` | 业务源的稳定 ID，保留各数据源的累计值身份，区别于 Prometheus 抓取 Gos 的 `instance`。 |

除 Agent、平台采集健康和元信息外，发布指标均含 `project_id/application_id/env`。不把发布单号、提交 SHA、用户 ID、构建地址或错误全文作为标签。

`gos_project_info{project_id,project_key,project_name}=1`、`gos_application_info{project_id,application_id,application_key,application_name}=1` 用于显示名称。`gos_application_env_info` 表示从发布历史或业务源配置中发现的应用环境组合。尚未出现的组合不会被凭空生成。

## 指标清单

| 指标 | 类型 | 额外标签 / 口径 |
| --- | --- | --- |
| `gos_release_orders` | Gauge | `status`，当前保留单据的状态数量，与页面复用阶段推导。 |
| `gos_release_latest_status` | Gauge | `status`，各应用环境最新创建单据的状态；当前状态为 1，其余为 0。 |
| `gos_release_orders_created_total` | Counter | `trigger_type,operation`，创建事件，包括草稿；同一张单编辑不增加。 |
| `gos_release_executions_total` | Counter | `scope,provider,result`，每个执行记录首次具有完成时间的终态结果。 |
| `gos_release_execution_duration_seconds` | Histogram | `scope,provider`，真正有开始、结束时间的成功/失败执行。 |
| `gos_release_last_success_timestamp_seconds` | Gauge | 最近一次 CD 执行成功的 Unix 秒数；尚无成功记录时为 0。 |
| `gos_release_approval_pending` | Gauge | `gate`，待处理人工审批节点数；兼容旧的整单审批，避免重复计数。 |
| `gos_agents` | Gauge | `env,status`，在线、离线、忙碌、停用和维护中的 Agent 数量。 |
| `gos_agent_state` | Gauge | `agent_id,env,status`，单个 Agent 状态，当前状态为 1。 |
| `gos_agent_info` | Gauge | `agent_id,agent_code,agent_name,env`，节点元信息。 |
| `gos_agent_last_heartbeat_timestamp_seconds` | Gauge | `agent_id,env`，最近心跳，未收到心跳为 0。 |
| `gos_metrics_collection_success` | Gauge | 最近一次数据库采集是否成功。 |
| `gos_metrics_last_success_timestamp_seconds` | Gauge | 最近一次数据库采集成功的 Unix 秒数。 |
| `gos_metrics_collection_duration_seconds` | Gauge | 最近一次完整刷新耗时，含业务源采集。 |
| `gos_business_source_up` | Gauge | `project_id,application_id,env,source_id`，最近一次业务源采集是否成功。 |
| `gos_business_source_last_success_timestamp_seconds` | Gauge | 同上，业务源最近成功采集时间。 |

同时输出标准 `go_*` 和 `process_*` 指标。已知应用环境的发布状态、审批关口，已知执行器组合的终态结果，以及已知环境的 Agent 状态会输出明确的零值。

发布业务状态包括 `draft,pending_execution,pending_approval,approving,approved,queued,building,built_waiting_deploy,deploying,deploy_success,deploy_failed,rejected,cancelled`。审批拒绝不作为一次 CD 失败，仅构建成功也不作为一次 CD 成功。

耗时当前口径是执行全耗时，包含外部排队和状态追踪延迟。秒数桶边界为 `1,5,10,15,30,45,60,90,120,180,240,300,450,600,900,1800,3600,7200,+Inf`。P95 是桶估算值；没有观测样本时展示无数据。

## 接入订单、支付等业务数据

业务源返回权威的数字 JSON。例如应用的内部统计接口返回：

```json
{
  "orders": { "created_total": 10000, "pending": 28 },
  "payments": { "success_total": 9850, "failed_total": 150 }
}
```

把下面的源追加到 `metrics.business_sources`，替换成 Gos 中真实的应用 ID、环境和可访问的地址：

```json
{
  "id": "order-prod",
  "application_id": "replace-with-application-id",
  "env": "prod",
  "url": "http://order-service:8080/internal/business-stats",
  "bearer_token": "",
  "metrics": [
    { "name": "biz_orders_created_total", "help": "Created business orders.", "type": "counter", "path": "/orders/created_total" },
    { "name": "biz_pending_orders", "help": "Current pending business orders.", "type": "gauge", "path": "/orders/pending" },
    { "name": "biz_payment_attempts_total", "help": "Completed payment attempts.", "type": "counter", "path": "/payments/success_total", "labels": { "result": "success" } },
    { "name": "biz_payment_attempts_total", "help": "Completed payment attempts.", "type": "counter", "path": "/payments/failed_total", "labels": { "result": "failed" } }
  ]
}
```

每个数据源绑定一个应用和一个环境。订单和支付是两个应用时，分别配置各自数据源与指标。示例将字段放在同一响应中，仅演示 JSON 映射。Docker 把整个源数组作为 JSON 放入 `GOS_METRICS_BUSINESS_SOURCES`；配置和凭据保存在私有配置或部署系统，不提交到仓库。

`path` 使用 JSON Pointer，支持对象、数组及 `~0/~1` 转义。只接受有限数值，Counter 必须非负且名称以 `_total` 结尾；累计值在源进程重启时允许归零。按日清零的“今日订单数”应使用 Gauge，精确经营报表继续读取业务数据库。

同名指标的 help、type、标签名称必须一致；只允许配置静态标签，项目、应用、环境、source_id 等定位标签由 Gos 填充。每部署最多 32 个源，每源最多 64 个指标、每指标最多 8 个自定义标签。请求最多并发 4 个，单次完整刷新受 `collection_timeout_sec` 约束，响应上限 1 MiB，不跟随重定向。

某个源失败时不发布部分更新，保留该源上一次成功值，`gos_business_source_up=0` 且成功时间不前进。首次失败时该源的业务值保持缺失。业务指标图表应同时检查源健康和新鲜度；否则缓存值被重复抓取时可能被误认为业务没有变化。

数据库失败同样保留最近完整快照，并输出 `gos_metrics_collection_success=0`。`/metrics` 本身正常返回 `200`；Prometheus 的 `up` 只代表 Gos 指标出口可达，不等同于业务源健康。

## Prometheus 与 Grafana

抓取示例见 [monitoring/prometheus.yml](monitoring/prometheus.yml)，告警示例见 [monitoring/alerts.yml](monitoring/alerts.yml)。使用 30 秒抓取间隔与缓存刷新周期对齐。开启鉴权时通过部署系统挂载 Token 文件；关闭鉴权时可删除抓取配置中的 `authorization` 段。

在 Grafana 创建项目、应用、环境、执行器变量。项目和应用名称从 `*_info` 指标展示，变量实际值使用 ID。下面的查询中的 `$project/$application/$env/$provider` 设为支持多选并把 All 值设为 `.*`；`$__range/$__rate_interval` 为 Grafana 内置变量。

### 发布次数与成功率

```promql
# 所选时间内创建的发布单，包含草稿。部署次数请查询 CD 执行计数。
sum(increase(gos_release_orders_created_total{project_id=~"$project",application_id=~"$application",env=~"$env"}[$__range]))

# CD 完成次数（成功 + 失败）
sum(increase(gos_release_executions_total{project_id=~"$project",application_id=~"$application",env=~"$env",provider=~"$provider",scope="cd",result=~"success|failed"}[$__range]))

# CD 成功率，取消/跳过/审批拒绝不进入分母。没有样本时结果为空或 NaN，展示“无数据”。
100 * sum(increase(gos_release_executions_total{project_id=~"$project",application_id=~"$application",env=~"$env",provider=~"$provider",scope="cd",result="success"}[$__range]))
/ sum(increase(gos_release_executions_total{project_id=~"$project",application_id=~"$application",env=~"$env",provider=~"$provider",scope="cd",result=~"success|failed"}[$__range]))
```

创建量没有 provider 维度；在看板上应明确标记“创建量（含草稿）”。执行器筛选作用于 CI/CD 次数和耗时，不作用于创建量、审批或 Agent 面板。

### 耗时、审批、Agent 与业务指标

```promql
# CD P95，CI 改为 scope="ci"
histogram_quantile(0.95, sum by (le) (increase(gos_release_execution_duration_seconds_bucket{project_id=~"$project",application_id=~"$application",env=~"$env",provider=~"$provider",scope="cd"}[$__range])))

sum(gos_release_approval_pending{project_id=~"$project",application_id=~"$application",env=~"$env"})

# 忙碌节点也属于在线。Agent 面板只按 env 筛选，共享节点不重复分配给应用。
sum(gos_agents{env=~"$env",status=~"online|busy"})
sum(gos_agents{env=~"$env",status=~"online|busy|offline"})

# 订单量：每分钟，源不健康时隐藏对应缓存值
sum(rate(biz_orders_created_total{project_id=~"$project",application_id=~"$application",env=~"$env"}[$__rate_interval])
  and on(project_id,application_id,env,source_id)
  ((gos_business_source_up == 1)
    and on(project_id,application_id,env,source_id)
    (time() - gos_business_source_last_success_timestamp_seconds < 120))) * 60

# 支付成功率，源健康应通过同样的源级过滤处理
100 * sum(rate(biz_payment_attempts_total{project_id=~"$project",application_id=~"$application",env=~"$env",result="success"}[$__rate_interval]))
/ sum(rate(biz_payment_attempts_total{project_id=~"$project",application_id=~"$application",env=~"$env",result=~"success|failed"}[$__rate_interval]))

sum(biz_pending_orders{project_id=~"$project",application_id=~"$application",env=~"$env"}
  and on(project_id,application_id,env,source_id)
  ((gos_business_source_up == 1)
    and on(project_id,application_id,env,source_id)
    (time() - gos_business_source_last_success_timestamp_seconds < 120)))
```

应用明细表按 `(project_id,application_id,env)` 连接：`gos_application_info`、`gos_release_latest_status == 1`、CD Counter 的区间增长量、按应用环境聚合的 Histogram P95、`gos_release_last_success_timestamp_seconds`。排查链接使用应用 ID、环境和告警时间范围筛选发布单列表，再进入单据详情；发布单 ID 不进入时序标签。

## 数据一致性与部署注意事项

服务启动自动迁移 `gos_metric_release_event`，并对现有发布单和已完成执行进行一次幂等回填。以后创建单据、执行完成与事件记录在同一数据库事务提交。接口关闭期间仍记录事件。重启、重复追踪、删除发布单不会清空累计次数与耗时分布。每个执行记录的第一个有效终态是最终计数口径；重放产生新的执行记录后另计一次。

历史回填在首次抓取形成累计基线，不应被理解为首次启动时新发生的发布。Prometheus 的 `increase` 有采样和外推误差，无法补齐启用监控之前的历史曲线，也不是精确业务账本。

指标包含同一个数据库的全局业务数据。多副本部署应通过一个稳定逻辑出口抓取一份全局指标，不要把多个副本的相同全局指标相加。采集 Go/process 指标需要逐副本时，应另外过滤或分流全局业务指标。

事件表独立于发布单清理，不应手动删除；后续归档必须保留计数、耗时桶和最近成功时间。业务源若返回整个应用的全局总量，只配置一个源；多个源都返回同一总量会重复统计。只有互不重叠的实例或分片指标才可直接求和。
