-- Optional manual DDL. GOS applies the same schema and backfills existing
-- release events automatically at startup through migration
-- 20261107_01_prometheus_release_events; do not mark it applied here.
CREATE TABLE IF NOT EXISTS `gos_metric_release_event` (
  `event_id` VARCHAR(160) NOT NULL,
  `kind` VARCHAR(16) NOT NULL,
  `application_id` VARCHAR(64) NOT NULL,
  `env_code` VARCHAR(120) NOT NULL,
  `trigger_type` VARCHAR(50) NOT NULL,
  `operation_type` VARCHAR(32) NOT NULL,
  `pipeline_scope` VARCHAR(20) NOT NULL,
  `provider` VARCHAR(32) NOT NULL,
  `result` VARCHAR(32) NOT NULL,
  `duration_seconds` DOUBLE NULL,
  `occurred_at` BIGINT NOT NULL,
  PRIMARY KEY (`event_id`),
  KEY `idx_metric_event_scope` (`kind`,`application_id`,`env_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
