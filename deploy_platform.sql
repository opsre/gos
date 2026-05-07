-- ============================================================
-- GOS 数据库完整表结构导出
-- 生成时间: 2026-05-07
-- 说明: 包含所有内置字段信息
-- ============================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- --------------------------------------------------------
-- 1. 系统模块 - sys_user (用户表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `sys_user` (
    `id` VARCHAR(64) NOT NULL COMMENT '用户唯一标识，格式: usr-xxx',
    `username` VARCHAR(100) NOT NULL COMMENT '用户名，登录凭证',
    `display_name` VARCHAR(100) NOT NULL COMMENT '显示名称',
    `email` VARCHAR(200) NOT NULL DEFAULT '' COMMENT '邮箱',
    `phone` VARCHAR(50) NOT NULL DEFAULT '' COMMENT '电话号码',
    `role` VARCHAR(20) NOT NULL COMMENT '角色: admin=管理员, normal=普通用户',
    `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态: active=激活, inactive=停用',
    `password_hash` VARCHAR(255) NOT NULL COMMENT 'bcrypt加密后的密码哈希',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_sys_user_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户信息表';

-- --------------------------------------------------------
-- 2. 系统模块 - sys_permission (权限表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `sys_permission` (
    `id` VARCHAR(64) NOT NULL COMMENT '权限唯一标识',
    `code` VARCHAR(100) NOT NULL COMMENT '权限代码，如: release.view',
    `name` VARCHAR(100) NOT NULL COMMENT '权限名称，如: 查看发布单',
    `module` VARCHAR(50) NOT NULL COMMENT '所属模块: application/pipeline/release/system等',
    `action` VARCHAR(50) NOT NULL COMMENT '操作类型: view/manage/create等',
    `description` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '权限描述说明',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_sys_permission_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限定义表';

-- --------------------------------------------------------
-- 3. 系统模块 - sys_user_permission (用户权限关联表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `sys_user_permission` (
    `id` VARCHAR(64) NOT NULL COMMENT '记录唯一标识',
    `user_id` VARCHAR(64) NOT NULL COMMENT '关联用户ID',
    `permission_code` VARCHAR(100) NOT NULL COMMENT '关联权限代码',
    `scope_type` VARCHAR(30) NOT NULL DEFAULT 'global' COMMENT '权限范围: global=全局, application=应用级',
    `scope_value` VARCHAR(200) NOT NULL DEFAULT '' COMMENT '范围值，scope_type=application时为应用ID',
    `enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用: 1=启用, 0=禁用',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_sup_unique` (`user_id`, `permission_code`, `scope_type`, `scope_value`),
    KEY `idx_sup_user` (`user_id`),
    KEY `idx_sup_code` (`permission_code`),
    KEY `idx_sup_scope` (`scope_type`, `scope_value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户权限关联表';

-- --------------------------------------------------------
-- 4. 系统模块 - sys_user_param_permission (用户参数权限表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `sys_user_param_permission` (
    `id` VARCHAR(64) NOT NULL COMMENT '记录唯一标识',
    `user_id` VARCHAR(64) NOT NULL COMMENT '关联用户ID',
    `param_key` VARCHAR(100) NOT NULL COMMENT '参数键名',
    `application_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '应用ID，空表示全局参数',
    `can_view` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否可查看: 1=是, 0=否',
    `can_edit` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否可编辑: 1=是, 0=否',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_supp_unique` (`user_id`, `param_key`, `application_id`),
    KEY `idx_supp_user` (`user_id`),
    KEY `idx_supp_param` (`param_key`),
    KEY `idx_supp_app` (`application_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户参数权限控制表';

-- --------------------------------------------------------
-- 5. 系统模块 - sys_user_session (用户会话表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `sys_user_session` (
    `id` VARCHAR(64) NOT NULL COMMENT '会话唯一标识，格式: ses-xxx',
    `user_id` VARCHAR(64) NOT NULL COMMENT '关联用户ID',
    `access_token` VARCHAR(512) NOT NULL COMMENT '访问令牌，64位十六进制随机字符串',
    `expired_at` BIGINT NOT NULL COMMENT '过期时间，Unix纳秒时间戳',
    `client_ip` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '登录时客户端IP地址',
    `user_agent` VARCHAR(300) NOT NULL DEFAULT '' COMMENT '登录时User-Agent头',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_sus_token` (`access_token`),
    KEY `idx_sus_user` (`user_id`),
    KEY `idx_sus_expired` (`expired_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户会话表(基于数据库的Token存储)';

-- --------------------------------------------------------
-- 6. 项目模块 - projects (项目表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `projects` (
    `id` VARCHAR(64) NOT NULL COMMENT '项目唯一标识',
    `name` VARCHAR(128) NOT NULL COMMENT '项目名称',
    `project_key` VARCHAR(128) NOT NULL COMMENT '项目标识Key，用于URL和关联',
    `description` TEXT NOT NULL COMMENT '项目描述',
    `status` VARCHAR(32) NOT NULL COMMENT '状态: active=活跃, archived=归档',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_project_key` (`project_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目表';

-- --------------------------------------------------------
-- 7. 系统模块 - announcements (公告表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `announcements` (
    `id` VARCHAR(64) NOT NULL COMMENT '公告唯一标识',
    `title` VARCHAR(256) NOT NULL COMMENT '公告标题',
    `content` TEXT NOT NULL COMMENT '公告内容，支持富文本',
    `enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用: 1=启用, 0=禁用',
    `start_time` BIGINT NOT NULL COMMENT '生效开始时间，Unix纳秒时间戳',
    `end_time` BIGINT NOT NULL COMMENT '生效结束时间，Unix纳秒时间戳',
    `created_by` VARCHAR(128) NOT NULL COMMENT '创建人用户ID',
    `updated_by` VARCHAR(128) NOT NULL COMMENT '更新人用户ID',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统公告表';

-- --------------------------------------------------------
-- 8. 组件模块 - k8s_cluster_refs (K8s集群引用表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `k8s_cluster_refs` (
    `id` VARCHAR(64) NOT NULL COMMENT '集群引用唯一标识',
    `code` VARCHAR(64) NOT NULL COMMENT '集群代码，唯一标识',
    `cluster_name` VARCHAR(128) NOT NULL COMMENT '集群显示名称',
    `environment_code` VARCHAR(64) NOT NULL COMMENT '关联的环境代码',
    `api_server` VARCHAR(256) NOT NULL COMMENT 'K8s API Server地址',
    `default_namespace` VARCHAR(128) NOT NULL DEFAULT 'default' COMMENT '默认命名空间',
    `access_mode` VARCHAR(32) NOT NULL DEFAULT 'argocd' COMMENT '访问模式: argocd=通过ArgoCD访问, direct=直连',
    `argocd_instance_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联的ArgoCD实例ID',
    `supports_native_strategy` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否支持K8s原生策略: 1=支持, 0=不支持',
    `supports_rollouts` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否支持ArgoRollouts: 1=支持, 0=不支持',
    `traffic_provider` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '流量管理提供者: istio/nginx/envoy等',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_k8s_cluster_ref_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='K8s集群引用表';

-- --------------------------------------------------------
-- 9. 平台参数 - platform_param_dict (平台参数字典表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `platform_param_dict` (
    `id` VARCHAR(64) NOT NULL COMMENT '参数唯一标识',
    `param_key` VARCHAR(100) NOT NULL COMMENT '参数键名，全局唯一',
    `name` VARCHAR(100) NOT NULL COMMENT '参数显示名称',
    `description` VARCHAR(500) NOT NULL COMMENT '参数描述说明',
    `param_type` VARCHAR(50) NOT NULL COMMENT '参数类型: string/number/boolean/select等',
    `required` TINYINT(1) NOT NULL COMMENT '是否必填: 1=必填, 0=可选',
    `gitops_locator` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否GitOps定位器: 1=是, 0=否',
    `cd_self_fill` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否CD自填: 1=是, 0=否',
    `builtin` TINYINT(1) NOT NULL COMMENT '是否内置参数: 1=内置, 0=自定义',
    `status` TINYINT(1) NOT NULL COMMENT '状态: 1=启用, 0=禁用',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_platform_param_key` (`param_key`),
    KEY `idx_platform_param_status_updated_at` (`status`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='平台参数字典表';

-- --------------------------------------------------------
-- 10. 策略模块 - release_strategy_templates (发布策略模板表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_strategy_templates` (
    `id` VARCHAR(64) NOT NULL COMMENT '策略模板唯一标识',
    `name` VARCHAR(128) NOT NULL COMMENT '策略模板名称',
    `strategy_engine` VARCHAR(32) NOT NULL DEFAULT 'k8s_native' COMMENT '策略引擎: k8s_native/K8s原生, argo_rollouts/ArgoRollouts',
    `strategy_type` VARCHAR(32) NOT NULL DEFAULT 'rolling_update' COMMENT '策略类型: rolling_update/滚动更新, canary/金丝雀, blue_green/蓝绿部署',
    `strategy_config` TEXT NOT NULL COMMENT '策略配置JSON，含副本数/最大Surge/最大Unavailable等',
    `description` TEXT NOT NULL COMMENT '策略模板描述',
    `status` VARCHAR(32) NOT NULL DEFAULT 'active' COMMENT '状态: active=激活, inactive=停用',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_release_strategy_template_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布策略模板表';

-- --------------------------------------------------------
-- 11. 策略模块 - application_env_runtime_bindings (应用环境运行时绑定表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `application_env_runtime_bindings` (
    `id` VARCHAR(64) NOT NULL COMMENT '运行时绑定唯一标识',
    `application_id` VARCHAR(64) NOT NULL COMMENT '关联应用ID',
    `env_code` VARCHAR(64) NOT NULL COMMENT '环境代码',
    `k8s_cluster_ref_id` VARCHAR(64) NOT NULL COMMENT '关联K8s集群引用ID',
    `namespace` VARCHAR(128) NOT NULL DEFAULT 'default' COMMENT 'K8s命名空间',
    `workload_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '工作负载名称(Deployment/StatefulSet等)',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_app_env_runtime` (`application_id`, `env_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用环境运行时绑定表(记录app在K8s中的落点)';

-- --------------------------------------------------------
-- 12. 策略模块 - application_env_strategy_bindings (应用环境策略绑定表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `application_env_strategy_bindings` (
    `id` VARCHAR(64) NOT NULL COMMENT '策略绑定唯一标识',
    `application_id` VARCHAR(64) NOT NULL COMMENT '关联应用ID',
    `env_code` VARCHAR(64) NOT NULL COMMENT '环境代码',
    `strategy_template_id` VARCHAR(64) NOT NULL COMMENT '关联策略模板ID',
    `overrides_config` TEXT NOT NULL COMMENT '策略覆盖配置JSON',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_app_env_strategy` (`application_id`, `env_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用环境策略绑定表';

-- --------------------------------------------------------
-- 13. 发布模块 - release_order (发布单主表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order` (
    `id` VARCHAR(64) NOT NULL COMMENT '发布单唯一标识',
    `order_no` VARCHAR(64) NOT NULL COMMENT '发布单号，全局唯一',
    `release_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '发布单名称/标题',
    `previous_order_no` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '上一张发布单号，用于回滚追溯',
    `operation_type` VARCHAR(32) NOT NULL DEFAULT 'deploy' COMMENT '操作类型: deploy=正常发布, rollback=标准回滚, replay=管线重放',
    `source_order_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '恢复来源发布单ID(rollback/replay时)',
    `source_order_no` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '恢复来源发布单号',
    `is_concurrent` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否并发发布: 1=是, 0=否',
    `concurrent_batch_no` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '并发批次号',
    `concurrent_batch_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '并发批次名称',
    `concurrent_batch_seq` INT NOT NULL DEFAULT 0 COMMENT '并发批次序号',
    `application_id` VARCHAR(64) NOT NULL COMMENT '关联应用ID',
    `application_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '应用名称(冗余存储)',
    `template_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联发布模板ID',
    `template_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '发布模板名称',
    `binding_id` VARCHAR(64) NOT NULL COMMENT '关联模板绑定ID',
    `pipeline_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联管线ID',
    `env_code` VARCHAR(50) NOT NULL COMMENT '目标环境代码',
    `son_service` VARCHAR(200) NOT NULL DEFAULT '' COMMENT '子服务标识',
    `git_ref` VARCHAR(200) NOT NULL DEFAULT '' COMMENT 'Git引用: 分支/tag/commit',
    `image_tag` VARCHAR(200) NOT NULL DEFAULT '' COMMENT '镜像版本标签',
    `trigger_type` VARCHAR(50) NOT NULL COMMENT '触发类型: manual=手动, scheduled=定时, webhook= webhook触发, api=API触发',
    `status` VARCHAR(50) NOT NULL DEFAULT 'pending' COMMENT '发布单状态: pending/pending_approval/approving/approved/running/success/failed/cancelled',
    `approval_required` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否需要审批: 1=是, 0=否',
    `approval_mode` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '审批模式: manual=人工审批, auto=自动通过',
    `approval_approver_ids_json` TEXT NOT NULL COMMENT '审批人ID列表JSON',
    `approval_approver_names_json` TEXT NOT NULL COMMENT '审批人名称列表JSON',
    `approved_at` BIGINT NULL COMMENT '审批通过时间，Unix纳秒时间戳',
    `approved_by` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '审批人用户ID',
    `rejected_at` BIGINT NULL COMMENT '审批拒绝时间，Unix纳秒时间戳',
    `rejected_by` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '拒绝人用户ID',
    `rejected_reason` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '拒绝原因',
    `remark` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '发布备注说明',
    `creator_user_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '创建人用户ID',
    `triggered_by` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '触发人用户ID',
    `executor_user_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '执行人用户ID',
    `executor_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '执行人名称',
    `started_at` BIGINT NULL COMMENT '开始执行时间，Unix纳秒时间戳',
    `finished_at` BIGINT NULL COMMENT '执行完成时间，Unix纳秒时间戳',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_order_no` (`order_no`),
    KEY `idx_release_order_application` (`application_id`),
    KEY `idx_release_order_binding` (`binding_id`),
    KEY `idx_release_order_batch` (`concurrent_batch_no`),
    KEY `idx_release_order_batch_name` (`concurrent_batch_name`),
    KEY `idx_release_order_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单主表';

-- --------------------------------------------------------
-- 14. 发布模块 - release_order_execution (发布单执行记录表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_execution` (
    `id` VARCHAR(64) NOT NULL COMMENT '执行记录唯一标识',
    `release_order_id` VARCHAR(64) NOT NULL COMMENT '关联发布单ID',
    `pipeline_scope` VARCHAR(20) NOT NULL COMMENT '管线作用域: ci=构建阶段, cd=部署阶段',
    `binding_id` VARCHAR(64) NOT NULL COMMENT '关联模板绑定ID',
    `binding_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '绑定名称',
    `provider` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '提供者: jenkins/argocd/gitops/agent',
    `pipeline_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '管线ID',
    `status` VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT '执行状态: pending/running/success/failed',
    `queue_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '任务队列URL',
    `build_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '构建URL',
    `external_run_id` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '外部系统运行ID(Jenkins Build Number等)',
    `started_at` BIGINT NULL COMMENT '开始执行时间',
    `finished_at` BIGINT NULL COMMENT '执行完成时间',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_order_execution_scope` (`release_order_id`, `pipeline_scope`),
    KEY `idx_release_order_execution_order` (`release_order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单执行记录表';

-- --------------------------------------------------------
-- 15. 发布模块 - release_order_deploy_snapshot (发布单部署快照表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_deploy_snapshot` (
    `id` VARCHAR(64) NOT NULL COMMENT '快照唯一标识',
    `release_order_id` VARCHAR(64) NOT NULL COMMENT '关联发布单ID',
    `provider` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '提供者: argocd',
    `gitops_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'GitOps类型: helm/kustomize',
    `argocd_instance_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'ArgoCD实例ID',
    `gitops_instance_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'GitOps实例ID',
    `argocd_app_name` VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'ArgoCD Application名称',
    `repo_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT 'Git仓库URL',
    `branch` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '分支名',
    `source_path` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '源码路径',
    `env_code` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '环境代码',
    `snapshot_payload_json` LONGTEXT NOT NULL COMMENT '部署快照JSON，包含完整Helm values',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_order_snapshot_order` (`release_order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单部署快照表(用于回滚)';

-- --------------------------------------------------------
-- 16. 发布模块 - release_execution_lock (发布执行锁表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_execution_lock` (
    `id` VARCHAR(64) NOT NULL COMMENT '锁记录唯一标识',
    `lock_scope` VARCHAR(32) NOT NULL COMMENT '锁作用域: application/env/global',
    `lock_key` VARCHAR(500) NOT NULL COMMENT '锁键值',
    `application_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '应用ID',
    `env_code` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '环境代码',
    `release_order_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '持锁的发布单ID',
    `release_order_no` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '持锁的发布单号',
    `status` VARCHAR(32) NOT NULL DEFAULT 'active' COMMENT '锁状态: active=持锁中, released=已释放',
    `owner_type` VARCHAR(32) NOT NULL DEFAULT 'release_order' COMMENT '持有者类型',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `expired_at` BIGINT NULL COMMENT '锁过期时间',
    `released_at` BIGINT NULL COMMENT '锁释放时间',
    PRIMARY KEY (`id`),
    KEY `idx_release_execution_lock_key_status` (`lock_key`, `status`),
    KEY `idx_release_execution_lock_order` (`release_order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布执行并发控制锁表';

-- --------------------------------------------------------
-- 17. 发布模块 - release_order_param (发布单参数表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_param` (
    `id` VARCHAR(64) NOT NULL COMMENT '参数记录唯一标识',
    `release_order_id` VARCHAR(64) NOT NULL COMMENT '关联发布单ID',
    `pipeline_scope` VARCHAR(20) NOT NULL DEFAULT '' COMMENT '参数作用域: ci/cd/global',
    `binding_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联绑定ID',
    `param_key` VARCHAR(100) NOT NULL COMMENT '参数键名',
    `executor_param_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '执行器参数名称',
    `param_value` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '参数值',
    `value_source` VARCHAR(50) NOT NULL COMMENT '值来源: release_input/user_input/fixed/deploy_snapshot/platform',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    KEY `idx_release_order_param_order` (`release_order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单参数表';

-- --------------------------------------------------------
-- 18. 发布模块 - release_order_step (发布单步骤表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_step` (
    `id` VARCHAR(64) NOT NULL COMMENT '步骤记录唯一标识',
    `release_order_id` VARCHAR(64) NOT NULL COMMENT '关联发布单ID',
    `step_scope` VARCHAR(20) NOT NULL DEFAULT 'global' COMMENT '步骤作用域: global/ci/cd',
    `execution_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联执行记录ID',
    `step_code` VARCHAR(100) NOT NULL COMMENT '步骤代码，如: cd:gitops_update/cd:argocd_sync/cd:health_check',
    `step_name` VARCHAR(200) NOT NULL DEFAULT '' COMMENT '步骤显示名称',
    `status` VARCHAR(50) NOT NULL COMMENT '步骤状态: pending/running/success/failed/skipped',
    `message` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '步骤消息/错误信息',
    `sort_no` INT NOT NULL DEFAULT 0 COMMENT '排序序号',
    `started_at` BIGINT NULL COMMENT '开始执行时间',
    `finished_at` BIGINT NULL COMMENT '执行完成时间',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_order_step_code` (`release_order_id`, `step_code`),
    KEY `idx_release_order_step_order_sort` (`release_order_id`, `sort_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单步骤执行记录表';

-- --------------------------------------------------------
-- 19. 发布模块 - release_order_pipeline_stage (发布单管线阶段表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_pipeline_stage` (
    `id` VARCHAR(64) NOT NULL COMMENT '阶段记录唯一标识',
    `release_order_id` VARCHAR(64) NOT NULL COMMENT '关联发布单ID',
    `execution_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联执行记录ID',
    `pipeline_scope` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '管线作用域',
    `executor_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '执行器类型: jenkins/argocd',
    `stage_key` VARCHAR(128) NOT NULL COMMENT '阶段标识',
    `stage_name` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '阶段名称',
    `status` VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT '阶段状态',
    `raw_status` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '原始状态(来自外部系统)',
    `sort_no` INT NOT NULL DEFAULT 0 COMMENT '排序序号',
    `duration_millis` BIGINT NOT NULL DEFAULT 0 COMMENT '持续时间(毫秒)',
    `started_at` BIGINT NULL COMMENT '开始时间',
    `finished_at` BIGINT NULL COMMENT '结束时间',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_order_pipeline_stage_key` (`release_order_id`, `executor_type`, `pipeline_scope`, `stage_key`),
    KEY `idx_release_order_pipeline_stage_order_sort` (`release_order_id`, `pipeline_scope`, `sort_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单管线阶段执行记录表';

-- --------------------------------------------------------
-- 20. 发布模块 - release_template (发布模板表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_template` (
    `id` VARCHAR(64) NOT NULL COMMENT '发布模板唯一标识',
    `name` VARCHAR(128) NOT NULL COMMENT '发布模板名称',
    `application_id` VARCHAR(64) NOT NULL COMMENT '关联应用ID',
    `application_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '应用名称(冗余)',
    `binding_id` VARCHAR(64) NOT NULL COMMENT '关联管线绑定ID',
    `binding_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '绑定名称',
    `binding_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '绑定类型: jenkins/gitops/agent',
    `gitops_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'GitOps类型: helm/kustomize',
    `status` VARCHAR(32) NOT NULL DEFAULT 'active' COMMENT '状态: active/inactive',
    `approval_enabled` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否启用审批: 1=是, 0=否',
    `approval_mode` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '审批模式',
    `approval_approver_ids_json` TEXT NOT NULL COMMENT '审批人ID列表JSON',
    `approval_approver_names_json` TEXT NOT NULL COMMENT '审批人名称JSON',
    `remark` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '模板备注',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_template_binding_name` (`binding_id`, `name`),
    KEY `idx_release_template_application` (`application_id`),
    KEY `idx_release_template_binding` (`binding_id`),
    KEY `idx_release_template_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布模板表';

-- --------------------------------------------------------
-- 21. 发布模块 - release_template_binding (发布模板绑定表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_template_binding` (
    `id` VARCHAR(64) NOT NULL COMMENT '绑定记录唯一标识',
    `template_id` VARCHAR(64) NOT NULL COMMENT '关联发布模板ID',
    `pipeline_scope` VARCHAR(20) NOT NULL COMMENT '管线作用域: ci/cd',
    `binding_id` VARCHAR(64) NOT NULL COMMENT '管线绑定ID',
    `binding_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '绑定名称',
    `provider` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '提供者: jenkins/argocd/gitops/agent',
    `pipeline_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '管线ID',
    `enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用: 1=启用, 0=禁用',
    `sort_no` INT NOT NULL DEFAULT 1 COMMENT '排序序号',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_template_scope` (`template_id`, `pipeline_scope`),
    KEY `idx_release_template_binding_template` (`template_id`),
    KEY `idx_release_template_binding_binding` (`binding_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布模板绑定表';

-- --------------------------------------------------------
-- 22. 发布模块 - release_template_param (发布模板参数定义表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_template_param` (
    `id` VARCHAR(64) NOT NULL COMMENT '参数定义唯一标识',
    `template_id` VARCHAR(64) NOT NULL COMMENT '关联发布模板ID',
    `template_binding_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联模板绑定ID',
    `pipeline_scope` VARCHAR(20) NOT NULL DEFAULT '' COMMENT '管线作用域',
    `binding_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联绑定ID',
    `executor_param_def_id` VARCHAR(64) NOT NULL COMMENT '执行器参数定义ID',
    `param_key` VARCHAR(100) NOT NULL COMMENT '参数键名',
    `param_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '参数显示名称',
    `executor_param_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '执行器参数名称',
    `value_source` VARCHAR(32) NOT NULL DEFAULT 'release_input' COMMENT '值来源: release_input/user_input/fixed/platform',
    `source_param_key` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '源参数键名',
    `source_param_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '源参数名称',
    `fixed_value` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '固定值(value_source=fixed时)',
    `required` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否必填: 1=必填, 0=可选',
    `sort_no` INT NOT NULL DEFAULT 0 COMMENT '排序序号',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_template_param_unique` (`template_id`, `executor_param_def_id`),
    KEY `idx_release_template_param_template_sort` (`template_id`, `sort_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布模板参数定义表';

-- --------------------------------------------------------
-- 23. 发布模块 - release_template_gitops_rule (发布模板GitOps渲染规则表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_template_gitops_rule` (
    `id` VARCHAR(64) NOT NULL COMMENT '规则唯一标识',
    `template_id` VARCHAR(64) NOT NULL COMMENT '关联发布模板ID',
    `pipeline_scope` VARCHAR(20) NOT NULL DEFAULT 'cd' COMMENT '管线作用域',
    `source_param_key` VARCHAR(100) NOT NULL COMMENT '源参数键名',
    `source_param_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '源参数名称',
    `source_from` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '来源: param/git_ref/image_tag',
    `locator_param_key` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '定位器参数键名',
    `locator_param_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '定位器参数名称',
    `file_path_template` VARCHAR(255) NOT NULL COMMENT '文件路径模板',
    `document_kind` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '文档类型: ConfigMap/Secret/Deployment',
    `document_name` VARCHAR(150) NOT NULL DEFAULT '' COMMENT '文档名称',
    `target_path` VARCHAR(255) NOT NULL COMMENT '目标路径',
    `value_template` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '值模板',
    `sort_no` INT NOT NULL DEFAULT 0 COMMENT '排序序号',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    KEY `idx_release_template_gitops_rule_template_sort` (`template_id`, `sort_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布模板GitOps渲染规则表';

-- --------------------------------------------------------
-- 24. 发布模块 - release_template_hook (发布模板Webhook钩子表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_template_hook` (
    `id` VARCHAR(64) NOT NULL COMMENT '钩子唯一标识',
    `template_id` VARCHAR(64) NOT NULL COMMENT '关联发布模板ID',
    `hook_type` VARCHAR(64) NOT NULL COMMENT '钩子类型: webhook/notification',
    `name` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '钩子名称',
    `execute_stage` VARCHAR(32) NOT NULL DEFAULT 'post_release' COMMENT '执行阶段: pre_release/post_release',
    `execute_stages_json` TEXT NOT NULL COMMENT '执行的阶段列表JSON',
    `trigger_condition` VARCHAR(32) NOT NULL DEFAULT 'on_success' COMMENT '触发条件: on_success/on_failure/always',
    `failure_policy` VARCHAR(32) NOT NULL DEFAULT 'warn_only' COMMENT '失败策略: warn_only/block',
    `env_codes_json` TEXT NOT NULL COMMENT '适用环境列表JSON',
    `target_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '目标ID',
    `target_name` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '目标名称',
    `webhook_method` VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'HTTP方法: GET/POST/PUT',
    `webhook_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT 'Webhook URL',
    `webhook_body` TEXT NOT NULL COMMENT 'Webhook请求体模板',
    `note` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '备注说明',
    `sort_no` INT NOT NULL DEFAULT 0 COMMENT '排序序号',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    KEY `idx_release_template_hook_template_sort` (`template_id`, `sort_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布模板Webhook钩子表';

-- --------------------------------------------------------
-- 25. 发布模块 - release_order_approval_record (发布单审批记录表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_approval_record` (
    `id` VARCHAR(64) NOT NULL COMMENT '审批记录唯一标识',
    `release_order_id` VARCHAR(64) NOT NULL COMMENT '关联发布单ID',
    `action` VARCHAR(32) NOT NULL COMMENT '操作类型: approve=通过, reject=拒绝, submit=提交审批',
    `operator_user_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '操作用户ID',
    `operator_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '操作人名称',
    `comment` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '审批意见',
    `created_at` BIGINT NOT NULL COMMENT '操作时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    KEY `idx_release_order_approval_record_order_created` (`release_order_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单审批记录表';

-- --------------------------------------------------------
-- 26. 发布模块 - release_order_schedule (发布单计划表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_schedule` (
    `id` VARCHAR(64) NOT NULL COMMENT '计划唯一标识',
    `schedule_no` VARCHAR(64) NOT NULL COMMENT '计划单号，全局唯一',
    `release_order_id` VARCHAR(64) NOT NULL COMMENT '关联发布单ID',
    `release_order_no` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联发布单号',
    `application_id` VARCHAR(64) NOT NULL COMMENT '关联应用ID',
    `application_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '应用名称',
    `env_code` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '环境代码',
    `template_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '发布模板ID',
    `template_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '发布模板名称',
    `schedule_mode` VARCHAR(32) NOT NULL COMMENT '计划模式: immediate=立即, scheduled=定时, build_triggered=构建触发',
    `build_scheduled_at` BIGINT NULL COMMENT '构建计划时间',
    `deploy_scheduled_at` BIGINT NULL COMMENT '部署计划时间',
    `execute_scheduled_at` BIGINT NULL COMMENT '执行计划时间',
    `cd_conflict_at` BIGINT NULL COMMENT 'CD冲突检测时间',
    `timezone` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '时区',
    `status` VARCHAR(32) NOT NULL COMMENT '状态: pending_approval/approving/scheduled/dispatching/running/success/failed/cancelled/expired',
    `approval_required` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否需要审批',
    `approval_mode` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '审批模式',
    `approval_approver_ids_json` TEXT NOT NULL COMMENT '审批人ID列表JSON',
    `approval_approver_names_json` TEXT NOT NULL COMMENT '审批人名称JSON',
    `approved_at` BIGINT NULL COMMENT '审批通过时间',
    `approved_by` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '审批人',
    `rejected_at` BIGINT NULL COMMENT '审批拒绝时间',
    `rejected_by` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '拒绝人',
    `rejected_reason` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '拒绝原因',
    `build_dispatched_at` BIGINT NULL COMMENT '构建已下发时间',
    `deploy_dispatched_at` BIGINT NULL COMMENT '部署已下发时间',
    `execute_dispatched_at` BIGINT NULL COMMENT '执行已下发时间',
    `expired_at` BIGINT NULL COMMENT '过期时间',
    `cancelled_at` BIGINT NULL COMMENT '取消时间',
    `cancelled_by` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '取消人',
    `last_error` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '最后错误信息',
    `remark` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '备注',
    `creator_user_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '创建人',
    `creator_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '创建人名称',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_order_schedule_no` (`schedule_no`),
    KEY `idx_release_order_schedule_order_status` (`release_order_id`, `status`),
    KEY `idx_release_order_schedule_cd_conflict` (`application_id`, `env_code`, `cd_conflict_at`, `status`),
    KEY `idx_release_order_schedule_status_build` (`status`, `build_scheduled_at`),
    KEY `idx_release_order_schedule_status_deploy` (`status`, `deploy_scheduled_at`),
    KEY `idx_release_order_schedule_status_execute` (`status`, `execute_scheduled_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单定时计划表';

-- --------------------------------------------------------
-- 27. 发布模块 - release_order_schedule_approval_record (发布单计划审批记录表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_schedule_approval_record` (
    `id` VARCHAR(64) NOT NULL COMMENT '审批记录唯一标识',
    `schedule_id` VARCHAR(64) NOT NULL COMMENT '关联计划ID',
    `action` VARCHAR(32) NOT NULL COMMENT '操作类型: approve/reject/submit',
    `operator_user_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '操作用户ID',
    `operator_name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '操作人名称',
    `comment` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '审批意见',
    `created_at` BIGINT NOT NULL COMMENT '操作时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    KEY `idx_release_order_schedule_approval_record_schedule_created` (`schedule_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单计划审批记录表';

SET FOREIGN_KEY_CHECKS = 1;
