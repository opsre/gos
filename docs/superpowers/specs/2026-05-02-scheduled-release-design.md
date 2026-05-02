# 预约发布设计

## 1. 背景

GOS 当前以发布单为发布治理入口，已经具备发布单创建、审批、执行前预检、CI/CD 分段执行、并发锁、执行追踪、取消、回滚和重放能力。现有领域模型中已经存在 `trigger_type = schedule`，但缺少真正的预约单、预约审批、到点调度和预约列表管理能力。

本设计新增“预约发布”能力。用户先创建发布单，再为该发布单创建预约单。预约单先走审批，审批通过后进入待触发状态；到预约时间后由后台调度器复用现有 `Build`、`Deploy`、`Execute` 入口触发执行。

## 2. 已确认决策

- 采用“预约已有发布单”方案，不在一期做周期性自动建单。
- 一个发布单同一时间只允许一个 active 预约单。
- 预约单可以表达 CI、CD、CI+CD、全流程执行。
- 预约审批复用发布模板里的审批人配置。
- 创建预约单后先进入预约审批，审批通过后才允许到点触发。
- 预约单审批通过后不可编辑。如需调整，只能取消后重新创建并重新审批。
- 未审批通过且预约时间已到的预约单进入失效状态。
- 前端新增独立“预约发布”列表页。
- CI 与 CD 预约并存时，CD 时间必须晚于 CI 时间。
- 同一应用、同一环境下，两个 active 预约单的 CD 风险时间不能相同。后创建或编辑的一方直接拦截。
- `execute` 没有单独 CD 时间，使用执行开始时间作为 CD 风险时间参与互斥。

## 3. 目标

1. 支持用户对已有发布单创建一次预约发布计划。
2. 支持预约 CI 阶段、CD 阶段、CI+CD 分阶段、全流程执行。
3. 预约行为必须先审批，审批人复用发布模板配置。
4. 到点触发必须复用现有预检、审批、并发锁和执行入口。
5. 提供独立预约发布列表，便于查看待审批、待触发、已触发、已失效和失败预约。
6. 对同应用同环境的 CD 风险时间做强互斥，避免生产发布窗口撞车。

## 4. 非目标

- 不做周期性预约规则，例如每天、每周、cron 表达式。
- 不做审批通过后的预约信息编辑。
- 不绕过现有发布单预检和并发锁。
- 不在预约到点后自动顺延业务阻断场景。
- 不新增一套独立执行器，调度器只负责到点调用现有发布执行入口。

## 5. 核心概念

### 5.1 预约单

预约单是发布单下的一张独立审批工单。它不替代发布单，只描述“何时触发发布单的哪个执行动作”。

新增领域对象 `ReleaseOrderSchedule`，对应表 `release_order_schedule`。

### 5.2 预约模式

`schedule_mode` 可选值：

| 值 | 含义 | 触发入口 |
| --- | --- | --- |
| `build` | 只预约 CI 构建 | `ReleaseOrderManager.Build` |
| `deploy` | 只预约 CD 部署 | `ReleaseOrderManager.Deploy` |
| `build_deploy` | 分别预约 CI 构建和 CD 部署 | 到 CI 时间调用 `Build`，到 CD 时间调用 `Deploy` |
| `execute` | 预约全流程执行 | `ReleaseOrderManager.Execute` |

### 5.3 CD 风险时间

CD 风险时间用于同应用同环境预约冲突判断：

| 预约模式 | CD 风险时间 |
| --- | --- |
| `build` | 空 |
| `deploy` | `deploy_scheduled_at` |
| `build_deploy` | `deploy_scheduled_at` |
| `execute` | `execute_scheduled_at` |

## 6. 数据模型

新增表 `release_order_schedule`：

