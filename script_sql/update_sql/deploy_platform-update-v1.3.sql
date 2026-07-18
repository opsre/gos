-- ============================================================
-- GOS 数据库增量更新脚本 v1.3
-- 基准版本: v1.2 更新后的数据库
-- 目标版本: 2026-07-17 deploy_platform.sql
-- 说明: 最新服务会在启动时自动执行等价迁移；本文件仅用于人工核查或应急补执行。
-- ============================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

CREATE TABLE IF NOT EXISTS `gos_schema_migration` (
    `version` VARCHAR(128) NOT NULL,
    `description` VARCHAR(500) NOT NULL DEFAULT '',
    `applied_at` BIGINT NOT NULL,
    PRIMARY KEY (`version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='GOS启动数据库迁移版本记录表';

CREATE TABLE IF NOT EXISTS `sys_user_manager` (
    `user_id` VARCHAR(64) NOT NULL,
    `manager_user_id` VARCHAR(64) NOT NULL,
    `updated_at` BIGINT NOT NULL,
    PRIMARY KEY (`user_id`),
    KEY `idx_sum_manager` (`manager_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户直属主管关系表';

CREATE TABLE IF NOT EXISTS `release_approval_flow_definition` (
    `id` VARCHAR(64) NOT NULL,
    `name` VARCHAR(128) NOT NULL,
    `status` VARCHAR(32) NOT NULL,
    `nodes_json` TEXT NOT NULL,
    `created_at` BIGINT NOT NULL,
    `updated_at` BIGINT NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布审批流定义表';

CREATE TABLE IF NOT EXISTS `release_order_approval_flow_instance` (
    `id` VARCHAR(64) NOT NULL,
    `release_order_id` VARCHAR(64) NOT NULL,
    `flow_definition_id` VARCHAR(64) NOT NULL,
    `flow_name` VARCHAR(128) NOT NULL,
    `flow_snapshot_json` TEXT NOT NULL,
    `status` VARCHAR(32) NOT NULL,
    `current_gate` VARCHAR(32) NOT NULL DEFAULT '',
    `current_scope` VARCHAR(32) NOT NULL DEFAULT '',
    `current_node_code` VARCHAR(64) NOT NULL DEFAULT '',
    `current_task_id` VARCHAR(64) NOT NULL DEFAULT '',
    `created_at` BIGINT NOT NULL,
    `updated_at` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_release_order_approval_flow_instance_order` (`release_order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布单审批流实例表';

CREATE TABLE IF NOT EXISTS `release_order_approval_flow_task` (
    `id` VARCHAR(64) NOT NULL,
    `instance_id` VARCHAR(64) NOT NULL,
    `release_order_id` VARCHAR(64) NOT NULL,
    `node_code` VARCHAR(64) NOT NULL,
    `node_name` VARCHAR(128) NOT NULL,
    `gate` VARCHAR(32) NOT NULL,
    `node_type` VARCHAR(32) NOT NULL DEFAULT 'approval',
    `approval_mode` VARCHAR(32) NOT NULL,
    `approver_ids_json` TEXT NOT NULL,
    `approver_names_json` TEXT NOT NULL,
    `agent_task_id` VARCHAR(64) NOT NULL DEFAULT '',
    `agent_task_name` VARCHAR(128) NOT NULL DEFAULT '',
    `agent_batch_id` VARCHAR(64) NOT NULL DEFAULT '',
    `message` VARCHAR(2000) NOT NULL DEFAULT '',
    `status` VARCHAR(32) NOT NULL,
    `created_at` BIGINT NOT NULL,
    `updated_at` BIGINT NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布审批流任务表';

CREATE TABLE IF NOT EXISTS `release_order_approval_flow_task_record` (
    `id` VARCHAR(64) NOT NULL,
    `task_id` VARCHAR(64) NOT NULL,
    `action` VARCHAR(32) NOT NULL,
    `operator_user_id` VARCHAR(64) NOT NULL DEFAULT '',
    `operator_name` VARCHAR(128) NOT NULL DEFAULT '',
    `comment` VARCHAR(1000) NOT NULL DEFAULT '',
    `created_at` BIGINT NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发布审批流任务操作记录表';

CREATE TABLE IF NOT EXISTS `release_application_approval_flow_binding` (
    `application_id` VARCHAR(64) NOT NULL,
    `approval_flow_id` VARCHAR(64) NOT NULL DEFAULT '',
    `updated_at` BIGINT NOT NULL,
    PRIMARY KEY (`application_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用审批流绑定表';

-- 兼容已经提前创建审批流表、但字段仍停留在旧草稿结构的数据库。
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

CALL gos_add_column_if_missing('release_order_approval_flow_instance', 'current_scope', 'VARCHAR(32) NOT NULL DEFAULT ''''', 'AFTER `current_gate`');
CALL gos_add_column_if_missing('release_order_approval_flow_instance', 'current_node_code', 'VARCHAR(64) NOT NULL DEFAULT ''''', 'AFTER `current_scope`');
CALL gos_add_column_if_missing('release_order_approval_flow_task', 'node_type', 'VARCHAR(32) NOT NULL DEFAULT ''approval''', 'AFTER `gate`');
CALL gos_add_column_if_missing('release_order_approval_flow_task', 'agent_task_id', 'VARCHAR(64) NOT NULL DEFAULT ''''', 'AFTER `approver_names_json`');
CALL gos_add_column_if_missing('release_order_approval_flow_task', 'agent_task_name', 'VARCHAR(128) NOT NULL DEFAULT ''''', 'AFTER `agent_task_id`');
CALL gos_add_column_if_missing('release_order_approval_flow_task', 'agent_batch_id', 'VARCHAR(64) NOT NULL DEFAULT ''''', 'AFTER `agent_task_name`');
CALL gos_add_column_if_missing('release_order_approval_flow_task', 'message', 'VARCHAR(2000) NOT NULL DEFAULT ''''', 'AFTER `agent_batch_id`');

DROP PROCEDURE IF EXISTS gos_add_column_if_missing;

SET @gos_migration_now_ns = CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6)) * 1000000000 AS SIGNED);
INSERT INTO `gos_schema_migration` (`version`, `description`, `applied_at`)
VALUES
  ('deploy_platform_v1_1_user_session', 'add user session revocation fields', @gos_migration_now_ns),
  ('deploy_platform_v1_1_platform_param', 'add GitOps locator and CD self-fill platform parameter fields', @gos_migration_now_ns),
  ('deploy_platform_v1_1_release_schema', 'upgrade legacy release tables, columns, indexes and data', @gos_migration_now_ns),
  ('deploy_platform_v1_1_release_schedule', 'create release scheduling tables', @gos_migration_now_ns),
  ('deploy_platform_v1_2_argocd_multi_instance', 'upgrade ArgoCD applications and environment bindings for multiple instances', @gos_migration_now_ns),
  ('20260717_01_user_manager', 'create direct user manager relationships', @gos_migration_now_ns),
  ('20260717_02_release_approval_flow', 'create release approval flow definitions, instances, tasks and bindings', @gos_migration_now_ns),
  ('20260718_01_release_approval_flow_runtime_columns', 'ensure approval flow runtime columns after early v1.3 builds', @gos_migration_now_ns)
ON DUPLICATE KEY UPDATE
  `description` = VALUES(`description`);

SET FOREIGN_KEY_CHECKS = 1;
