---
goal: 提供长期可复用的 GOS 应用接入向导，串联首次初始化、后续新增应用及已有应用配置接续
version: 1.2
date_created: 2026-09-05
last_updated: 2026-09-05
owner: GOS 项目维护者
status: In progress
tags: [feature, onboarding, configuration, pipeline, release]
---

# Introduction

![Status: In progress](https://img.shields.io/badge/status-In%20progress-yellow)

本方案提供长期保留的“应用接入向导”，同时覆盖 GOS 新安装后的首次业务配置、平台使用期间持续新增应用和已有应用配置接续。首次使用时可以没有预建项目、应用、自定义标准字段、应用管线绑定或发布模板，但执行端已有需要接入的管线；后续接入时复用已存在且校验有效的公共配置。目标是让用户沿一个连续任务完成每个应用的配置，无需提前理解内部模块依赖，也无需提前规划完整标准字库。

范围是“接入已有管线并完成首单发布”，不是从代码仓库生成管线，不要求填写应用访问地址，也不新增用户必须先维护的“发布方案”对象。仓库地址仅在用户选择了依赖该数据的参数来源时才成为条件必填项。

首版核心接入流程已在本地实现，并通过隔离空业务库的浏览器验收：首个应用就地补字段、绑定与模板、保存首单不执行，以及第二个应用复用公共配置但创建独立绑定/模板。代码基线 HEAD 为 `d87e1d49`。工程根目录为 `/Users/lingyunxieqing/Desktop/gos`，下文工程路径均相对该根目录。本次未提交、推送或部署。

首版实现合并了若干原定组件/校验文件：领域与仓储接口位于 `internal/domain/onboarding/entity.go`；配置检查、局部参数同步和编排位于 `onboarding.go`，调用已有发布参数校验；前端步骤暂保留在 `ApplicationOnboardingView.vue`，没有改造原高级模板编辑器。使用说明及实测限制见 `docs/first-release-onboarding.md`。下方任务按功能及真实覆盖范围标记，未完成项不计为已交付。

## 1. Requirements & Constraints

### 1.1 已确认的代码事实

| 位置 | 已有行为 | 对方案的约束 |
|------|----------|--------------|
| `cmd/server/main.go:69`、`:166` | 启动时初始化表结构和管理员 | 不另建一套账户、表结构初始化机制 |
| `internal/infrastructure/persistence/sqlrepo/platform_param_repository.go:90` | 初始化内置标准字段 | 优先复用内置字段，不能把“维护全部标准字段”作为引导第一步 |
| `internal/bootstrap/config.go:223` | 默认发布环境为 dev/test/prod | 展示并确认已有环境，不自动更改已有全局环境 |
| `cmd/server/main.go:307` | 启用自动同步时，同步管线和执行器参数 | 展示同步结果与失败原因，避免重复要求手工同步 |
| `frontend/src/router/index.ts:295` | `/system/quick-start` 仅重定向到系统设置 | 可将该入口替换为真实引导中心 |
| `internal/application/usecase/create.go:41`、`update.go:52` | 应用创建和更新强制要求语言、制品类型 | 精简应用基本信息需要同步调整后端校验，不能只隐藏前端字段 |
| `frontend/src/views/application/ApplicationCreateView.vue:84` | 保存应用后返回列表 | 缺少进入绑定、参数、模板配置的连续交互 |
| `internal/application/usecase/pipeline_binding.go:375` | 同一应用同一 CI/CD 类型不能重复绑定 | 引导必须识别并复用已有绑定，不能重复创建 |
| `internal/domain/executorparam/entity.go` | 参数定义以管线为归属，标准字段映射不是应用私有数据 | 引导不能覆盖共享映射而影响其他应用 |
| `internal/application/usecase/executor_param_def.go:229` | 映射要求字段已存在、已启用且不是 CD 自填字段 | 需要先就地补建缺少的字段，再提交映射 |
| `internal/application/usecase/release_template.go:190` | 模板绑定应用、管线绑定和参数定义；审批已移至应用级 | 审批设置必须操作应用级流程，不能写回模板审批字段 |
| `frontend/src/views/release/ReleaseOrderCreateView.vue:448` | 已支持通过查询参数带入应用和模板 | 首单创建可以复用现有页面 |
| `internal/application/usecase/release_order_precheck.go:68` | 已有发布、构建、部署前检查 | 配置检查应抽取共用规则，实际执行仍走原预检与权限链路 |

### 1.2 产品目标与边界

- **REQ-001**: 新用户完成一次“接入应用”任务即可具备创建首个发布单的条件；不要求离开引导去标准字库、绑定和模板列表分别创建对象。
- **REQ-002**: 接入顺序固定为“基础检查 → 项目与应用 → 选择管线 → 参数与标准字段 → 模板及应用发布流程 → 完成检查 → 首单发布”。用户可以返回已完成步骤；返回后的修改使相关下游检查失效。
- **REQ-003**: 标准字段按实际管线参数按需处理。已存在的有效映射直接复用；明确匹配的未映射字段给出建议；未识别参数可在当前步骤补建字段。所有待写入的共享变更在保存前展示。
- **REQ-004**: 创建项目和应用时，项目/应用名称、Key、应用负责人为必要业务信息。语言、制品类型改为可选分类信息，空值存为空字符串；不伪造语言或制品类型。旧记录已有值保持不变。
- **REQ-005**: 仓库地址、发布分支、制品库、GitOps 配置均按实际选中的参数来源或执行方式决定是否需要；纯已有管线绑定流程不要求这些信息。
- **REQ-006**: 每步显示“本步用途、需要填写什么、将生成什么、完成后下一步是什么”。字段说明必须使用用户可理解的业务描述，并保留真实管线参数名便于核对。
- **REQ-007**: 引导支持保存草稿、刷新恢复、重新登录恢复、指定已有应用继续配置和失败步骤重试。已合法创建的资源不因取消引导而自动删除。
- **REQ-008**: 区分“配置完成”和“首次执行验证成功”。创建模板或创建发布单都不能被标为发布成功；首单结果来自实际发布单和执行记录。
- **REQ-009**: 首页、应用空状态、应用列表和详情提供一致的接入入口与配置状态。已有可用应用不因缺少引导会话而被标记为未配置。
- **REQ-010**: 首版完整覆盖已有 Jenkins 管线的 CI-only、CI+CD，以及具备独立输入条件的 CD-only 接入；CD-only 需要上游构建结果而没有有效来源时必须阻止完成。已有 ArgoCD/GitOps 高级配置继续可用，检测到该模式时链接原编辑器并保留会话，不伪装成已完成 Jenkins 引导。
- **REQ-011**: 首单创建默认只保存，不触发执行。发布、构建、部署、审批继续使用原有业务接口、状态机、权限和并发规则。
- **REQ-012**: 通知、Hook、制品配置等非必需项可延后，不能成为纯管线接入的全局必填项。既有应用的审批和高级配置必须保留，不能被默认值清空。
- **REQ-013**: 应用接入入口长期存在，不因已有应用、已有成功发布单、某个会话 configured/verified 或平台初始化完成而隐藏。具备权限的用户可随时从应用列表和项目详情发起新应用接入。
- **REQ-014**: 后续新应用复用用户选定的现有项目、可用管线、标准字段和有效管线参数映射，但创建归属新应用的独立绑定与模板。不得复用旧应用绑定/模板 ID，不复制构建号、环境固定值、审批人、权限或敏感配置；同项目只是项目归属一致，不代表所有应用差异相同。
- **REQ-015**: 每个新应用建立独立会话，进度和首单验证按应用/会话隔离。完成页提供“继续接入下一个应用”，仅可预选当前项目；应用信息、绑定、模板、首单记录和应用专属参数重新配置。允许改选其他项目或在同一引导中新建项目。
- **SEC-001**: 新接口按实际步骤复用现有 `application.manage`、`pipeline.view`、`pipeline.manage`、`platform_param.manage`、`pipeline_param.manage`、`release.template.manage` 以及应用/环境发布权限；不因拥有引导会话而自动获得管理权限。
- **SEC-002**: 会话仅创建者及具有相应管理权限的管理员可访问。新建字段、修改映射、创建模板分别校验对应权限；无权限时显示需要管理员处理的具体项，不自动提权、不复制其他应用的授权。
- **SEC-003**: 凭据不写入普通草稿字段、URL、日志或明文导出。已知敏感类型参数的固定值不进入首版引导；要求沿用执行端已有凭据机制或由管理员走受控高级配置。不能仅凭名称推断敏感参数已安全。
- **CON-001**: 不创建或修改远端管线脚本，不调用已有的 raw pipeline 写接口，不触发测试构建来探测参数，不自动选择或发布到生产环境。
- **CON-002**: GOS 数据库、管理员和执行端连接仍是安装期基础配置。引导检查连接与权限但不新增 Jenkins 连接热更新模块；若启用了启动强检查而连接失败，需先修正安装配置，网页引导无法修复一个尚未启动的服务。
- **CON-003**: 本方案不重构 CI/CD 状态机、不引入跨应用发布方案版本继承、不自动迁移现有模板。不改动当前工作区已有的配置、数据库和 Nginx 相关修改。
- **PAT-001**: UI 和 API 共用同一个接入编排用例；复用原领域校验。新增检查结果使用结构化错误定位，不在前端重复实现业务校验。

### 1.3 用户流程与页面契约

入口使用 `/system/quick-start`，页面标题为“应用接入向导”。新建任务后进入 `/onboarding/:sessionId`。应用列表长期保留“新增应用（引导）”；项目管理入口提供“新增应用”并预选该项目。登录后对无可用应用的管理员额外展示首次引导提示卡，不强制劫持所有用户导航。首次完成后只收起首次提示卡，不隐藏新增应用入口。

| 使用场景 | 会话与数据处理 | 流程表现 |
|----------|----------------|----------|
| 平台首次使用 | 创建新应用会话，允许项目、字段、绑定和模板都从零开始 | 展开基础检查和术语说明，完整带领用户完成配置 |
| 后续新增应用 | 每次创建独立新应用会话；可选择已有项目或原地新建项目 | 实时检查已配置公共能力，已通过项显示摘要，无需重复填写；选中管线后复用有效标准映射，仅处理新增参数与应用差异 |
| 已有应用继续配置 | 显式指定已有 application_id，恢复其可访问的未完成会话或新建补齐会话 | 读取该应用真实配置，定位缺失步骤；不能把“新增应用”误判为继续修改上一个应用 |

首次提示是界面呈现方式，不是只能执行一次的系统模式。所有场景共用同一套步骤与后端编排；已配置项不重复录入，但连通性、权限和管线元数据仍按本次接入重新检查。

| 步骤代码 | 页面内容 | 本步保存行为 | 缺项处理 |
|----------|----------|--------------|----------|
| `preflight` | 执行端是否启用、连通性、权限、管线和参数同步状态、现有发布环境 | 保存检查摘要；用户明确点击同步时才同步元数据 | 无连接时显示安装配置项和重试按钮；空管线与读取失败分开显示；不把读取失败视为零参数 |
| `identity` | 选择/新建项目；选择/新建应用；名称、Key、负责人；可选分类折叠 | 保存项目、应用引用；新建操作生成实际对象 | Key 冲突时选择已有对象或修改 Key，不按同名静默合并；仓库地址默认不显示 |
| `pipelines` | CI-only、CI+CD、CD-only；选择实际管线，展示完整任务路径、说明、状态 | 创建或复用应用绑定；读取选中管线参数 | 既有同类型绑定不同则要求明确变更；先列出关联模板影响，存在影响时转高级配置，不自动替换 |
| `parameters` | CI/CD 参数表、自动匹配结果、未识别参数、值来源、发布表单预览 | 逐项保存明确确认的字段和映射；保存应用模板参数草稿 | 新字段原地创建；类型不兼容、名称歧义、必填参数未覆盖时定位到具体行 |
| `template_flow` | 默认模板名称、阶段摘要；应用审批流程保持原值或显式选择；高级设置入口 | 保存模板与应用流程配置 | 新应用默认呈现当前分段发布方式，并在摘要中说明；不新增隐藏的自动部署开关，不绕过审批 |
| `review` | 已创建/复用对象、参数来源、发布时需填写项、检查结果 | 所有配置检查通过后会话置为 configured | 出错项有“返回修复”按钮；网络失败为 unknown/retry，不显示通过 |
| `first_release` | 带入应用与模板的发布单表单；执行入口和结果跟踪 | 单独明确创建首单，随后由用户明确点击执行 | 执行失败保留配置完成状态；根据失败原因修复配置或重放，不能把所有执行失败都归为配置失败 |

页面固定显示步骤条、当前保存状态、上一步、保存并继续、稍后继续。草稿编辑停止 800ms 后保存非敏感数据；创建实际资源只在“保存并继续”时发生。离开未成功保存的页面需要提示。移动端将参数表转为逐参数卡片；不依赖宽表水平滚动才能看到错误或下一步按钮。

### 1.4 参数步骤：把标准字库前置要求消除掉

每行保留以下信息：阶段、真实参数名、说明、类型、是否必填、执行端默认值/候选项、标准字段、值来源、检查状态。默认只展开待确认行；已复用的标准映射可展开查看，不重复要求逐项确认。

字段匹配顺序固定：

1. 已有非空、启用且合法的映射：复用，不因名称相似自动替换。
2. 已有无效映射：标为阻塞；展示失效原因及共享影响，不自动清空重建。
3. 未映射参数：先匹配平台已支持的确定性运行时规则（`CI_JOB`、`CI_BUILD`），再匹配规范化后的字段 Key 和名称。只有唯一、类型兼容、非 CD 自填字段可作为自动建议。
4. 有多个候选或仅语义相近：必须由用户选择，不使用模糊匹配直接写入。
5. 没有匹配：提供“新建标准字段”内联表单，填写显示名称、建议 Key、类型。Key 建议为管线参数名转小写、非字母数字下划线替换为下划线、首字符非字母时加 `p_`；长度超过 90 时使用前 81 字符加下划线和原名 SHA-256 前 8 位。Key 冲突时展示现有字段，不静默覆盖或自动合并。创建仍调用标准字段领域校验。

类型规则：bool 仅匹配 bool；number 仅匹配 number；string 匹配 string；choice 可以映射到 string 或 choice，候选值以具体执行端实时定义为准，不把某一条管线的候选集写成全局字典选项。多选、动态候选、密码或不支持的插件类型明确标记“需高级配置”，不假装是普通字符串。

值来源不是标准字段的同义词，界面必须分开说明：

| 用户文案 | 底层来源 | 完成检查要求 |
|----------|----------|--------------|
| 发布时填写 | `release_input` | 配置阶段不要求实际发布值；预览对应输入框，真实值在发布单创建/执行时校验 |
| 每次使用固定值 | `fixed` | 保存前填写并验证类型、候选值；不得覆盖已有模板固定值 |
| 使用发布基础信息 | `builtin` | 必须明确具体来源；如果来源为应用仓库或发布分支而信息缺失，在此处补齐，不向所有应用加必填项 |
| 沿用 CI 参数 | `ci_param` | 仅 CD 可选；引用已配置 CI 标准字段，来源存在且类型兼容 |
| 使用本次 CI 构建结果 | 已有 CI 运行时字段处理 | `CI_JOB`/`CI_BUILD` 由实际成功的上游 CI 提供；配置时显示“运行时生成”，不要求现在填写构建号 |

固定值与运行时生成项在发布表单中只读展示来源，不再次要求用户填写。`DEPLOY_ENV` 不因名字相似就强制映射到平台环境；先确认其含义和候选值。`PROJECT_NAME` 不直接假定等于 GOS 项目显示名称：可明确选固定值或发布时填写，CI/CD 保持一致来源。

全部实时必填参数必须被覆盖，包括尚未加入模板的参数。可选参数只有在实时定义确认为可省略时才可选择“使用管线默认值/不传递”，摘要记录选择；不能用隐藏参数来绕过必填校验。同一阶段多个真实参数映射相同标准字段时，首版阻止完成并要求消除歧义，避免发布表单值覆盖。

现有管线映射是共享数据。引导允许新增标准字段和补齐未映射项，但已有非空映射不允许通过快捷操作改成另一字段；需要进入原管理页确认影响后处理。所有共享写入在保存前重查当前版本和引用，发现变化返回冲突并重新预览。

### 1.5 配置完成与首次发布闭环

配置检查返回 `ready/blocked/unknown`，不能用“存在模板”代替配置正确。检查覆盖应用状态、负责人、绑定归属/阶段/状态、管线可读取性、参数真实存在性、类型与候选项、必填覆盖、标准字段状态、值来源有效性、启用的管线规范和应用审批流程有效性。

检查区分三个时间点：配置时检查来源关系；创建发布单时检查用户填写的实际值；执行前检查最新管线定义、权限、审批和并发锁。后一个检查不能因为前一个已通过而省略。CI 运行时构建号在配置阶段缺失不是错误；需要上游的 CD-only 没有可解析来源则是错误。

配置成功页有三个入口：“创建该应用的第一个发布单”“继续接入下一个应用”和“返回应用”。继续接入会创建独立会话，按用户当前项目预选项目引用但不复制应用专属配置，也不要求先完成当前应用的首单执行。首单页面保留现有 `application_id`、`template_id` 查询参数，额外携带 `onboarding_session_id`；该 ID 只关联配置进度，不赋予权限，也不改变执行方式。这里的“首单”始终是该应用的首单，而非全平台只有一次首单机会。

发布列表与详情对同一发布单提供一致动作。已构建待部署且有执行权限时提供部署入口；没有权限、审批未完成或其他阻塞时展示具体原因。构建中、等待手动部署、等待审批和执行失败不能使用同一种“执行中”提示。

用户完成 CI-only 首单构建，或完成所配置 CI+CD/CD-only 的目标执行后，才将首单验证标为成功。实际执行失败不自动重试，不修改原执行状态；按原发布单重放/重试功能处理。若首单失败后用户明确选择一个重放单作为验证单，记录其 ID 并核验应用及来源关系。

### 1.6 状态、数据与 API 契约

新增 `onboarding_sessions`，物理列为 `id`、`owner_user_id`、`version`、`payload`、`lock_key`、`locked_until`、`updated_at`。类型化 payload JSON 保存 `mode/status/current_step/draft/refs/check/first_release_order_id/created_at/updated_at` 等会话字段；版本与所有者同步存储用于 SQL 条件更新。mode 为 `create_application/complete_application`：前者不接受现有 application_id，应用创建后由服务端关联 ID；后者要求明确的已有 application_id 并校验访问权限。项目预选写入 draft，校验项目存在。状态枚举为 `draft/in_progress/blocked/configured/verified/abandoned`。阻塞不删除已有进度；configured 表示配置检查通过；verified 表示目标首单实际成功。会话版本使用乐观锁，不设置禁止再次接入的全局完成标志。

新增 `onboarding_operations`，字段为 `session_id`、`step_code`、`request_key`、`request_hash`、`status`。主键为 `(session_id, request_key)`；状态为 `prepared/applied/failed`。资源 ID 由会话与资源种类确定并在会话 refs 中记录；错误写入会话 check，不在回执中重复存储。

采用逐步骤提交，不承诺多个现有管理用例之间的全局事务：

1. 保存操作意图、输入摘要和预分配资源 ID，再调用原用例的内部指定 ID 创建入口。
2. 用例创建成功后记录 applied；若进程在创建后、记录前退出，重试按已保存资源 ID 查证并比较内容，成功则补记回执，不重复创建。
3. 公共 HTTP 创建接口仍由服务端生成 ID，不接受任意客户端资源 ID。内部 `CreateWithID`/`ExecuteWithID` 入口仅供编排用例使用，并复用原验证逻辑。
4. 新建项目/应用遇到其他资源占用 Key 时返回 409，不自动认领。新建标准字段遇到并发同 Key 创建时，重读并展示可复用结果，由新预览确认。
5. 共享资源更新要求预期版本匹配；同会话操作以数据库条件更新串行认领，不仅依赖前端禁用按钮或进程内锁。
6. 取消会话仅标为 abandoned，并显示已创建资源清单。不开启自动清理，不删除共享字段、管线绑定、审批流程或已有应用。

| 新接口，均为待实现 | 用途与副作用边界 |
|------------------|------------------|
| `GET /onboarding/status` | 返回当前账号可见的接入状态、能力和入口建议；只读 |
| `POST /onboarding/sessions` | mode=create_application 始终为新应用建立独立会话，可预选 project_id；mode=complete_application 要求 application_id，仅恢复该应用可访问的未完成会话；写入草稿，不创建业务资源，不依据项目相同或上一次会话完成情况自动复用应用 |
| `GET /onboarding/sessions/:id` | 返回草稿、资源引用、版本、步骤状态和错误定位 |
| `PUT /onboarding/sessions/:id/draft` | 带 `expected_version` 保存非敏感草稿；冲突返回 409 |
| `POST /onboarding/sessions/:id/inspect` | 实时读取选中管线定义并生成建议；不创建标准字段、不修改映射、不触发管线 |
| `POST /onboarding/sessions/:id/steps/:step/apply` | 带 `request_key`、`expected_version`、确认后的操作列表，执行本步明确写入；相同 key 不同内容返回 409 |
| `POST /onboarding/sessions/:id/check` | 重新计算配置完成情况，不创建发布单、不执行管线 |
| `POST /onboarding/sessions/:id/first-release` | 使用原发布单 Create 逻辑幂等创建首单并关联；无执行副作用；相同请求返回同一单 |
| `POST /onboarding/sessions/:id/abandon` | 放弃会话，返回保留的资源摘要，不删除资源 |
| `GET /applications/:id/setup-status` | 聚合已有真实配置和会话状态，返回 ready/blocked/unknown 及具体继续入口 |

统一问题结构为 `{code, severity, step, field_path, message, remedy}`；相关资源 ID 由会话 refs 或外层模板检查项提供。severity 为 `info/warning/blocking`。网络未知状态用独立 code 返回；HTTP 权限错误不转成“未配置”。inspect/check 的元数据读取可以更新会话检查摘要，但不得改变业务资源。

## 2. Implementation Steps

### Implementation Phase 1 — 共用检查与最小模型调整

- **GOAL-001**: 明确配置就绪条件并解除纯管线应用的无关分类必填项。完成标准：共用校验测试通过；旧应用字段和执行行为不变。无前置阶段依赖。

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-001 | `onboarding.go:CheckSession` 与 `SetupStatus` 提供结构化配置检查，复用 `validateSingleTemplateParamMapping`，不调用 Execute；实际执行预检保持不变。未为此复制或拆分原预检文件。 | 是 | 2026-09-05 |
| TASK-002 | `create.go`、`update.go` 和 `ApplicationForm.vue` 的语言、制品类型改为可选；向导使用精简表单，应用来源更新保留原分类/制品/GitOps 字段；旧记录导入与放弃保留资源的回归已覆盖。 | 是 | 2026-09-05 |
| TASK-003 | `onboarding_param_resolver.go:ResolveOnboardingParamSuggestions` 提供只读建议、歧义、类型及敏感字段校验；`onboarding_param_mapping.go:MapUnmappedParameter` 使用未映射/启用状态 CAS，不覆盖共享映射。 | 是 | 2026-09-05 |
| TASK-004 | `onboarding.go` 通过 `GetJobParamSet` 读取所选管线；inspect 不写业务资源，明确提交绑定步骤后仅同步所选管线参数，不停用范围外定义。 | 是 | 2026-09-05 |

### Implementation Phase 2 — 可恢复的后端编排

- **GOAL-002**: 提供经过权限控制、幂等和崩溃恢复的接入 API。完成标准：用 API 从空业务数据创建应用、绑定、字段和模板；任一步失败可恢复且不重复创建。依赖 GOAL-001。

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-005 | `entity.go` 合并领域与仓储契约；`onboarding_repository.go` 新增版本迁移、类型化 JSON 会话、SQL 所有者/版本/租约和幂等回执。包含 SQLite/MySQL DDL 分支，SQLite 已测试；MySQL 实例验收见 TASK-017。 | 是 | 2026-09-05 |
| TASK-006 | `onboarding_creation.go` 的内部恢复 ID 上下文接入六个原创建用例；公共 API 不接受 ID，正常创建仍随机。已测试应用已创建但回执丢失的恢复。 | 是 | 2026-09-05 |
| TASK-007 | `onboarding.go` 实现两种 mode、草稿、检查、步骤编排、首单、放弃，覆盖隔离、保护、幂等、部分失败恢复及审批重试；首单重放重新关联见 TASK-016。 | 是 | 2026-09-05 |
| TASK-008 | `onboarding_handler.go` 实现步骤权限/所有权控制，`ApplicationHandler.RegisterRoutes` 注册新接口，`cmd/server/main.go` 注入仓储与用例；不改变原 Router 公共签名。 | 是 | 2026-09-05 |

### Implementation Phase 3 — 连续引导页面

- **GOAL-003**: 用户在一个任务中完成所有必要配置，标准字段可原地补齐。完成标准：不打开旧模块列表也能完成标准 CI+CD 接入；刷新后恢复到相同步骤和内容。依赖 GOAL-002。

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-009 | 新增类型/API/中心/七步页面和永久路由；实现 800ms 串行草稿保存、版本冲突和路由离开/同路由任务切换保护。已修复自动保存时禁用输入导致下拉选择被中断的问题。 | 是 | 2026-09-05 |
| TASK-010 | 项目/应用与管线步骤已在 `ApplicationOnboardingView.vue` 实现；实际任务全名、绑定保护与条件来源齐备，无应用访问地址输入。组件拆分不作为首版必需。 | 是 | 2026-09-05 |
| TASK-011 | 同页实现字段内联创建、已有映射折叠、应用专属来源和发布表单摘要、条件字段；运行时参数锁定正确来源，窄屏使用参数卡片。 | 是 | 2026-09-05 |
| TASK-012 | 同页实现新模板及应用审批显式设置和检查；只有一个旧模板时引用且锁定，多模板检查/高级编辑保留，不覆盖原模板。原高级编辑器组件未抽取。 | 是 | 2026-09-05 |

### Implementation Phase 4 — 首单闭环和日常接续

- **GOAL-004**: 配置结果能直达首单，列表能解释下一步，已有应用能按真实配置继续。完成标准：配置完成与执行成功分开；手动部署入口及阻塞原因在列表和详情一致。依赖 GOAL-003。

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-013 | `ReleaseOrderCreateView.vue` 会话模式锁定应用和模板、禁用批量/快捷执行分支，使用首单幂等 API；创建只保存，刷新后返回同一发布单。 | 是 | 2026-09-05 |
| TASK-014 | 永久应用/项目入口、详情接入入口、仅预选项目的下一应用、既有应用逐模板检查均已实现。未将实时远端检查批量接入应用工作台，避免每次列表加载产生远端扇出；后续在 `application_workbench.go` 实现有缓存/刷新策略的聚合展示。 | 部分 | 2026-09-05 |
| TASK-015 | 从 `ReleaseOrderListView.vue`、`ReleaseOrderDetailView.vue` 的执行动作判断抽出共用的展示层解析函数到 `frontend/src/utils/release-dispatch-actions.ts`，使列表与详情对同一状态/权限有相同动作；后端 precheck 仍是执行权威。等待部署、审批、权限不足分别解释，保留原重放规则，不新增自动执行分支。 | 否 | 未开始 |
| TASK-016 | 已实现按实际成功状态刷新 verified，以及无会话旧应用按真实模板检查。显式关联失败首单派生的重放单未实现，继续使用原重放页面但不替换向导验证单；后续需补来源链/应用/模板校验和授权 API。 | 部分 | 2026-09-05 |

### Implementation Phase 5 — 验证、文档和交付

- **GOAL-005**: 在隔离环境完成真实空库流程和失败恢复验证，交付可复核证据。完成标准：第 6 节测试全部通过，明确记录测试范围与未覆盖项。依赖 GOAL-004。

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-017 | 新增用例/仓储/HTTP 测试、可重复 SQLite 迁移、故障注入、权限和本地只读执行端夹具；`go test ./...` 通过。尚未连接隔离 MySQL 实例验证，不宣称第 6 节所有故障矩阵全覆盖。 | 部分 | 2026-09-05 |
| TASK-018 | 新增 4 项前端契约测试通过，正式构建通过；全量静态测试 241 项中 231 通过/10 个原有失败，类型检查 114 条基线错误且无新增。浏览器走通首个/第二个应用和首单只保存、桌面/390px 布局；真实缩放 80%/125%、执行部署和全部异常 UI 路径未验收。 | 部分 | 2026-09-05 |
| TASK-019 | 更新 README、新增使用/API/恢复/验证说明，添加 10 个 HTTP 路由的 Swagger 注解并生成 docs.go、swagger.json、swagger.yaml；生成同时补齐此前已有代码未同步的接口文档，不改变其业务接口。 | 是 | 2026-09-05 |

## 3. Alternatives

- **ALT-001**: 只新增发布方案管理。未选用，因为空库用户仍需先配置方案，不能解决首次使用不知道如何开始的问题；未来复用需求可独立评估。
- **ALT-002**: 只增加静态教程或跨页面跳转清单。未选用，因为输入、状态和错误仍然分散；可作为补充帮助，不能代替可恢复引导。
- **ALT-003**: 先要求导入完整 YAML。未选用作为首版主入口，因为新用户仍需理解标准字段和对象关联；后续可在同一编排 API 上增加导入适配器。
- **ALT-004**: 取消标准字库或所有参数直接透传。未选用，因为会改变模板校验、发布表单、CI/CD 继承和现有执行链；本方案保留模型，仅将必要维护移入实际参数步骤。
- **ALT-005**: 在最后一步用一个超大事务创建全部资源。未选用，因为现有用例分散、模板仓储已有内部事务，外部元数据读取也不能放入数据库长事务；采用明确可见的逐步骤提交、幂等回执和恢复机制。

## 4. Dependencies

- **DEP-001**: GOS 服务能够启动并登录；数据库与管理员沿用现有部署配置，内置字段由既有启动初始化提供。没有部署可用的 GOS 服务时，本方案网页部分无法执行。
- **DEP-002**: 执行端已有用户希望绑定的管线，GOS 安装配置中的执行端地址和凭据有效；这里只依赖读参数、同步元数据和后续用户授权的执行能力，不要求维护新管线脚本。
- **DEP-003**: 复用现有 Vue、Ant Design Vue、Axios、Go usecase/repository、SQLite/MySQL 和发布单执行链；本方案不要求升级依赖包。
- **DEP-004**: 普通用户没有全局字段管理权限时，需要有权限的管理员继续相应步骤。向导负责暴露原因与接续入口，不通过后台代替普通用户越权写入。
- **DEP-005**: 阶段执行关系为 GOAL-001 → GOAL-002 → GOAL-003 → GOAL-004 → GOAL-005；同阶段没有显式依赖的任务可以独立开发，但不得并发编辑同一函数。

## 5. Files

- **FILE-001**: 新增 `internal/domain/onboarding/entity.go`、`repository.go`，定义独立接入任务状态，不将其混入发布单执行状态。
- **FILE-002**: 新增 `internal/infrastructure/persistence/sqlrepo/onboarding_repository.go`、`onboarding_repository_test.go`，提供迁移、会话和操作回执持久化。
- **FILE-003**: 新增 `internal/application/usecase/onboarding.go`、`onboarding_param_resolver.go`、`application_setup_check.go`、`release_configuration_validation.go` 及对应 `_test.go`。
- **FILE-004**: 调整 `internal/application/usecase/project_management.go`、`create.go`、`update.go`、`pipeline_binding.go`、`platform_param_dict.go`、`executor_param_def.go`、`executor_param_sync.go`、`release_template.go`、`release_order.go`、`release_order_precheck.go`、`application_workbench.go`，复用校验并提供内部恢复入口。
- **FILE-005**: 新增 `internal/interfaces/http/onboarding_handler.go`、`onboarding_handler_test.go`；调整 `router.go`、`application_handler.go`、`cmd/server/main.go` 及相关路由测试。
- **FILE-006**: 新增 `frontend/src/api/onboarding.ts`、`types/onboarding.ts`、`views/onboarding/` 页面与组件；调整 `frontend/src/router/index.ts`。
- **FILE-007**: 调整 `frontend/src/views/application/ApplicationForm.vue`、`ApplicationListView.vue`、`ApplicationDetailView.vue`、`ProjectManagementView.vue`，保留原高级管理入口，新增永久的新应用接入入口。
- **FILE-008**: 调整 `frontend/src/views/release/ReleaseTemplateView.vue`、`ReleaseOrderCreateView.vue`、`ReleaseOrderListView.vue`、`ReleaseOrderDetailView.vue`；新增 `frontend/src/utils/release-dispatch-actions.ts`。
- **FILE-009**: 新增 onboarding 前端测试和 `docs/first-release-onboarding.md`；更新 `README.md` 与生成的 HTTP 接口文档。实际拆分与未完成文件以上方任务完成记录为准。

## 6. Testing

- **TEST-001**: 空业务库保留默认管理员和内置字段，从 preflight 走到 configured；不预置项目、应用、自定义字典、绑定或模板。断言每种资源只创建预期数量。
- **TEST-002**: CI+CD 管线含全新必填参数；在 parameters 步骤内补建字典、映射并配置来源，完成后无需访问旧标准字库/执行器参数/模板列表。
- **TEST-003**: 已有映射复用、新字段同名冲突、类型不兼容、多候选、CD 自填字段不可映射、同阶段重复标准 Key、共享映射并发修改均返回预期定位，其他应用配置不变。
- **TEST-004**: 实时必填参数未选入模板仍被检查出来；false 和 0 被视为有效值；固定值不在发布页重复必填；候选值变更在执行前阻止错误参数。
- **TEST-005**: CI_JOB/CI_BUILD 在配置时不要求具体值，CI+CD 在运行时引用对应成功 CI；没有上游来源的依赖型 CD-only 无法标为 ready。重放仍满足原上游选择规则。
- **TEST-006**: 纯管线应用不填仓库地址、语言和制品类型可以完成配置；显式选择 repo_url 来源且无值时按条件阻塞。旧应用已有分类、制品、GitOps、审批和 Hook 数据不被清空。
- **TEST-007**: 每步保存后刷新和重新登录恢复；在业务资源已创建但回执未写入时注入崩溃，恢复后不重复创建。相同 request_key 同内容返回原结果，不同内容返回 409；会话并发编辑返回版本冲突。
- **TEST-008**: 字典已创建而模板失败时，显示已创建资源与可重试步骤；abandon 不自动删除资源；没有权限的用户不能通过 inspect/apply/first-release 读取或修改越权资源。
- **TEST-009**: 远端超时、401/403、管线停用、真正无参数与参数读取失败分别显示不同结果；check 为只读业务操作，不能触发构建或部署。
- **TEST-010**: 首单创建幂等，创建不执行；用户明确执行后状态来自真实执行记录。configured、首次运行失败、verified 分别显示，不将创建成功标为发布成功。
- **TEST-011**: 已构建待部署且有权限时列表与详情都有部署入口；审批未完成、无权限、并发阻塞均显示原因。CI-only、原有分段、重放和应用级审批回归通过。
- **TEST-012**: 已有应用无需引导会话也能判定配置状态；一个失败模板不屏蔽同应用其他有效模板；检查变更后重新计算，不靠全局 setup_complete 标志。
- **TEST-013**: 执行 `go test ./...`、`node --test tests/*.test.mjs frontend/tests/*.test.mjs`、在 frontend 目录执行 `./node_modules/.bin/vue-tsc --noEmit -p tsconfig.app.json`、`npm run build`。不能只检查含项目引用的根 tsconfig 而宣称应用已通过类型检查；实际结果见 TASK-017、TASK-018。
- **TEST-014**: 隔离运行环境实际浏览器验收：1440px 与 1024px 桌面宽度、390px 窄屏，80%/100%/125% 缩放；步骤条、参数行、固定操作栏、错误定位和确认摘要不重叠或遮挡。覆盖键盘前进、焦点定位和明确的保存反馈。
- **TEST-015**: SQLite 和 MySQL 新库/已有库迁移都通过；密钥、执行端 Token 和敏感参数不出现在草稿 JSON、接口错误、日志及文档中；发布测试只能使用隔离执行端，不操作生产环境。
- **TEST-016**: 第一个应用 configured 或 verified 后，应用列表、项目入口仍可创建第二个应用。第二个应用复用现有项目、同一组管线和标准映射，新增独立应用、绑定和模板；不重复创建全局标准字段、不更改第一个应用的配置和发布记录。
- **TEST-017**: 点击“继续接入下一个应用”只预选项目，新应用 Key、应用字段、模板参数值和首单记录不沿用旧值；可改选其他项目或新建项目。基础能力摘要通过实际检查生成，不因已有成功会话而跳过失效凭据、停用管线或字段变更检查。
- **TEST-018**: 同时打开两个新应用会话，草稿、保存结果、状态及首单验证互不覆盖；create_application 不接受旧 application_id，complete_application 只能接续明确指定且可访问的应用。首次提示卡隐藏不影响有权限用户访问永久新增入口。

## 7. Risks & Assumptions

- **RISK-001**: 管线参数映射是共享的，快捷改名或重映射可能影响多个应用；通过只复用既有映射、版本检查和显式高级处理控制影响。
- **RISK-002**: 向导若另写一份业务校验，会与实际发布预检出现分歧；共用静态规则，动态执行条件仍由原预检决定。
- **RISK-003**: 动态插件参数不一定能可靠枚举或推导含义；明确标为高级配置，不通过执行脚本探测，不声称所有管线都能零人工接入。
- **RISK-004**: 逐步骤提交会留下部分合法资源；用回执和进度解释清楚，不把取消向导做成危险的资源删除操作。
- **RISK-005**: 分类字段改为可选涉及旧表单、展示和测试；空字符串保持现有列结构，原值不迁移清空，并对所有使用点做回归。
- **RISK-006**: “配置检查通过”不能证明远端管线内部部署逻辑正确；只有用户明确执行并获得目标阶段成功结果才标为首单已验证。
- **ASSUMPTION-001**: 用户需要简化的是已有管线绑定式接入，不是创建远端执行平台、自动发现应用访问地址或生成仓库构建脚本。
- **ASSUMPTION-002**: 首次完整初始化由管理员完成；后续普通用户按现有权限继续或请管理员补齐全局字段配置，不增加隐式授权。
- **ASSUMPTION-003**: 首版不改变自动/手动部署的现有业务语义，提供可见的实际流程摘要和下一步动作；新的自动部署策略若有需求，另行评估状态机变更。

## 8. Related Specifications / Further Reading

- [当前初始化说明](/Users/lingyunxieqing/Desktop/gos/README.md:563)
- [当前 quick-start 路由](/Users/lingyunxieqing/Desktop/gos/frontend/src/router/index.ts:295)
- [内置标准字段初始化](/Users/lingyunxieqing/Desktop/gos/internal/infrastructure/persistence/sqlrepo/platform_param_repository.go:149)
- [应用创建的现有校验](/Users/lingyunxieqing/Desktop/gos/internal/application/usecase/create.go:31)
- [执行器参数映射约束](/Users/lingyunxieqing/Desktop/gos/internal/application/usecase/executor_param_def.go:229)
- [模板创建和应用级审批边界](/Users/lingyunxieqing/Desktop/gos/internal/application/usecase/release_template.go:190)
- [运行时 CI 来源规则](/Users/lingyunxieqing/Desktop/gos/internal/application/usecase/jenkins_upstream_params.go:16)
- [现有发布预检](/Users/lingyunxieqing/Desktop/gos/internal/application/usecase/release_order_precheck.go:68)
- [现有首单页面上下文](/Users/lingyunxieqing/Desktop/gos/frontend/src/views/release/ReleaseOrderCreateView.vue:448)