```sql
CREATE TABLE release_order_schedule (
  id VARCHAR(64) PRIMARY KEY,
  schedule_no VARCHAR(64) NOT NULL,
  release_order_id VARCHAR(64) NOT NULL,
  release_order_no VARCHAR(64) NOT NULL,
  application_id VARCHAR(64) NOT NULL,
  application_name VARCHAR(255) NOT NULL,
  env_code VARCHAR(64) NOT NULL,
  template_id VARCHAR(64) NOT NULL,
  template_name VARCHAR(255) NOT NULL,
  schedule_mode VARCHAR(32) NOT NULL,
  build_scheduled_at DATETIME NULL,
  deploy_scheduled_at DATETIME NULL,
  execute_scheduled_at DATETIME NULL,
  cd_conflict_at DATETIME NULL,
  timezone VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  approval_required BOOLEAN NOT NULL,
  approval_mode VARCHAR(32) NOT NULL,
  approval_approver_ids_json TEXT NOT NULL,
  approval_approver_names_json TEXT NOT NULL,
  approved_at DATETIME NULL,
  approved_by VARCHAR(255) NOT NULL,
  rejected_at DATETIME NULL,
  rejected_by VARCHAR(255) NOT NULL,
  rejected_reason TEXT NOT NULL,
  build_dispatched_at DATETIME NULL,
  deploy_dispatched_at DATETIME NULL,
  execute_dispatched_at DATETIME NULL,
  expired_at DATETIME NULL,
  cancelled_at DATETIME NULL,
  cancelled_by VARCHAR(255) NOT NULL,
  last_error TEXT NOT NULL,
  remark TEXT NOT NULL,
  creator_user_id VARCHAR(64) NOT NULL,
  creator_name VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
```

新增索引：

```sql
CREATE INDEX idx_release_order_schedule_order_status
  ON release_order_schedule (release_order_id, status);

CREATE INDEX idx_release_order_schedule_due
  ON release_order_schedule (status, build_scheduled_at, deploy_scheduled_at, execute_scheduled_at);

CREATE INDEX idx_release_order_schedule_cd_conflict
  ON release_order_schedule (application_id, env_code, cd_conflict_at, status);
```

说明：

- “同一发布单同一时间只有一个 active 预约单”由业务层在事务内强校验。MySQL 和 SQLite 都使用普通索引辅助查询；如目标数据库支持 partial unique index，可以额外增加只覆盖 active 状态的唯一约束作为防线。
- `cd_conflict_at` 由业务层根据 `schedule_mode` 计算并持久化，便于列表筛选和冲突查询。

新增表 `release_order_schedule_approval_record`，结构参考现有 `release_order_approval_record`：

```sql
CREATE TABLE release_order_schedule_approval_record (
  id VARCHAR(64) PRIMARY KEY,
  schedule_id VARCHAR(64) NOT NULL,
  action VARCHAR(32) NOT NULL,
  operator_user_id VARCHAR(64) NOT NULL,
  operator_name VARCHAR(255) NOT NULL,
  comment TEXT NOT NULL,
  created_at DATETIME NOT NULL
);
```

## 7. 状态机

预约单状态 `status`：

| 状态 | 含义 |
| --- | --- |
| `pending_approval` | 待提交或待审批 |
| `approving` | 审批中 |
| `scheduled` | 审批通过，等待触发 |
| `dispatching` | 调度器已抢占，正在触发 |
| `dispatched` | 预约动作已全部发起 |
| `expired` | 到点仍未审批通过或已错过有效触发窗口 |
| `blocked` | 到点后业务条件不满足，例如发布单状态不允许执行 |
| `failed` | 系统异常或执行入口调用异常 |
| `skipped` | 目标阶段在预约触发前已被人工完成 |
| `cancelled` | 用户取消 |
| `rejected` | 审批拒绝 |

active 状态：

```text
pending_approval
approving
scheduled
dispatching
```

active 状态参与“每个发布单只能有一个预约单”和“同应用同环境 CD 时间互斥”判断。

状态流转：

```text
pending_approval -> approving
pending_approval -> scheduled      // 自动通过或无需审批
approving -> scheduled
pending_approval -> rejected
approving -> rejected
pending_approval -> expired        // 到点仍未审批通过
approving -> expired               // 到点仍未审批通过
scheduled -> dispatching
dispatching -> dispatched
dispatching -> scheduled           // build_deploy 的 CI 已发起，继续等待 CD 时间
dispatching -> blocked
dispatching -> failed
dispatching -> skipped
pending_approval -> cancelled
approving -> cancelled
scheduled -> cancelled
```

## 8. 审批设计

### 8.1 审批配置来源

预约审批复用发布单所选发布模板里的审批配置：

- `approval_enabled`
- `approval_mode`
- `approval_approver_ids`
- `approval_approver_names`

