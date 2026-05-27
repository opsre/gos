-- ============================================================
-- GOS 数据库增量更新脚本 v1.1
-- 基准版本: git 上 2026-05-07 版本 deploy_platform.sql
-- 目标版本: 当前 deploy_platform.sql 表结构
-- 说明: 补齐 v1.1 新增字段和新增表；不包含环境配置数据、AccessKey、API Key 等密钥数据。
-- 执行方式: mysql <database> < deploy_platform-update-v1.1.sql
-- ============================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- --------------------------------------------------------
-- 1. 兼容重复执行的字段添加辅助过程
-- --------------------------------------------------------
DROP PROCEDURE IF EXISTS gos_add_column_if_missing;
DELIMITER $$
CREATE PROCEDURE gos_add_column_if_missing(
    IN p_table_name VARCHAR(128),
    IN p_column_name VARCHAR(128),
    IN p_column_definition TEXT,
    IN p_position_clause TEXT
)
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = p_table_name
          AND COLUMN_NAME = p_column_name
    ) THEN
        SET @gos_add_column_sql = CONCAT(
            'ALTER TABLE `', REPLACE(p_table_name, '`', '``'),
            '` ADD COLUMN `', REPLACE(p_column_name, '`', '``'),
            '` ', p_column_definition, ' ', p_position_clause
        );
        PREPARE gos_add_column_stmt FROM @gos_add_column_sql;
        EXECUTE gos_add_column_stmt;
        DEALLOCATE PREPARE gos_add_column_stmt;
    END IF;
END$$
DELIMITER ;

-- --------------------------------------------------------
-- 2. 已有表新增字段
-- --------------------------------------------------------
CALL gos_add_column_if_missing('sys_user_session', 'revoked_at', 'BIGINT NULL COMMENT ''撤销时间，Unix纳秒时间戳''', 'AFTER `user_agent`');
CALL gos_add_column_if_missing('sys_user_session', 'revoked_reason', 'VARCHAR(64) NOT NULL DEFAULT '''' COMMENT ''撤销原因''', 'AFTER `revoked_at`');

CALL gos_add_column_if_missing('release_order', 'delivery_engine', 'VARCHAR(32) NOT NULL DEFAULT ''k8s_native'' COMMENT ''交付引擎: k8s_native/argo_rollouts''', 'AFTER `template_name`');
CALL gos_add_column_if_missing('release_order', 'strategy_snapshot_json', 'LONGTEXT NULL COMMENT ''发布策略快照JSON''', 'AFTER `delivery_engine`');

UPDATE `release_order`
SET `strategy_snapshot_json` = ''
WHERE `strategy_snapshot_json` IS NULL;

ALTER TABLE `release_order`
    MODIFY COLUMN `strategy_snapshot_json` LONGTEXT NOT NULL COMMENT '发布策略快照JSON';

-- --------------------------------------------------------
-- 3. v1.1 新增表
-- --------------------------------------------------------

