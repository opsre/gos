-- ============================================================
-- GOS 数据库增量更新脚本 v1.2
-- 基准版本: v1.1 更新后的数据库
-- 目标版本: 当前 deploy_platform.sql 表结构与内置数据
-- 说明: 补齐 v1.2 新增索引调整和内置平台字段；不包含环境配置数据、AccessKey、API Key 等密钥数据。
-- 执行方式: mysql <database> < deploy_platform-update-v1.2.sql
-- ============================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- --------------------------------------------------------
-- 1. 兼容重复执行的索引调整辅助过程
-- --------------------------------------------------------
DROP PROCEDURE IF EXISTS gos_drop_index_if_exists;
DROP PROCEDURE IF EXISTS gos_add_unique_index_if_missing;
DELIMITER $$
CREATE PROCEDURE gos_drop_index_if_exists(
    IN p_table_name VARCHAR(128),
    IN p_index_name VARCHAR(128)
)
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = p_table_name
          AND INDEX_NAME = p_index_name
    ) THEN
        SET @gos_drop_index_sql = CONCAT(
            'ALTER TABLE `', REPLACE(p_table_name, '`', '``'),
            '` DROP INDEX `', REPLACE(p_index_name, '`', '``'), '`'
        );
        PREPARE gos_drop_index_stmt FROM @gos_drop_index_sql;
        EXECUTE gos_drop_index_stmt;
        DEALLOCATE PREPARE gos_drop_index_stmt;
    END IF;
END$$
CREATE PROCEDURE gos_add_unique_index_if_missing(
    IN p_table_name VARCHAR(128),
    IN p_index_name VARCHAR(128),
    IN p_columns_sql TEXT
)
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.TABLES
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = p_table_name
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = p_table_name
          AND INDEX_NAME = p_index_name
    ) THEN
        SET @gos_add_index_sql = CONCAT(
            'ALTER TABLE `', REPLACE(p_table_name, '`', '``'),
            '` ADD UNIQUE KEY `', REPLACE(p_index_name, '`', '``'),
            '` (', p_columns_sql, ')'
        );
        PREPARE gos_add_index_stmt FROM @gos_add_index_sql;
        EXECUTE gos_add_index_stmt;
        DEALLOCATE PREPARE gos_add_index_stmt;
    END IF;
END$$
DELIMITER ;

-- --------------------------------------------------------
-- 2. 发布部署快照支持同一发布单多 ArgoCD 应用快照
-- --------------------------------------------------------
CALL gos_drop_index_if_exists('release_order_deploy_snapshot', 'uk_release_order_snapshot_order');
CALL gos_add_unique_index_if_missing(
    'release_order_deploy_snapshot',
    'uk_release_order_snapshot_target',
    '`release_order_id`, `argocd_instance_id`, `argocd_app_name`'
);

-- --------------------------------------------------------
-- 3. ArgoCD 环境绑定支持同一环境绑定多个实例
-- --------------------------------------------------------
CALL gos_drop_index_if_exists('argocd_env_binding', 'uk_argocd_env_binding_env');
CALL gos_add_unique_index_if_missing(
    'argocd_env_binding',
    'uk_argocd_env_binding_env_instance',
    '`env_code`, `argocd_instance_id`'
);

-- --------------------------------------------------------
-- 4. 平台参数补齐 GOS 制品路径内置字段
-- --------------------------------------------------------
SET @gos_platform_param_now_ns = CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6)) * 1000000000 AS SIGNED);

INSERT INTO `platform_param_dict` (
    `id`, `param_key`, `name`, `description`, `param_type`, `required`, `gitops_locator`, `cd_self_fill`, `builtin`, `status`, `created_at`, `updated_at`
) VALUES (
    'ppd-gos-artifact-path',
    'gos_artifact_path',
    'GOS_ARTIFACT_PATH',
    '应用基础信息中的制品路径；发布模板、GitOps 替换规则和 Hook 变量可直接引用，不从发布执行日志或 CI/CD 输出取值。',
    'string',
    0,
    0,
    0,
    1,
    1,
    @gos_platform_param_now_ns,
    @gos_platform_param_now_ns
) ON DUPLICATE KEY UPDATE
    `name` = VALUES(`name`),
    `description` = VALUES(`description`),
    `param_type` = VALUES(`param_type`),
    `required` = VALUES(`required`),
    `gitops_locator` = VALUES(`gitops_locator`),
    `cd_self_fill` = VALUES(`cd_self_fill`),
    `builtin` = VALUES(`builtin`),
    `status` = VALUES(`status`),
    `updated_at` = VALUES(`updated_at`);

DROP PROCEDURE IF EXISTS gos_drop_index_if_exists;
DROP PROCEDURE IF EXISTS gos_add_unique_index_if_missing;
SET FOREIGN_KEY_CHECKS = 1;