创建预约单时，将模板审批配置快照写入预约单。后续模板变更不影响已创建预约单。

### 8.2 自动通过

沿用现有发布单自动审批语义。如果模板不需要审批，或者现有规则判断可自动审批，则预约单创建后直接进入 `scheduled`。

### 8.3 审批通过后锁定

预约单进入 `scheduled` 后不可编辑核心字段：

- `schedule_mode`
- `build_scheduled_at`
- `deploy_scheduled_at`
- `execute_scheduled_at`
- `timezone`
- `remark`

如需调整，用户必须取消当前预约单并重新创建。

### 8.4 审批超时失效

调度器扫描时，如果预约单仍处于 `pending_approval` 或 `approving`，且其最早预约时间已经到达，则将预约单置为 `expired`。

错误信息：

```text
预约时间已到，但预约审批未通过
```

## 9. 创建和编辑规则

### 9.1 创建预约单

创建前校验：

1. 发布单存在且当前用户可见。
2. 当前用户拥有发布单所属应用和环境的发布创建或预约权限。
3. 发布单下不存在 active 预约单。
4. 发布单未处于终态取消、失败、成功、拒绝等不可预约状态。
5. 预约时间必须是未来时间。
6. `schedule_mode` 与时间字段匹配。
7. 通过同发布单内部阶段规则。
8. 通过同应用同环境 CD 时间互斥规则。

### 9.2 编辑预约单

只有 `pending_approval` 和 `approving` 状态允许编辑。

编辑后：

- 清空已有审批记录。
- 状态回到 `pending_approval`。
- 重新快照发布模板审批配置。
- 重新计算 `cd_conflict_at`。
- 重新执行冲突校验。

`scheduled` 及之后状态不可编辑。

### 9.3 取消预约单

允许取消状态：

```text
pending_approval
approving
scheduled
```

取消后：

```text
status = cancelled
cancelled_at = now
cancelled_by = current_user
```

已进入 `dispatching` 后不允许取消预约单。此时如需终止执行，应使用发布单取消能力。

## 10. 冲突规则

### 10.1 同一发布单唯一预约

同一发布单同一时间只允许一个 active 预约单。

active 状态：

```text
pending_approval
approving
scheduled
dispatching
```

非 active 预约单不阻止重新创建：

```text
dispatched
expired
blocked
failed
skipped
cancelled
rejected
```

### 10.2 模式与时间字段匹配

| 模式 | 必填时间 | 禁止填写 |
| --- | --- | --- |
| `build` | `build_scheduled_at` | `deploy_scheduled_at`, `execute_scheduled_at` |
| `deploy` | `deploy_scheduled_at` | `build_scheduled_at`, `execute_scheduled_at` |
| `build_deploy` | `build_scheduled_at`, `deploy_scheduled_at` | `execute_scheduled_at` |
| `execute` | `execute_scheduled_at` | `build_scheduled_at`, `deploy_scheduled_at` |

### 10.3 阶段顺序

`build_deploy` 模式下：

```text
deploy_scheduled_at > build_scheduled_at
```

增加最小间隔配置，默认 5 分钟：

```text
deploy_scheduled_at - build_scheduled_at >= min_stage_interval
```

### 10.4 阶段能力

`build` 和 `build_deploy`：

- 发布单必须有 CI 执行单元。
- 发布单不能已经完成 CI 构建。

`deploy`：

- 发布单必须有 CD 执行单元。
- 发布单可以已经处于 `built_waiting_deploy`。
- 如果发布单尚未完成 CI，只有当预约单模式为 `build_deploy` 时才允许预约 CD。

`execute`：

- 发布单必须存在 pending 执行单元。
- 如果发布模板同时有 CI/CD，则 `execute` 表示全流程执行。

### 10.5 同应用同环境 CD 时间互斥

创建或编辑预约单时，如果满足以下条件，直接拦截：

```text
application_id 相同
env_code 相同
cd_conflict_at 相同
status in pending_approval / approving / scheduled / dispatching
不是当前预约单自身
```

参与 CD 互斥的模式：

```text
deploy
build_deploy
execute
```

不参与 CD 互斥的模式：

```text
build
```

错误文案：

```text
同一应用同一环境在该 CD 时间已存在预约发布，请调整时间后再提交
```

### 10.6 跨发布单并发锁