-- --------------------------------------------------------
-- 9.1 制品中心 - artifact_repository_config (制品库配置表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `artifact_repository_config` (
    `id` VARCHAR(64) NOT NULL COMMENT '制品库唯一标识',
    `name` VARCHAR(100) NOT NULL COMMENT '制品库名称',
    `repository_type` VARCHAR(50) NOT NULL COMMENT '制品库类型: oss=对象存储',
    `endpoint` VARCHAR(500) NOT NULL COMMENT '对象存储Endpoint',
    `bucket` VARCHAR(200) NOT NULL COMMENT 'Bucket名称',
    `directory` VARCHAR(500) NOT NULL COMMENT '目录前缀',
    `access_key_id` VARCHAR(255) NOT NULL COMMENT 'AccessKey ID',
    `access_key_secret_ciphertext` TEXT NOT NULL COMMENT '加密后的AccessKey Secret',
    `acl` VARCHAR(50) NOT NULL COMMENT '默认ACL: private/public-read',
    `status` VARCHAR(50) NOT NULL COMMENT '状态: enabled=启用, disabled=停用',
    `created_at` BIGINT NOT NULL COMMENT '创建时间，Unix纳秒时间戳',
    `updated_at` BIGINT NOT NULL COMMENT '更新时间，Unix纳秒时间戳',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_artifact_repository_name` (`name`),
    KEY `idx_artifact_repository_type_status_updated_at` (`repository_type`, `status`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='制品库配置表';

-- --------------------------------------------------------
-- 28. Agent模块 - agent_bootstrap_token (Agent引导令牌表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `agent_bootstrap_token` (
  `id` varchar(32) NOT NULL,
  `token_ciphertext` text NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 29. Agent模块 - agent_instance (Agent实例表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `agent_instance` (
  `id` varchar(64) NOT NULL,
  `machine_id` varchar(120) NOT NULL DEFAULT '',
  `agent_code` varchar(100) NOT NULL,
  `name` varchar(120) NOT NULL,
  `environment_code` varchar(120) NOT NULL DEFAULT '',
  `work_dir` varchar(500) NOT NULL,
  `token_ciphertext` text NOT NULL,
  `tags_json` text NOT NULL,
  `hostname` varchar(255) NOT NULL DEFAULT '',
  `host_ip` varchar(120) NOT NULL DEFAULT '',
  `agent_version` varchar(120) NOT NULL DEFAULT '',
  `os` varchar(120) NOT NULL DEFAULT '',
  `arch` varchar(120) NOT NULL DEFAULT '',
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `last_heartbeat_at` bigint NOT NULL DEFAULT '0',
  `current_task_id` varchar(120) NOT NULL DEFAULT '',
  `current_task_name` varchar(255) NOT NULL DEFAULT '',
  `current_task_type` varchar(120) NOT NULL DEFAULT '',
  `current_task_started_at` bigint NOT NULL DEFAULT '0',
  `last_task_status` varchar(20) NOT NULL DEFAULT 'unknown',
  `last_task_summary` varchar(500) NOT NULL DEFAULT '',
  `last_task_finished_at` bigint NOT NULL DEFAULT '0',
  `remark` varchar(500) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_agent_instance_code` (`agent_code`),
  KEY `idx_agent_instance_status` (`status`),
  KEY `idx_agent_instance_env` (`environment_code`),
  KEY `idx_agent_instance_heartbeat` (`last_heartbeat_at`),
  KEY `idx_agent_instance_machine` (`machine_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 30. Agent模块 - agent_script (Agent脚本表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `agent_script` (
  `id` varchar(64) NOT NULL,
  `name` varchar(160) NOT NULL,
  `description` varchar(500) NOT NULL DEFAULT '',
  `task_type` varchar(50) NOT NULL,
  `shell_type` varchar(20) NOT NULL DEFAULT 'sh',
  `script_path` varchar(500) NOT NULL DEFAULT '',
  `script_text` mediumtext NOT NULL,
  `created_by` varchar(100) NOT NULL DEFAULT '',
  `updated_by` varchar(100) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_script_type_created` (`task_type`,`created_at`),
  KEY `idx_agent_script_name_created` (`name`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 31. Agent模块 - agent_task (Agent任务表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `agent_task` (
  `id` varchar(64) NOT NULL,
  `agent_id` varchar(64) NOT NULL,
  `agent_code` varchar(100) NOT NULL,
  `target_agent_ids_json` mediumtext NOT NULL,
  `source_task_id` varchar(64) NOT NULL DEFAULT '',
  `dispatch_batch_id` varchar(64) NOT NULL DEFAULT '',
  `name` varchar(200) NOT NULL,
  `task_mode` varchar(20) NOT NULL DEFAULT 'temporary',
  `task_type` varchar(50) NOT NULL,
  `shell_type` varchar(20) NOT NULL DEFAULT 'sh',
  `work_dir` varchar(500) NOT NULL,
  `script_id` varchar(64) NOT NULL DEFAULT '',
  `script_name` varchar(200) NOT NULL DEFAULT '',
  `script_path` varchar(500) NOT NULL DEFAULT '',
  `script_text` mediumtext NOT NULL,
  `variables_json` mediumtext NOT NULL,
  `timeout_sec` int NOT NULL DEFAULT '300',
  `status` varchar(20) NOT NULL DEFAULT 'pending',
  `claimed_at` bigint NOT NULL DEFAULT '0',
  `started_at` bigint NOT NULL DEFAULT '0',
  `finished_at` bigint NOT NULL DEFAULT '0',
  `exit_code` int NOT NULL DEFAULT '0',
  `stdout_text` mediumtext NOT NULL,
  `stderr_text` mediumtext NOT NULL,
  `failure_reason` text NOT NULL,
  `run_count` int NOT NULL DEFAULT '0',
  `success_count` int NOT NULL DEFAULT '0',
  `failure_count` int NOT NULL DEFAULT '0',
  `last_run_status` varchar(20) NOT NULL DEFAULT '',
  `last_run_summary` text NOT NULL,
  `created_by` varchar(100) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_task_agent_status` (`agent_id`,`status`),
  KEY `idx_agent_task_status_created` (`status`,`created_at`),
  KEY `idx_agent_task_agent_created` (`agent_id`,`created_at`),
  KEY `idx_agent_task_agent_mode_status` (`agent_id`,`task_mode`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 32. AI模块 - ai_model_config (AI模型配置表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `ai_model_config` (
  `id` varchar(64) NOT NULL,
  `name` varchar(120) NOT NULL,
  `provider` varchar(64) NOT NULL,
  `base_url` varchar(500) NOT NULL,
  `model` varchar(160) NOT NULL,
  `api_key_cipher` text NOT NULL,
  `temperature` double NOT NULL DEFAULT '0.2',
  `max_tokens` int NOT NULL DEFAULT '2048',
  `timeout_sec` int NOT NULL DEFAULT '60',
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `is_diagnosis_model` tinyint(1) NOT NULL DEFAULT '0',
  `created_by` varchar(64) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ai_model_config_diagnosis` (`is_diagnosis_model`,`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 33. 发布模块 - app_release_state (应用发布状态表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `app_release_state` (
  `id` varchar(64) NOT NULL,
  `release_order_id` varchar(64) NOT NULL,
  `release_order_no` varchar(64) NOT NULL DEFAULT '',
  `application_id` varchar(64) NOT NULL,
  `application_name` varchar(128) NOT NULL DEFAULT '',
  `env_code` varchar(64) NOT NULL DEFAULT '',
  `operation_type` varchar(32) NOT NULL DEFAULT 'deploy',
  `template_id` varchar(64) NOT NULL DEFAULT '',
  `template_name` varchar(128) NOT NULL DEFAULT '',
  `cd_provider` varchar(32) NOT NULL DEFAULT '',
  `gitops_type` varchar(32) NOT NULL DEFAULT '',
  `has_ci_execution` tinyint(1) NOT NULL DEFAULT '0',
  `has_cd_execution` tinyint(1) NOT NULL DEFAULT '0',
  `git_ref` varchar(200) NOT NULL DEFAULT '',
  `image_tag` varchar(200) NOT NULL DEFAULT '',
  `state_status` varchar(32) NOT NULL DEFAULT 'pending_confirm',
  `is_current_live` tinyint(1) NOT NULL DEFAULT '0',
  `previous_state_id` varchar(64) NOT NULL DEFAULT '',
  `confirmed_at` bigint DEFAULT NULL,
  `confirmed_by` varchar(128) NOT NULL DEFAULT '',
  `k8s_cluster_ref_id` varchar(64) NOT NULL DEFAULT '',
  `namespace` varchar(128) NOT NULL DEFAULT 'default',
  `workload_name` varchar(128) NOT NULL DEFAULT '',
  `strategy_name` varchar(128) NOT NULL DEFAULT '',
  `strategy_type` varchar(32) NOT NULL DEFAULT '',
  `strategy_engine` varchar(32) NOT NULL DEFAULT '',
  `params_snapshot_json` longtext NOT NULL,
  `execution_snapshot_json` longtext NOT NULL,
  `deploy_snapshot_json` longtext NOT NULL,
  `result_snapshot_json` longtext NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_app_release_state_order` (`release_order_id`),
  KEY `idx_app_release_state_app_env_current` (`application_id`,`env_code`,`is_current_live`),
  KEY `idx_app_release_state_previous` (`previous_state_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 34. 应用模块 - applications (应用表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `applications` (
  `id` varchar(64) NOT NULL,
  `name` varchar(128) NOT NULL,
  `app_key` varchar(128) NOT NULL,
  `project_id` varchar(64) NOT NULL DEFAULT '',
  `repo_url` text NOT NULL,
  `description` text NOT NULL,
  `owner_user_id` varchar(64) NOT NULL DEFAULT '',
  `owner` varchar(128) NOT NULL,
  `status` varchar(32) NOT NULL,
  `artifact_type` varchar(64) NOT NULL,
  `language` varchar(64) NOT NULL,
  `artifact_repository_id` varchar(64) NOT NULL DEFAULT '',
  `artifact_directory` varchar(512) NOT NULL DEFAULT '',
  `gitops_branch_mappings` json DEFAULT NULL,
  `release_branches` json DEFAULT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_application_key` (`app_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 35. ArgoCD模块 - argocd_application (ArgoCD应用表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `argocd_application` (
  `id` varchar(64) NOT NULL,
  `argocd_instance_id` varchar(64) NOT NULL DEFAULT '',
  `instance_code` varchar(100) NOT NULL DEFAULT '',
  `instance_name` varchar(120) NOT NULL DEFAULT '',
  `cluster_name` varchar(120) NOT NULL DEFAULT '',
  `instance_base_url` varchar(500) NOT NULL DEFAULT '',
  `app_name` varchar(200) NOT NULL,
  `project` varchar(100) NOT NULL DEFAULT '',
  `repo_url` varchar(500) NOT NULL DEFAULT '',
  `source_path` varchar(500) NOT NULL DEFAULT '',
  `target_revision` varchar(200) NOT NULL DEFAULT '',
  `dest_server` varchar(500) NOT NULL DEFAULT '',
  `dest_namespace` varchar(200) NOT NULL DEFAULT '',
  `sync_status` varchar(50) NOT NULL DEFAULT '',
  `health_status` varchar(50) NOT NULL DEFAULT '',
  `operation_phase` varchar(50) NOT NULL DEFAULT '',
  `argocd_url` varchar(500) NOT NULL DEFAULT '',
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `raw_meta` json DEFAULT NULL,
  `last_synced_at` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_argocd_application_instance_name` (`argocd_instance_id`,`app_name`),
  KEY `idx_argocd_project` (`project`),
  KEY `idx_argocd_sync_status` (`sync_status`),
  KEY `idx_argocd_health_status` (`health_status`),
  KEY `idx_argocd_status` (`status`),
  KEY `idx_argocd_application_instance` (`argocd_instance_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 36. ArgoCD模块 - argocd_env_binding (ArgoCD环境绑定表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `argocd_env_binding` (
  `id` varchar(64) NOT NULL,
  `env_code` varchar(64) NOT NULL,
  `argocd_instance_id` varchar(64) NOT NULL,
  `priority` int NOT NULL DEFAULT '1',
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_argocd_env_binding_env` (`env_code`),
  KEY `idx_argocd_env_binding_instance` (`argocd_instance_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 37. ArgoCD模块 - argocd_instance (ArgoCD实例表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `argocd_instance` (
  `id` varchar(64) NOT NULL,
  `instance_code` varchar(100) NOT NULL,
  `name` varchar(120) NOT NULL,
  `base_url` varchar(500) NOT NULL,
  `insecure_skip_verify` tinyint(1) NOT NULL DEFAULT '0',
  `auth_mode` varchar(32) NOT NULL DEFAULT '',
  `token_ciphertext` text NOT NULL,
  `username` varchar(120) NOT NULL DEFAULT '',
  `password_ciphertext` text NOT NULL,
  `gitops_instance_id` varchar(64) NOT NULL DEFAULT '',
  `cluster_name` varchar(120) NOT NULL DEFAULT '',
  `default_namespace` varchar(120) NOT NULL DEFAULT '',
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `health_status` varchar(32) NOT NULL DEFAULT '',
  `last_check_at` bigint NOT NULL DEFAULT '0',
  `remark` varchar(500) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_argocd_instance_code` (`instance_code`),
  UNIQUE KEY `uk_argocd_instance_base_url` (`base_url`),
  KEY `idx_argocd_instance_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 38. Jenkins模块 - executor_param_def (执行器参数定义表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `executor_param_def` (
  `id` varchar(64) NOT NULL,
  `pipeline_id` varchar(64) NOT NULL,
  `executor_type` varchar(50) NOT NULL,
  `executor_param_name` varchar(100) NOT NULL,
  `param_key` varchar(100) NOT NULL DEFAULT '',
  `param_type` varchar(50) NOT NULL,
  `single_select` tinyint(1) NOT NULL DEFAULT '0',
  `required` tinyint(1) NOT NULL,
  `default_value` varchar(500) NOT NULL,
  `description` varchar(500) NOT NULL,
  `visible` tinyint(1) NOT NULL,
  `editable` tinyint(1) NOT NULL,
  `source_from` varchar(50) NOT NULL,
  `status` varchar(32) NOT NULL DEFAULT 'active',
  `raw_meta` json DEFAULT NULL,
  `sort_no` int NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pipeline_param_unique` (`pipeline_id`,`executor_type`,`executor_param_name`),
  KEY `idx_pipeline_param_pipeline_sort` (`pipeline_id`,`sort_no`),
  KEY `idx_pipeline_param_param_key` (`param_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 39. GitOps模块 - gitops_instance (GitOps实例表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `gitops_instance` (
  `id` varchar(64) NOT NULL,
  `instance_code` varchar(100) NOT NULL,
  `name` varchar(120) NOT NULL,
  `local_root` varchar(500) NOT NULL,
  `default_branch` varchar(120) NOT NULL DEFAULT 'master',
  `username` varchar(120) NOT NULL DEFAULT '',
  `password_ciphertext` text NOT NULL,
  `token_ciphertext` text NOT NULL,
  `author_name` varchar(120) NOT NULL DEFAULT '',
  `author_email` varchar(200) NOT NULL DEFAULT '',
  `commit_message_template` text NOT NULL,
  `command_timeout_sec` int NOT NULL DEFAULT '30',
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `remark` varchar(500) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_gitops_instance_code` (`instance_code`),
  UNIQUE KEY `uk_gitops_instance_local_root` (`local_root`),
  KEY `idx_gitops_instance_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 40. 通知模块 - notification_hook (通知Hook表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `notification_hook` (
  `id` varchar(64) NOT NULL,
  `name` varchar(200) NOT NULL,
  `source_id` varchar(64) NOT NULL,
  `markdown_template_id` varchar(64) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `remark` text NOT NULL,
  `created_by` varchar(128) NOT NULL,
  `updated_by` varchar(128) NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_hook_name` (`name`),
  KEY `idx_notification_hook_source` (`source_id`),
  KEY `idx_notification_hook_template` (`markdown_template_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 41. 通知模块 - notification_markdown_template (通知Markdown模板表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `notification_markdown_template` (
  `id` varchar(64) NOT NULL,
  `name` varchar(200) NOT NULL,
  `title_template` text NOT NULL,
  `body_template` text NOT NULL,
  `conditions_json` longtext NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `remark` text NOT NULL,
  `created_by` varchar(128) NOT NULL,
  `updated_by` varchar(128) NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_markdown_template_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 42. 通知模块 - notification_source (通知源表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `notification_source` (
  `id` varchar(64) NOT NULL,
  `name` varchar(200) NOT NULL,
  `source_type` varchar(32) NOT NULL,
  `webhook_url` text NOT NULL,
  `verification_param` text NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `remark` text NOT NULL,
  `created_by` varchar(128) NOT NULL,
  `updated_by` varchar(128) NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_source_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 43. Jenkins模块 - pipeline_bindings (应用管线绑定表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `pipeline_bindings` (
  `id` varchar(64) NOT NULL,
  `name` varchar(128) NOT NULL DEFAULT '',
  `application_id` varchar(64) NOT NULL,
  `application_name` varchar(128) NOT NULL DEFAULT '',
  `binding_type` varchar(32) NOT NULL DEFAULT 'ci',
  `provider` varchar(32) NOT NULL DEFAULT 'jenkins',
  `pipeline_id` varchar(64) NOT NULL,
  `external_ref` varchar(255) NOT NULL DEFAULT '',
  `trigger_mode` varchar(32) NOT NULL,
  `status` varchar(32) NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_binding_app_pipeline` (`application_id`,`pipeline_id`),
  UNIQUE KEY `uq_binding_app_type` (`application_id`,`binding_type`),
  KEY `idx_binding_app_created_at` (`application_id`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 44. 管线规范 - pipeline_scan_findings (管线扫描问题表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `pipeline_scan_findings` (
  `id` varchar(64) NOT NULL,
  `pipeline_id` varchar(64) NOT NULL,
  `rule_id` varchar(64) NOT NULL,
  `rule_code` varchar(128) NOT NULL,
  `rule_name` varchar(128) NOT NULL,
  `severity` varchar(32) NOT NULL,
  `line_no` int NOT NULL,
  `matched_text` text NOT NULL,
  `message` varchar(500) NOT NULL,
  `suggestion` varchar(500) NOT NULL,
  `details_json` text NOT NULL,
  `status` varchar(32) NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_pipeline_scan_finding_pipeline` (`pipeline_id`,`status`,`severity`),
  KEY `idx_pipeline_scan_finding_rule` (`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 45. 管线规范 - pipeline_scan_results (管线扫描结果表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `pipeline_scan_results` (
  `id` varchar(64) NOT NULL,
  `pipeline_id` varchar(64) NOT NULL,
  `pipeline_name` varchar(255) NOT NULL,
  `scan_status` varchar(32) NOT NULL,
  `total_findings` int NOT NULL,
  `error_count` int NOT NULL,
  `warning_count` int NOT NULL,
  `info_count` int NOT NULL,
  `script_hash` varchar(128) NOT NULL,
  `last_scanned_at` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pipeline_scan_result_pipeline` (`pipeline_id`),
  KEY `idx_pipeline_scan_result_status_updated_at` (`scan_status`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 46. 管线规范 - pipeline_scan_rules (管线扫描规则表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `pipeline_scan_rules` (
  `id` varchar(64) NOT NULL,
  `rule_code` varchar(128) NOT NULL,
  `rule_name` varchar(128) NOT NULL,
  `category` varchar(32) NOT NULL,
  `severity` varchar(32) NOT NULL,
  `enabled` tinyint(1) NOT NULL,
  `builtin` tinyint(1) NOT NULL DEFAULT '0',
  `template_validation_scopes_json` text,
  `scope_json` text NOT NULL,
  `rule_dsl_json` text NOT NULL,
  `message` varchar(500) NOT NULL,
  `suggestion` varchar(500) NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pipeline_scan_rule_code` (`rule_code`),
  KEY `idx_pipeline_scan_rule_enabled_category` (`enabled`,`category`),
  KEY `idx_pipeline_scan_rule_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 47. Jenkins模块 - pipelines (管线表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `pipelines` (
  `id` varchar(64) NOT NULL,
  `provider` varchar(32) NOT NULL,
  `job_full_name` varchar(255) NOT NULL,
  `job_name` varchar(255) NOT NULL,
  `job_url` text NOT NULL,
  `description` text NOT NULL,
  `credential_ref` varchar(255) NOT NULL,
  `default_branch` varchar(255) NOT NULL,
  `status` varchar(32) NOT NULL,
  `last_verified_at` bigint DEFAULT NULL,
  `last_synced_at` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pipeline_provider_full_name` (`provider`,`job_full_name`),
  KEY `idx_pipeline_status_updated_at` (`status`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 48. 制品中心 - release_order_artifact_metadata (发布单制品元数据表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_artifact_metadata` (
  `id` varchar(64) NOT NULL,
  `release_order_id` varchar(64) NOT NULL,
  `execution_id` varchar(64) NOT NULL DEFAULT '',
  `pipeline_scope` varchar(20) NOT NULL DEFAULT '',
  `artifact_name` varchar(255) NOT NULL DEFAULT '',
  `artifact_type` varchar(64) NOT NULL DEFAULT '',
  `artifact_version` varchar(128) NOT NULL DEFAULT '',
  `artifact_url` text NOT NULL,
  `repository_id` varchar(64) NOT NULL DEFAULT '',
  `repository_name` varchar(200) NOT NULL DEFAULT '',
  `bucket` varchar(200) NOT NULL DEFAULT '',
  `object_key` varchar(500) NOT NULL DEFAULT '',
  `checksum` varchar(255) NOT NULL DEFAULT '',
  `checksum_type` varchar(64) NOT NULL DEFAULT '',
  `size_bytes` bigint NOT NULL DEFAULT '0',
  `build_number` varchar(128) NOT NULL DEFAULT '',
  `metadata_json` longtext NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_release_order_artifact_order` (`release_order_id`,`updated_at`),
  KEY `idx_release_order_artifact_execution` (`execution_id`),
  KEY `idx_release_order_artifact_url` (`release_order_id`,`artifact_url`(255))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 49. AI诊断 - release_order_pipeline_stage_diagnosis (发布单阶段AI诊断表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `release_order_pipeline_stage_diagnosis` (
  `id` varchar(64) NOT NULL,
  `release_order_id` varchar(64) NOT NULL,
  `stage_id` varchar(128) NOT NULL,
  `execution_id` varchar(64) NOT NULL DEFAULT '',
  `pipeline_scope` varchar(16) NOT NULL,
  `executor_type` varchar(32) NOT NULL,
  `stage_name` varchar(255) NOT NULL,
  `stage_status` varchar(32) NOT NULL,
  `ai_model_config_id` varchar(64) NOT NULL,
  `ai_model_name` varchar(120) NOT NULL,
  `ai_model` varchar(160) NOT NULL,
  `prompt_version` varchar(64) NOT NULL,
  `log_hash` varchar(64) NOT NULL,
  `log_excerpt` mediumtext,
  `status` varchar(32) NOT NULL,
  `result_json` json DEFAULT NULL,
  `error_message` text,
  `created_by` varchar(64) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `finished_at` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_stage_diagnosis_stage` (`release_order_id`,`stage_id`,`created_at`),
  KEY `idx_stage_diagnosis_cache` (`stage_id`,`log_hash`,`ai_model_config_id`,`prompt_version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- --------------------------------------------------------
-- 50. 系统模块 - system_settings (系统设置表)
-- --------------------------------------------------------
CREATE TABLE IF NOT EXISTS `system_settings` (
  `setting_key` varchar(120) NOT NULL,
  `setting_value` json NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`setting_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DROP PROCEDURE IF EXISTS gos_add_column_if_missing;
SET FOREIGN_KEY_CHECKS = 1;