预约创建时只处理明确的 CD 时间互斥。其他跨发布单并发冲突继续交给现有发布并发锁处理。

到点触发时：

- 并发策略为 `reject` 且预检不通过：预约单进入 `blocked`。
- 并发策略为 `queue` 且执行入口接受排队：预约单进入 `dispatched`，发布单进入现有排队状态。

## 11. 到点调度

新增后台任务 `ReleaseScheduleTask`。

建议扫描周期：10 到 30 秒。

### 11.1 扫描范围

扫描两类数据：

1. 未审批且已到最早预约时间的预约单：

```text
status in pending_approval / approving
earliest_scheduled_at <= now
```

处理为 `expired`。

2. 已审批且到点的预约单：

```text
status = scheduled
next_due_stage_at <= now
```

原子抢占为 `dispatching` 后触发。

### 11.2 分阶段触发

`build`：

- 到 `build_scheduled_at` 后触发 `Build`。
- 成功后预约单进入 `dispatched`。

`deploy`：

- 到 `deploy_scheduled_at` 后触发 `Deploy`。
- 成功后预约单进入 `dispatched`。

`build_deploy`：

- 到 `build_scheduled_at` 后触发 `Build`，记录 `build_dispatched_at`，预约单仍保持 `scheduled`。
- 到 `deploy_scheduled_at` 后触发 `Deploy`，记录 `deploy_dispatched_at`，预约单进入 `dispatched`。

`execute`：

- 到 `execute_scheduled_at` 后触发 `Execute`。
- 成功后预约单进入 `dispatched`。

### 11.3 到点前最终预检

调度器触发前必须重新调用现有预检：

| 动作 | 预检 | 执行 |
| --- | --- | --- |
| CI | `PrecheckBuild` | `Build` |
| CD | `PrecheckDeploy` | `Deploy` |
| 全流程 | `PrecheckExecute` | `Execute` |

如果预检不通过：

```text
status = blocked
last_error = 预检阻断原因
```

如果目标阶段已经人工完成：

```text
status = skipped
last_error = 目标阶段已完成，预约触发已跳过
```

如果系统异常：

```text
status = failed
last_error = 异常信息
```

## 12. 人工操作联动

人工执行发布单时，预约单需要同步处理，避免重复触发：

| 人工动作 | 预约单处理 |
| --- | --- |
| 人工 `Execute` | 当前 active 预约单置为 `skipped`，原因记录“发布单已人工执行” |
| 人工 `Build` | 如果预约单只含 CI，置为 `skipped`；如果是 `build_deploy`，记录 CI 已完成，保留 CD 预约 |
| 人工 `Deploy` | 当前包含 CD 的 active 预约单置为 `skipped` |
| 取消发布单 | 当前 active 预约单置为 `cancelled` |
| 删除发布单 | 存在 active 预约单时拒绝删除 |

一期使用保守策略：

- 人工 `Build` 对 `build_deploy` 只跳过 CI 阶段，保留 CD 阶段。
- 人工 `Execute` 和人工 `Deploy` 直接终结预约单。

## 13. 后端接口

### 13.1 创建预约单

```http
POST /release-orders/:id/schedule
```

请求：

```json
{
  "schedule_mode": "build_deploy",
  "build_scheduled_at": "2026-05-02T20:00:00+08:00",
  "deploy_scheduled_at": "2026-05-02T21:00:00+08:00",
  "timezone": "Asia/Shanghai",
  "remark": "生产窗口预约发布"
}
```

### 13.2 更新预约单

```http
PUT /release-order-schedules/:id
```

仅允许 `pending_approval` 和 `approving` 状态。

### 13.3 取消预约单

```http
POST /release-order-schedules/:id/cancel
```

### 13.4 查询发布单预约

```http
GET /release-orders/:id/schedule
```

### 13.5 预约单列表

```http
GET /release-order-schedules
```

过滤条件：

- `keyword`
- `application_id`
- `env_code`
- `schedule_mode`
- `status`
- `approval_approver_user_id`
- `creator_user_id`
- `scheduled_at_from`
- `scheduled_at_to`
- `page`
- `page_size`

### 13.6 预约审批

```http
POST /release-order-schedules/:id/submit-approval
POST /release-order-schedules/:id/approve
POST /release-order-schedules/:id/reject
GET  /release-order-schedules/:id/approval-records
```

审批行为和权限参考现有发布单审批接口。

## 14. 前端设计

### 14.1 预约发布列表页

新增菜单：

```text
发布管理 -> 预约发布
```

列表字段：

- 预约单号
- 发布单号
- 应用
- 环境
- 预约模式
- CI 时间
- CD 时间
- 全流程时间
- CD 风险时间
- 审批状态
- 预约状态
- 创建人
- 审批人
- 最近错误
- 创建时间
- 操作

操作：

- 查看发布单
- 编辑预约
- 提交审批
- 审批通过
- 拒绝
- 取消预约

### 14.2 发布单详情页

在发布单详情页增加预约卡片：

- 当前是否存在 active 预约单
- 预约模式
- 预约时间
- 审批状态
- 预约状态
- 最近错误
- 操作入口

如果不存在 active 预约单，且发布单可预约，展示“创建预约”按钮。

### 14.3 创建和编辑弹窗

字段：

- 预约模式：CI / CD / CI+CD / 全流程
- CI 预约时间
- CD 预约时间
- 全流程开始时间
- 时区
- 备注

前端校验：

- 时间必填规则。
- 时间必须晚于当前时间。
- CI+CD 模式下 CD 时间必须晚于 CI 时间。
- 审批通过后不展示编辑入口。

后端仍必须做所有强校验。

## 15. 权限

新增权限：

- `release.schedule.create`
- `release.schedule.view`
- `release.schedule.cancel`
- `release.schedule.approve`

权限作用域沿用发布单所属应用和环境。

一期兼容策略：

- `release.create` 覆盖 `release.schedule.create`。
- 预约审批必须校验用户是否在预约单快照的审批人列表中。

## 16. 错误处理

业务阻断返回 409：

- 发布单已有 active 预约单。
- 同应用同环境同 CD 时间已有 active 预约。
- 预约单审批通过后不可编辑。
- 预约时间不合法。
- 阶段时间顺序不合法。
- 发布单状态不允许预约。

调度执行中的业务阻断写入预约单状态：

- 预检不通过：`blocked`
- 未审批到点：`expired`
- 阶段已人工完成：`skipped`
- 执行入口异常：`failed`

## 17. 测试点

后端单元测试：

- 创建 `build` 预约成功。
- 创建 `deploy` 预约成功。
- 创建 `build_deploy` 时 CD 早于 CI 被拒绝。
- 创建 `execute` 时写入 `cd_conflict_at = execute_scheduled_at`。
- 同发布单存在 active 预约时再次创建被拒绝。
- 同应用同环境同 CD 风险时间预约被拒绝。
- 审批通过后编辑被拒绝。
- 未审批到点后调度器置为 `expired`。
- `scheduled` 到点触发 `Build`。
- `scheduled` 到点触发 `Deploy`。
- `build_deploy` 先触发 CI 后保留 CD。
- 目标阶段已人工完成时预约置为 `skipped`。

集成测试：

- 预约审批流复用模板审批人快照。
- 到点触发前重新执行现有 `PrecheckBuild`、`PrecheckDeploy`、`PrecheckExecute`。
- 并发锁 reject 场景进入 `blocked`。
- 并发锁 queue 场景预约置为 `dispatched`，发布单进入现有排队状态。

前端测试：

- 预约发布列表筛选和状态展示。
- 发布单详情预约卡片展示。
- 创建弹窗按模式显示不同时间字段。
- 审批通过后不显示编辑入口。
- CD 时间冲突错误展示。

## 18. 实施顺序建议

1. 新增领域模型、Repository 接口和数据库表。
2. 实现预约创建、编辑、取消、查询和冲突校验。
3. 实现预约审批，复用发布模板审批人快照。
4. 实现调度器和到点触发逻辑。
5. 接入人工执行联动。
6. 新增 HTTP 接口和 Swagger。
7. 新增前端 API、类型、预约列表页和发布单详情卡片。
8. 补齐后端单元测试和关键前端测试。

## 19. 默认配置

- 预约时间最小提前量默认为 2 分钟。
- CI 与 CD 最小间隔默认为 5 分钟。
- 调度器扫描周期默认为 10 秒。
- 发布单删除时，若存在 active 预约单则拒绝删除。
