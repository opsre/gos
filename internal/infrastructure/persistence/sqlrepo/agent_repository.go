package sqlrepo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/agent"
)

type AgentRepository struct {
	db       *sql.DB
	dbDriver string
}

const staleClaimTimeout = time.Minute
const defaultAgentBootstrapTokenID = "default"

// NewAgentRepository 创建并返回对应组件实例。
func NewAgentRepository(db *sql.DB, dbDriver string) *AgentRepository {
	return &AgentRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *AgentRepository) InitSchema(ctx context.Context) error {
	var statements []string
	switch r.dbDriver {
	case "mysql":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS agent_instance (
	id VARCHAR(64) PRIMARY KEY,
	machine_id VARCHAR(120) NOT NULL DEFAULT '',
	agent_code VARCHAR(100) NOT NULL,
	name VARCHAR(120) NOT NULL,
	environment_code VARCHAR(120) NOT NULL DEFAULT '',
	work_dir VARCHAR(500) NOT NULL,
	token_ciphertext TEXT NOT NULL,
	tags_json TEXT NOT NULL,
	hostname VARCHAR(255) NOT NULL DEFAULT '',
	host_ip VARCHAR(120) NOT NULL DEFAULT '',
	agent_version VARCHAR(120) NOT NULL DEFAULT '',
	os VARCHAR(120) NOT NULL DEFAULT '',
	arch VARCHAR(120) NOT NULL DEFAULT '',
	status VARCHAR(20) NOT NULL DEFAULT 'active',
	last_heartbeat_at BIGINT NOT NULL DEFAULT 0,
	current_task_id VARCHAR(120) NOT NULL DEFAULT '',
	current_task_name VARCHAR(255) NOT NULL DEFAULT '',
	current_task_type VARCHAR(120) NOT NULL DEFAULT '',
	current_task_started_at BIGINT NOT NULL DEFAULT 0,
	last_task_status VARCHAR(20) NOT NULL DEFAULT 'unknown',
	last_task_summary VARCHAR(500) NOT NULL DEFAULT '',
	last_task_finished_at BIGINT NOT NULL DEFAULT 0,
	remark VARCHAR(500) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_agent_instance_code (agent_code),
	KEY idx_agent_instance_status (status),
	KEY idx_agent_instance_env (environment_code),
	KEY idx_agent_instance_heartbeat (last_heartbeat_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS agent_bootstrap_token (
	id VARCHAR(32) PRIMARY KEY,
	token_ciphertext TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS agent_task (
	id VARCHAR(64) PRIMARY KEY,
	agent_id VARCHAR(64) NOT NULL,
	agent_code VARCHAR(100) NOT NULL,
	target_agent_ids_json MEDIUMTEXT NOT NULL,
	source_task_id VARCHAR(64) NOT NULL DEFAULT '',
	dispatch_batch_id VARCHAR(64) NOT NULL DEFAULT '',
	name VARCHAR(200) NOT NULL,
	task_mode VARCHAR(20) NOT NULL DEFAULT 'temporary',
	task_type VARCHAR(50) NOT NULL,
	shell_type VARCHAR(20) NOT NULL DEFAULT 'sh',
	work_dir VARCHAR(500) NOT NULL,
	script_id VARCHAR(64) NOT NULL DEFAULT '',
	script_name VARCHAR(200) NOT NULL DEFAULT '',
	script_path VARCHAR(500) NOT NULL DEFAULT '',
	script_text MEDIUMTEXT NOT NULL,
	variables_json MEDIUMTEXT NOT NULL,
	timeout_sec INT NOT NULL DEFAULT 300,
	status VARCHAR(20) NOT NULL DEFAULT 'pending',
	claimed_at BIGINT NOT NULL DEFAULT 0,
	started_at BIGINT NOT NULL DEFAULT 0,
	finished_at BIGINT NOT NULL DEFAULT 0,
	exit_code INT NOT NULL DEFAULT 0,
	stdout_text MEDIUMTEXT NOT NULL,
	stderr_text MEDIUMTEXT NOT NULL,
	failure_reason TEXT NOT NULL,
	run_count INT NOT NULL DEFAULT 0,
	success_count INT NOT NULL DEFAULT 0,
	failure_count INT NOT NULL DEFAULT 0,
	last_run_status VARCHAR(20) NOT NULL DEFAULT '',
	last_run_summary TEXT NOT NULL,
	created_by VARCHAR(100) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	KEY idx_agent_task_agent_status (agent_id, status),
	KEY idx_agent_task_status_created (status, created_at),
	KEY idx_agent_task_agent_created (agent_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS agent_script (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(160) NOT NULL,
	description VARCHAR(500) NOT NULL DEFAULT '',
	task_type VARCHAR(50) NOT NULL,
	shell_type VARCHAR(20) NOT NULL DEFAULT 'sh',
	script_path VARCHAR(500) NOT NULL DEFAULT '',
	script_text MEDIUMTEXT NOT NULL,
	created_by VARCHAR(100) NOT NULL DEFAULT '',
	updated_by VARCHAR(100) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	KEY idx_agent_script_type_created (task_type, created_at),
	KEY idx_agent_script_name_created (name, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}
	case "sqlite":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS agent_instance (
	id TEXT PRIMARY KEY,
	machine_id TEXT NOT NULL DEFAULT '',
	agent_code TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	environment_code TEXT NOT NULL DEFAULT '',
	work_dir TEXT NOT NULL,
	token_ciphertext TEXT NOT NULL DEFAULT '',
	tags_json TEXT NOT NULL DEFAULT '[]',
	hostname TEXT NOT NULL DEFAULT '',
	host_ip TEXT NOT NULL DEFAULT '',
	agent_version TEXT NOT NULL DEFAULT '',
	os TEXT NOT NULL DEFAULT '',
	arch TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	last_heartbeat_at INTEGER NOT NULL DEFAULT 0,
	current_task_id TEXT NOT NULL DEFAULT '',
	current_task_name TEXT NOT NULL DEFAULT '',
	current_task_type TEXT NOT NULL DEFAULT '',
	current_task_started_at INTEGER NOT NULL DEFAULT 0,
	last_task_status TEXT NOT NULL DEFAULT 'unknown',
	last_task_summary TEXT NOT NULL DEFAULT '',
	last_task_finished_at INTEGER NOT NULL DEFAULT 0,
	remark TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_instance_status ON agent_instance (status);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_instance_env ON agent_instance (environment_code);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_instance_heartbeat ON agent_instance (last_heartbeat_at);`,
			`CREATE TABLE IF NOT EXISTS agent_bootstrap_token (
	id TEXT PRIMARY KEY,
	token_ciphertext TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS agent_task (
	id TEXT PRIMARY KEY,
	agent_id TEXT NOT NULL,
	agent_code TEXT NOT NULL DEFAULT '',
	target_agent_ids_json TEXT NOT NULL DEFAULT '[]',
	source_task_id TEXT NOT NULL DEFAULT '',
	dispatch_batch_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	task_mode TEXT NOT NULL DEFAULT 'temporary',
	task_type TEXT NOT NULL,
	shell_type TEXT NOT NULL DEFAULT 'sh',
	work_dir TEXT NOT NULL,
	script_id TEXT NOT NULL DEFAULT '',
	script_name TEXT NOT NULL DEFAULT '',
	script_path TEXT NOT NULL DEFAULT '',
	script_text TEXT NOT NULL,
	variables_json TEXT NOT NULL DEFAULT '{}',
	timeout_sec INTEGER NOT NULL DEFAULT 300,
	status TEXT NOT NULL DEFAULT 'pending',
	claimed_at INTEGER NOT NULL DEFAULT 0,
	started_at INTEGER NOT NULL DEFAULT 0,
	finished_at INTEGER NOT NULL DEFAULT 0,
	exit_code INTEGER NOT NULL DEFAULT 0,
	stdout_text TEXT NOT NULL DEFAULT '',
	stderr_text TEXT NOT NULL DEFAULT '',
	failure_reason TEXT NOT NULL DEFAULT '',
	run_count INTEGER NOT NULL DEFAULT 0,
	success_count INTEGER NOT NULL DEFAULT 0,
	failure_count INTEGER NOT NULL DEFAULT 0,
	last_run_status TEXT NOT NULL DEFAULT '',
	last_run_summary TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_task_agent_status ON agent_task (agent_id, status);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_task_status_created ON agent_task (status, created_at);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_task_agent_created ON agent_task (agent_id, created_at);`,
			`CREATE TABLE IF NOT EXISTS agent_script (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	task_type TEXT NOT NULL,
	shell_type TEXT NOT NULL DEFAULT 'sh',
	script_path TEXT NOT NULL DEFAULT '',
	script_text TEXT NOT NULL,
	created_by TEXT NOT NULL DEFAULT '',
	updated_by TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_script_type_created ON agent_script (task_type, created_at);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_script_name_created ON agent_script (name, created_at);`,
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}

	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return r.migrateSchema(ctx)
}

// migrateSchema 封装当前模块的业务处理逻辑。
func (r *AgentRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		instanceDefinitions := []struct {
			name string
			sql  string
		}{
			{"machine_id", `ALTER TABLE agent_instance ADD COLUMN machine_id VARCHAR(120) NOT NULL DEFAULT '' AFTER id;`},
		}
		taskDefinitions := []struct {
			name string
			sql  string
		}{
			{"target_agent_ids_json", `ALTER TABLE agent_task ADD COLUMN target_agent_ids_json MEDIUMTEXT NOT NULL AFTER agent_code;`},
			{"source_task_id", `ALTER TABLE agent_task ADD COLUMN source_task_id VARCHAR(64) NOT NULL DEFAULT '' AFTER target_agent_ids_json;`},
			{"dispatch_batch_id", `ALTER TABLE agent_task ADD COLUMN dispatch_batch_id VARCHAR(64) NOT NULL DEFAULT '' AFTER source_task_id;`},
			{"script_path", `ALTER TABLE agent_task ADD COLUMN script_path VARCHAR(500) NOT NULL DEFAULT '' AFTER work_dir;`},
			{"task_mode", `ALTER TABLE agent_task ADD COLUMN task_mode VARCHAR(20) NOT NULL DEFAULT 'temporary' AFTER name;`},
			{"script_id", `ALTER TABLE agent_task ADD COLUMN script_id VARCHAR(64) NOT NULL DEFAULT '' AFTER work_dir;`},
			{"script_name", `ALTER TABLE agent_task ADD COLUMN script_name VARCHAR(200) NOT NULL DEFAULT '' AFTER script_id;`},
			{"run_count", `ALTER TABLE agent_task ADD COLUMN run_count INT NOT NULL DEFAULT 0 AFTER failure_reason;`},
			{"success_count", `ALTER TABLE agent_task ADD COLUMN success_count INT NOT NULL DEFAULT 0 AFTER run_count;`},
			{"failure_count", `ALTER TABLE agent_task ADD COLUMN failure_count INT NOT NULL DEFAULT 0 AFTER success_count;`},
			{"last_run_status", `ALTER TABLE agent_task ADD COLUMN last_run_status VARCHAR(20) NOT NULL DEFAULT '' AFTER failure_count;`},
			{"last_run_summary", `ALTER TABLE agent_task ADD COLUMN last_run_summary TEXT NOT NULL AFTER last_run_status;`},
		}
		for _, definition := range instanceDefinitions {
			exists, err := r.mysqlColumnExists(ctx, "agent_instance", definition.name)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			if _, err = r.db.ExecContext(ctx, definition.sql); err != nil {
				return err
			}
		}
		for _, definition := range taskDefinitions {
			exists, err := r.mysqlColumnExists(ctx, "agent_task", definition.name)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			if _, err = r.db.ExecContext(ctx, definition.sql); err != nil {
				return err
			}
		}
		if _, err := r.db.ExecContext(ctx, `CREATE INDEX idx_agent_instance_machine ON agent_instance (machine_id);`); err != nil && !isAlreadyExistsIndexError(err) {
			return err
		}
		if _, err := r.db.ExecContext(ctx, `CREATE INDEX idx_agent_task_agent_mode_status ON agent_task (agent_id, task_mode, status);`); err != nil && !isAlreadyExistsIndexError(err) {
			return err
		}
		return nil
	case "sqlite":
		instanceColumns, err := r.sqliteTableColumns(ctx, "agent_instance")
		if err != nil {
			return err
		}
		taskColumns, err := r.sqliteTableColumns(ctx, "agent_task")
		if err != nil {
			return err
		}
		instanceDefinitions := []struct {
			name string
			sql  string
		}{
			{"machine_id", `ALTER TABLE agent_instance ADD COLUMN machine_id TEXT NOT NULL DEFAULT '';`},
		}
		taskDefinitions := []struct {
			name string
			sql  string
		}{
			{"target_agent_ids_json", `ALTER TABLE agent_task ADD COLUMN target_agent_ids_json TEXT NOT NULL DEFAULT '[]';`},
			{"source_task_id", `ALTER TABLE agent_task ADD COLUMN source_task_id TEXT NOT NULL DEFAULT '';`},
			{"dispatch_batch_id", `ALTER TABLE agent_task ADD COLUMN dispatch_batch_id TEXT NOT NULL DEFAULT '';`},
			{"script_path", `ALTER TABLE agent_task ADD COLUMN script_path TEXT NOT NULL DEFAULT '';`},
			{"task_mode", `ALTER TABLE agent_task ADD COLUMN task_mode TEXT NOT NULL DEFAULT 'temporary';`},
			{"script_id", `ALTER TABLE agent_task ADD COLUMN script_id TEXT NOT NULL DEFAULT '';`},
			{"script_name", `ALTER TABLE agent_task ADD COLUMN script_name TEXT NOT NULL DEFAULT '';`},
			{"run_count", `ALTER TABLE agent_task ADD COLUMN run_count INTEGER NOT NULL DEFAULT 0;`},
			{"success_count", `ALTER TABLE agent_task ADD COLUMN success_count INTEGER NOT NULL DEFAULT 0;`},
			{"failure_count", `ALTER TABLE agent_task ADD COLUMN failure_count INTEGER NOT NULL DEFAULT 0;`},
			{"last_run_status", `ALTER TABLE agent_task ADD COLUMN last_run_status TEXT NOT NULL DEFAULT '';`},
			{"last_run_summary", `ALTER TABLE agent_task ADD COLUMN last_run_summary TEXT NOT NULL DEFAULT '';`},
		}
		for _, definition := range instanceDefinitions {
			if _, ok := instanceColumns[definition.name]; ok {
				continue
			}
			if _, err = r.db.ExecContext(ctx, definition.sql); err != nil {
				return err
			}
		}
		for _, definition := range taskDefinitions {
			if _, ok := taskColumns[definition.name]; ok {
				continue
			}
			if _, err = r.db.ExecContext(ctx, definition.sql); err != nil {
				return err
			}
		}
		if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_agent_instance_machine ON agent_instance (machine_id);`); err != nil {
			return err
		}
		if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_agent_task_agent_mode_status ON agent_task (agent_id, task_mode, status);`); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// CreateInstance 创建业务资源并返回处理结果。
func (r *AgentRepository) CreateInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	encryptedToken, err := encryptStoredSecret(strings.TrimSpace(item.Token))
	if err != nil {
		return domain.Instance{}, err
	}
	const q = `
INSERT INTO agent_instance (
	id, machine_id, agent_code, name, environment_code, work_dir, token_ciphertext, tags_json,
	hostname, host_ip, agent_version, os, arch, status, last_heartbeat_at,
	current_task_id, current_task_name, current_task_type, current_task_started_at,
	last_task_status, last_task_summary, last_task_finished_at, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err = r.db.ExecContext(ctx, q,
		item.ID,
		strings.TrimSpace(item.MachineID),
		strings.TrimSpace(item.AgentCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.EnvironmentCode),
		strings.TrimSpace(item.WorkDir),
		encryptedToken,
		marshalStringSlice(item.Tags),
		strings.TrimSpace(item.Hostname),
		strings.TrimSpace(item.HostIP),
		strings.TrimSpace(item.AgentVersion),
		strings.TrimSpace(item.OS),
		strings.TrimSpace(item.Arch),
		string(item.Status),
		timeToUnixNano(item.LastHeartbeatAt),
		strings.TrimSpace(item.CurrentTaskID),
		strings.TrimSpace(item.CurrentTaskName),
		strings.TrimSpace(item.CurrentTaskType),
		ptrTimeToUnixNano(item.CurrentTaskStarted),
		string(item.LastTaskStatus),
		strings.TrimSpace(item.LastTaskSummary),
		ptrTimeToUnixNano(item.LastTaskFinishedAt),
		strings.TrimSpace(item.Remark),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateAgentCodeError(r.dbDriver, err) {
			return domain.Instance{}, domain.ErrAgentCodeDuplicated
		}
		return domain.Instance{}, err
	}
	return r.GetInstanceByID(ctx, item.ID)
}

// UpdateInstance 更新业务资源并返回处理结果。
func (r *AgentRepository) UpdateInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	encryptedToken, err := encryptStoredSecret(strings.TrimSpace(item.Token))
	if err != nil {
		return domain.Instance{}, err
	}
	const q = `
UPDATE agent_instance
SET machine_id = ?, agent_code = ?, name = ?, environment_code = ?, work_dir = ?, token_ciphertext = ?, tags_json = ?,
	hostname = ?, host_ip = ?, agent_version = ?, os = ?, arch = ?, status = ?, last_heartbeat_at = ?,
	current_task_id = ?, current_task_name = ?, current_task_type = ?, current_task_started_at = ?,
	last_task_status = ?, last_task_summary = ?, last_task_finished_at = ?, remark = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(item.MachineID),
		strings.TrimSpace(item.AgentCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.EnvironmentCode),
		strings.TrimSpace(item.WorkDir),
		encryptedToken,
		marshalStringSlice(item.Tags),
		strings.TrimSpace(item.Hostname),
		strings.TrimSpace(item.HostIP),
		strings.TrimSpace(item.AgentVersion),
		strings.TrimSpace(item.OS),
		strings.TrimSpace(item.Arch),
		string(item.Status),
		timeToUnixNano(item.LastHeartbeatAt),
		strings.TrimSpace(item.CurrentTaskID),
		strings.TrimSpace(item.CurrentTaskName),
		strings.TrimSpace(item.CurrentTaskType),
		ptrTimeToUnixNano(item.CurrentTaskStarted),
		string(item.LastTaskStatus),
		strings.TrimSpace(item.LastTaskSummary),
		ptrTimeToUnixNano(item.LastTaskFinishedAt),
		strings.TrimSpace(item.Remark),
		item.UpdatedAt.UTC().UnixNano(),
		strings.TrimSpace(item.ID),
	)
	if err != nil {
		if isDuplicateAgentCodeError(r.dbDriver, err) {
			return domain.Instance{}, domain.ErrAgentCodeDuplicated
		}
		return domain.Instance{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Instance{}, err
	}
	if affected == 0 {
		return domain.Instance{}, domain.ErrInstanceNotFound
	}
	return r.GetInstanceByID(ctx, item.ID)
}

// GetInstanceByID 查询并返回指定资源数据。
func (r *AgentRepository) GetInstanceByID(ctx context.Context, id string) (domain.Instance, error) {
	const q = `
SELECT id, machine_id, agent_code, name, environment_code, work_dir, token_ciphertext, tags_json,
	hostname, host_ip, agent_version, os, arch, status, last_heartbeat_at,
	current_task_id, current_task_name, current_task_type, current_task_started_at,
	last_task_status, last_task_summary, last_task_finished_at, remark, created_at, updated_at
FROM agent_instance WHERE id = ?;`
	item, err := scanAgentInstance(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instance{}, domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}
	return item, nil
}

// GetInstanceByCode 查询并返回指定资源数据。
func (r *AgentRepository) GetInstanceByCode(ctx context.Context, code string) (domain.Instance, error) {
	const q = `
SELECT id, machine_id, agent_code, name, environment_code, work_dir, token_ciphertext, tags_json,
	hostname, host_ip, agent_version, os, arch, status, last_heartbeat_at,
	current_task_id, current_task_name, current_task_type, current_task_started_at,
	last_task_status, last_task_summary, last_task_finished_at, remark, created_at, updated_at
FROM agent_instance WHERE agent_code = ?;`
	item, err := scanAgentInstance(r.db.QueryRowContext(ctx, q, strings.TrimSpace(code)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instance{}, domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}
	return item, nil
}

// GetInstanceByMachineID 查询并返回指定资源数据。
func (r *AgentRepository) GetInstanceByMachineID(ctx context.Context, machineID string) (domain.Instance, error) {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return domain.Instance{}, domain.ErrInstanceNotFound
	}
	const q = `
SELECT id, machine_id, agent_code, name, environment_code, work_dir, token_ciphertext, tags_json,
	hostname, host_ip, agent_version, os, arch, status, last_heartbeat_at,
	current_task_id, current_task_name, current_task_type, current_task_started_at,
	last_task_status, last_task_summary, last_task_finished_at, remark, created_at, updated_at
FROM agent_instance
WHERE machine_id = ?
ORDER BY updated_at DESC, created_at DESC
LIMIT 1;`
	item, err := scanAgentInstance(r.db.QueryRowContext(ctx, q, machineID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instance{}, domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}
	return item, nil
}

// ListInstances 查询并返回列表数据。
func (r *AgentRepository) ListInstances(ctx context.Context, filter domain.ListFilter) ([]domain.Instance, int64, error) {
	args := make([]any, 0, 8)
	where := make([]string, 0, 2)
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		where = append(where, "(agent_code LIKE ? OR name LIKE ? OR hostname LIKE ? OR host_ip LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}
	countQ := "SELECT COUNT(1) FROM agent_instance"
	queryQ := `SELECT id, machine_id, agent_code, name, environment_code, work_dir, token_ciphertext, tags_json,
	hostname, host_ip, agent_version, os, arch, status, last_heartbeat_at,
	current_task_id, current_task_name, current_task_type, current_task_started_at,
	last_task_status, last_task_summary, last_task_finished_at, remark, created_at, updated_at
FROM agent_instance`
	if len(where) > 0 {
		clause := " WHERE " + strings.Join(where, " AND ")
		countQ += clause
		queryQ += clause
	}
	queryQ += " ORDER BY updated_at DESC, created_at DESC"
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	queryQ += " LIMIT ? OFFSET ?"
	queryArgs := append(append([]any(nil), args...), filter.PageSize, (filter.Page-1)*filter.PageSize)

	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, queryQ, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]domain.Instance, 0)
	for rows.Next() {
		item, scanErr := scanAgentInstance(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// DeleteInstance 删除业务资源并返回处理结果。
func (r *AgentRepository) DeleteInstance(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM agent_instance WHERE id = ?;`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrInstanceNotFound
	}
	return nil
}

// UpdateHeartbeat 更新业务资源并返回处理结果。
func (r *AgentRepository) UpdateHeartbeat(ctx context.Context, instanceID string, payload domain.HeartbeatPayload) (domain.Instance, error) {
	const q = `
UPDATE agent_instance
SET hostname = ?, host_ip = ?, agent_version = ?, os = ?, arch = ?, work_dir = ?, tags_json = ?,
	last_heartbeat_at = ?, current_task_id = ?, current_task_name = ?, current_task_type = ?, current_task_started_at = ?,
	last_task_status = ?, last_task_summary = ?, last_task_finished_at = ?, updated_at = ?
WHERE id = ?;`
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(payload.Hostname),
		strings.TrimSpace(payload.HostIP),
		strings.TrimSpace(payload.AgentVersion),
		strings.TrimSpace(payload.OS),
		strings.TrimSpace(payload.Arch),
		strings.TrimSpace(payload.WorkDir),
		marshalStringSlice(payload.Tags),
		now.UnixNano(),
		strings.TrimSpace(payload.CurrentTaskID),
		strings.TrimSpace(payload.CurrentTaskName),
		strings.TrimSpace(payload.CurrentTaskType),
		ptrTimeToUnixNano(payload.CurrentTaskStarted),
		string(payload.LastTaskStatus),
		strings.TrimSpace(payload.LastTaskSummary),
		ptrTimeToUnixNano(payload.LastTaskFinishedAt),
		now.UnixNano(),
		strings.TrimSpace(instanceID),
	)
	if err != nil {
		return domain.Instance{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Instance{}, err
	}
	if affected == 0 {
		return domain.Instance{}, domain.ErrInstanceNotFound
	}
	return r.GetInstanceByID(ctx, instanceID)
}

// UpdateRuntimeTask 更新业务资源并返回处理结果。
func (r *AgentRepository) UpdateRuntimeTask(ctx context.Context, instanceID string, payload domain.RuntimeTaskPayload) (domain.Instance, error) {
	const q = `
UPDATE agent_instance
SET current_task_id = ?, current_task_name = ?, current_task_type = ?, current_task_started_at = ?,
	last_task_status = ?, last_task_summary = ?, last_task_finished_at = ?, updated_at = ?
WHERE id = ?;`
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(payload.CurrentTaskID),
		strings.TrimSpace(payload.CurrentTaskName),
		strings.TrimSpace(payload.CurrentTaskType),
		ptrTimeToUnixNano(payload.CurrentTaskStarted),
		string(payload.LastTaskStatus),
		strings.TrimSpace(payload.LastTaskSummary),
		ptrTimeToUnixNano(payload.LastTaskFinishedAt),
		now.UnixNano(),
		strings.TrimSpace(instanceID),
	)
	if err != nil {
		return domain.Instance{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Instance{}, err
	}
	if affected == 0 {
		return domain.Instance{}, domain.ErrInstanceNotFound
	}
	return r.GetInstanceByID(ctx, instanceID)
}

// GetBootstrapToken 查询并返回指定资源数据。
func (r *AgentRepository) GetBootstrapToken(ctx context.Context) (string, error) {
	const q = `SELECT token_ciphertext FROM agent_bootstrap_token WHERE id = ?;`
	var encryptedToken string
	err := r.db.QueryRowContext(ctx, q, defaultAgentBootstrapTokenID).Scan(&encryptedToken)
	if err == nil {
		return decryptStoredSecret(encryptedToken)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return r.ResetBootstrapToken(ctx)
}

// ResetBootstrapToken 封装当前模块的业务处理逻辑。
func (r *AgentRepository) ResetBootstrapToken(ctx context.Context) (string, error) {
	token := generateBootstrapToken()
	encryptedToken, err := encryptStoredSecret(token)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().UnixNano()
	switch r.dbDriver {
	case "mysql":
		const q = `
INSERT INTO agent_bootstrap_token (id, token_ciphertext, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE token_ciphertext = VALUES(token_ciphertext), updated_at = VALUES(updated_at);`
		if _, err := r.db.ExecContext(ctx, q, defaultAgentBootstrapTokenID, encryptedToken, now, now); err != nil {
			return "", err
		}
	case "sqlite":
		const q = `
INSERT INTO agent_bootstrap_token (id, token_ciphertext, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET token_ciphertext = excluded.token_ciphertext, updated_at = excluded.updated_at;`
		if _, err := r.db.ExecContext(ctx, q, defaultAgentBootstrapTokenID, encryptedToken, now, now); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	return token, nil
}

// CreateScript 创建业务资源并返回处理结果。
func (r *AgentRepository) CreateScript(ctx context.Context, item domain.Script) (domain.Script, error) {
	const q = `
INSERT INTO agent_script (
	id, name, description, task_type, shell_type, script_path, script_text, created_by, updated_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.Description),
		strings.TrimSpace(item.TaskType),
		strings.TrimSpace(item.ShellType),
		strings.TrimSpace(item.ScriptPath),
		item.ScriptText,
		strings.TrimSpace(item.CreatedBy),
		strings.TrimSpace(item.UpdatedBy),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return domain.Script{}, err
	}
	return r.GetScriptByID(ctx, item.ID)
}

// UpdateScript 更新业务资源并返回处理结果。
func (r *AgentRepository) UpdateScript(ctx context.Context, item domain.Script) (domain.Script, error) {
	const q = `
UPDATE agent_script
SET name = ?, description = ?, task_type = ?, shell_type = ?, script_path = ?, script_text = ?, updated_by = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.Description),
		strings.TrimSpace(item.TaskType),
		strings.TrimSpace(item.ShellType),
		strings.TrimSpace(item.ScriptPath),
		item.ScriptText,
		strings.TrimSpace(item.UpdatedBy),
		item.UpdatedAt.UTC().UnixNano(),
		strings.TrimSpace(item.ID),
	)
	if err != nil {
		return domain.Script{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Script{}, err
	}
	if affected == 0 {
		return domain.Script{}, domain.ErrScriptNotFound
	}
	return r.GetScriptByID(ctx, item.ID)
}

// GetScriptByID 查询并返回指定资源数据。
func (r *AgentRepository) GetScriptByID(ctx context.Context, id string) (domain.Script, error) {
	const q = `
SELECT id, name, description, task_type, shell_type, script_path, script_text, created_by, updated_by, created_at, updated_at
FROM agent_script WHERE id = ?;`
	item, err := scanAgentScript(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Script{}, domain.ErrScriptNotFound
		}
		return domain.Script{}, err
	}
	return item, nil
}

// ListScripts 查询并返回列表数据。
func (r *AgentRepository) ListScripts(ctx context.Context, filter domain.ScriptListFilter) ([]domain.Script, int64, error) {
	args := make([]any, 0, 6)
	where := make([]string, 0, 2)
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		where = append(where, "(name LIKE ? OR description LIKE ? OR script_path LIKE ?)")
		args = append(args, like, like, like)
	}
	if filter.TaskType != "" {
		where = append(where, "task_type = ?")
		args = append(args, string(filter.TaskType))
	}
	countQ := "SELECT COUNT(1) FROM agent_script"
	queryQ := `SELECT id, name, description, task_type, shell_type, script_path, script_text, created_by, updated_by, created_at, updated_at FROM agent_script`
	if len(where) > 0 {
		clause := " WHERE " + strings.Join(where, " AND ")
		countQ += clause
		queryQ += clause
	}
	queryQ += " ORDER BY updated_at DESC, created_at DESC"
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	queryQ += " LIMIT ? OFFSET ?"
	queryArgs := append(append([]any(nil), args...), filter.PageSize, (filter.Page-1)*filter.PageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, queryQ, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]domain.Script, 0)
	for rows.Next() {
		item, scanErr := scanAgentScript(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// DeleteScript 删除业务资源并返回处理结果。
func (r *AgentRepository) DeleteScript(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM agent_script WHERE id = ?;`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrScriptNotFound
	}
	return nil
}

const agentTaskSelectColumns = `
id, agent_id, agent_code, target_agent_ids_json, source_task_id, dispatch_batch_id, name, task_mode, task_type, shell_type, work_dir, script_id, script_name, script_path, script_text, variables_json,
timeout_sec, status, claimed_at, started_at, finished_at, exit_code, stdout_text, stderr_text, failure_reason,
run_count, success_count, failure_count, last_run_status, last_run_summary,
created_by, created_at, updated_at`

// CreateTask 创建业务资源并返回处理结果。
func (r *AgentRepository) CreateTask(ctx context.Context, item domain.Task) (domain.Task, error) {
	const q = `
INSERT INTO agent_task (
	id, agent_id, agent_code, target_agent_ids_json, source_task_id, dispatch_batch_id, name, task_mode, task_type, shell_type, work_dir, script_id, script_name, script_path, script_text, variables_json,
	timeout_sec, status, claimed_at, started_at, finished_at, exit_code, stdout_text, stderr_text, failure_reason,
	run_count, success_count, failure_count, last_run_status, last_run_summary,
	created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		strings.TrimSpace(item.AgentID),
		strings.TrimSpace(item.AgentCode),
		marshalStringSlice(item.TargetAgentIDs),
		strings.TrimSpace(item.SourceTaskID),
		strings.TrimSpace(item.DispatchBatchID),
		strings.TrimSpace(item.Name),
		string(item.TaskMode),
		strings.TrimSpace(item.TaskType),
		strings.TrimSpace(item.ShellType),
		strings.TrimSpace(item.WorkDir),
		strings.TrimSpace(item.ScriptID),
		strings.TrimSpace(item.ScriptName),
		strings.TrimSpace(item.ScriptPath),
		item.ScriptText,
		marshalStringMap(item.Variables),
		item.TimeoutSec,
		string(item.Status),
		ptrTimeToUnixNano(item.ClaimedAt),
		ptrTimeToUnixNano(item.StartedAt),
		ptrTimeToUnixNano(item.FinishedAt),
		item.ExitCode,
		item.StdoutText,
		item.StderrText,
		item.FailureReason,
		item.RunCount,
		item.SuccessCount,
		item.FailureCount,
		string(item.LastRunStatus),
		item.LastRunSummary,
		strings.TrimSpace(item.CreatedBy),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return domain.Task{}, err
	}
	return r.GetTaskByID(ctx, item.ID)
}

// UpdateTask 更新业务资源并返回处理结果。
func (r *AgentRepository) UpdateTask(ctx context.Context, item domain.Task) (domain.Task, error) {
	const q = `
UPDATE agent_task
SET agent_id = ?, agent_code = ?, target_agent_ids_json = ?, source_task_id = ?, dispatch_batch_id = ?,
	name = ?, task_mode = ?, task_type = ?, shell_type = ?, work_dir = ?, script_id = ?, script_name = ?, script_path = ?, script_text = ?, variables_json = ?,
	timeout_sec = ?, status = ?, claimed_at = ?, started_at = ?, finished_at = ?, exit_code = ?, stdout_text = ?, stderr_text = ?, failure_reason = ?,
	run_count = ?, success_count = ?, failure_count = ?, last_run_status = ?, last_run_summary = ?, created_by = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(item.AgentID),
		strings.TrimSpace(item.AgentCode),
		marshalStringSlice(item.TargetAgentIDs),
		strings.TrimSpace(item.SourceTaskID),
		strings.TrimSpace(item.DispatchBatchID),
		strings.TrimSpace(item.Name),
		string(item.TaskMode),
		strings.TrimSpace(item.TaskType),
		strings.TrimSpace(item.ShellType),
		strings.TrimSpace(item.WorkDir),
		strings.TrimSpace(item.ScriptID),
		strings.TrimSpace(item.ScriptName),
		strings.TrimSpace(item.ScriptPath),
		item.ScriptText,
		marshalStringMap(item.Variables),
		item.TimeoutSec,
		string(item.Status),
		ptrTimeToUnixNano(item.ClaimedAt),
		ptrTimeToUnixNano(item.StartedAt),
		ptrTimeToUnixNano(item.FinishedAt),
		item.ExitCode,
		item.StdoutText,
		item.StderrText,
		item.FailureReason,
		item.RunCount,
		item.SuccessCount,
		item.FailureCount,
		string(item.LastRunStatus),
		item.LastRunSummary,
		strings.TrimSpace(item.CreatedBy),
		item.UpdatedAt.UTC().UnixNano(),
		strings.TrimSpace(item.ID),
	)
	if err != nil {
		return domain.Task{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if affected == 0 {
		return domain.Task{}, domain.ErrTaskNotFound
	}
	return r.GetTaskByID(ctx, item.ID)
}

// GetTaskByID 查询并返回指定资源数据。
func (r *AgentRepository) GetTaskByID(ctx context.Context, id string) (domain.Task, error) {
	q := `SELECT ` + agentTaskSelectColumns + ` FROM agent_task WHERE id = ?;`
	item, err := scanAgentTask(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, domain.ErrTaskNotFound
		}
		return domain.Task{}, err
	}
	return item, nil
}

// ListTasks 查询并返回列表数据。
func (r *AgentRepository) ListTasks(ctx context.Context, filter domain.TaskListFilter) ([]domain.Task, int64, error) {
	args := make([]any, 0, 8)
	where := make([]string, 0, 5)
	if agentID := strings.TrimSpace(filter.AgentID); agentID != "" {
		where = append(where, "agent_id = ?")
		args = append(args, agentID)
	}
	if scriptID := strings.TrimSpace(filter.ScriptID); scriptID != "" {
		where = append(where, "script_id = ?")
		args = append(args, scriptID)
	}
	if sourceTaskID := strings.TrimSpace(filter.SourceTaskID); sourceTaskID != "" {
		where = append(where, "source_task_id = ?")
		args = append(args, sourceTaskID)
	}
	if dispatchBatchID := strings.TrimSpace(filter.DispatchBatchID); dispatchBatchID != "" {
		where = append(where, "dispatch_batch_id = ?")
		args = append(args, dispatchBatchID)
	}
	if len(filter.Statuses) > 0 {
		holders := make([]string, 0, len(filter.Statuses))
		for _, item := range filter.Statuses {
			if !item.Valid() {
				continue
			}
			holders = append(holders, "?")
			args = append(args, string(item))
		}
		if len(holders) > 0 {
			where = append(where, "status IN ("+strings.Join(holders, ",")+")")
		}
	}
	countQ := "SELECT COUNT(1) FROM agent_task"
	queryQ := `SELECT ` + agentTaskSelectColumns + ` FROM agent_task`
	if len(where) > 0 {
		clause := " WHERE " + strings.Join(where, " AND ")
		countQ += clause
		queryQ += clause
	}
	queryQ += " ORDER BY created_at DESC"
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	queryQ += " LIMIT ? OFFSET ?"
	queryArgs := append(append([]any(nil), args...), filter.PageSize, (filter.Page-1)*filter.PageSize)

	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, queryQ, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]domain.Task, 0)
	for rows.Next() {
		item, scanErr := scanAgentTask(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// DeleteTask 删除业务资源并返回处理结果。
func (r *AgentRepository) DeleteTask(ctx context.Context, taskID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM agent_task WHERE id = ?;`, strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// ClaimNextPendingTask 封装当前模块的业务处理逻辑。
func (r *AgentRepository) ClaimNextPendingTask(ctx context.Context, agentID string, now time.Time) (domain.Task, bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.Task{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	const resetStaleClaimedQ = `
	UPDATE agent_task
	SET status = ?, claimed_at = ?, updated_at = ?, last_run_summary = ?
	WHERE agent_id = ? AND status = ? AND claimed_at > 0 AND claimed_at < ?;`
	if _, err := tx.ExecContext(
		ctx,
		resetStaleClaimedQ,
		string(domain.TaskStatusQueued),
		0,
		now.UTC().UnixNano(),
		"领取超时，已重新排队",
		strings.TrimSpace(agentID),
		string(domain.TaskStatusClaimed),
		now.Add(-staleClaimTimeout).UTC().UnixNano(),
	); err != nil {
		return domain.Task{}, false, err
	}
	const activeQ = `
	SELECT COUNT(1)
	FROM agent_task
	WHERE agent_id = ? AND status IN (?, ?);`
	var activeCount int
	if err := tx.QueryRowContext(ctx, activeQ, strings.TrimSpace(agentID), string(domain.TaskStatusClaimed), string(domain.TaskStatusRunning)).Scan(&activeCount); err != nil {
		return domain.Task{}, false, err
	}
	if activeCount > 0 {
		return domain.Task{}, false, nil
	}
	const selectQ = `
SELECT ` + agentTaskSelectColumns + `
FROM agent_task
WHERE agent_id = ? AND status IN (?, ?)
ORDER BY CASE WHEN task_mode = 'temporary' THEN 0 ELSE 1 END ASC,
         CASE WHEN status = 'pending' THEN 0 ELSE 1 END ASC,
         updated_at ASC, created_at ASC
LIMIT 1;`
	item, err := scanAgentTask(tx.QueryRowContext(ctx, selectQ, strings.TrimSpace(agentID), string(domain.TaskStatusPending), string(domain.TaskStatusQueued)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, false, nil
		}
		return domain.Task{}, false, err
	}
	const updateQ = `
UPDATE agent_task
SET status = ?, claimed_at = ?, updated_at = ?
WHERE id = ? AND status IN (?, ?);`
	res, err := tx.ExecContext(ctx, updateQ, string(domain.TaskStatusClaimed), now.UTC().UnixNano(), now.UTC().UnixNano(), item.ID, string(domain.TaskStatusPending), string(domain.TaskStatusQueued))
	if err != nil {
		return domain.Task{}, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, false, err
	}
	if affected == 0 {
		return domain.Task{}, false, nil
	}
	if err := tx.Commit(); err != nil {
		return domain.Task{}, false, err
	}
	task, err := r.GetTaskByID(ctx, item.ID)
	if err != nil {
		return domain.Task{}, false, err
	}
	return task, true, nil
}

// MarkTaskRunning 封装当前模块的业务处理逻辑。
func (r *AgentRepository) MarkTaskRunning(ctx context.Context, taskID string, startedAt time.Time) (domain.Task, error) {
	const q = `
	UPDATE agent_task
	SET status = ?, started_at = ?, updated_at = ?
	WHERE id = ? AND status = ?;`
	res, err := r.db.ExecContext(ctx, q,
		string(domain.TaskStatusRunning),
		startedAt.UTC().UnixNano(),
		startedAt.UTC().UnixNano(),
		strings.TrimSpace(taskID),
		string(domain.TaskStatusClaimed),
	)
	if err != nil {
		return domain.Task{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if affected == 0 {
		return domain.Task{}, domain.ErrTaskNotClaimable
	}
	return r.GetTaskByID(ctx, taskID)
}

// ActivateTemporaryTask 封装当前模块的业务处理逻辑。
func (r *AgentRepository) ActivateTemporaryTask(ctx context.Context, taskID string, nextStatus domain.TaskStatus, activatedAt time.Time) (domain.Task, error) {
	if nextStatus != domain.TaskStatusPending && nextStatus != domain.TaskStatusQueued {
		nextStatus = domain.TaskStatusPending
	}
	current, err := r.GetTaskByID(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if current.TaskMode != domain.TaskModeTemporary {
		return domain.Task{}, domain.ErrTaskNotClaimable
	}
	switch current.Status {
	case domain.TaskStatusPending, domain.TaskStatusQueued, domain.TaskStatusClaimed, domain.TaskStatusRunning:
		return domain.Task{}, domain.ErrTaskNotClaimable
	}
	const q = `
UPDATE agent_task
SET status = ?, claimed_at = 0, started_at = 0, finished_at = 0, exit_code = 0,
	stdout_text = '', stderr_text = '', failure_reason = '', updated_at = ?
WHERE id = ? AND task_mode = ? AND status IN (?, ?, ?, ?);`
	res, err := r.db.ExecContext(ctx, q,
		string(nextStatus),
		activatedAt.UTC().UnixNano(),
		strings.TrimSpace(taskID),
		string(domain.TaskModeTemporary),
		string(domain.TaskStatusDraft),
		string(domain.TaskStatusSuccess),
		string(domain.TaskStatusFailed),
		string(domain.TaskStatusCancelled),
	)
	if err != nil {
		return domain.Task{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if affected == 0 {
		return domain.Task{}, domain.ErrTaskNotClaimable
	}
	return r.GetTaskByID(ctx, taskID)
}

// CancelTask 封装当前模块的业务处理逻辑。
func (r *AgentRepository) CancelTask(ctx context.Context, taskID string, cancelledAt time.Time, reason string) (domain.Task, error) {
	const q = `
UPDATE agent_task
SET status = ?, updated_at = ?, failure_reason = ?, last_run_summary = ?
WHERE id = ? AND status IN (?, ?, ?, ?, ?);`
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "已手动停止常驻任务"
	}
	res, err := r.db.ExecContext(ctx, q,
		string(domain.TaskStatusCancelled),
		cancelledAt.UTC().UnixNano(),
		reason,
		reason,
		strings.TrimSpace(taskID),
		string(domain.TaskStatusPending),
		string(domain.TaskStatusQueued),
		string(domain.TaskStatusClaimed),
		string(domain.TaskStatusRunning),
		string(domain.TaskStatusCancelled),
	)
	if err != nil {
		return domain.Task{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if affected == 0 {
		return domain.Task{}, domain.ErrTaskNotFound
	}
	return r.GetTaskByID(ctx, taskID)
}

// ResumeTask 封装当前模块的业务处理逻辑。
func (r *AgentRepository) ResumeTask(ctx context.Context, taskID string, nextStatus domain.TaskStatus, resumedAt time.Time, summary string) (domain.Task, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "已重新启用常驻任务"
	}
	if !nextStatus.Valid() {
		nextStatus = domain.TaskStatusPending
	}
	const q = `
UPDATE agent_task
SET status = ?, updated_at = ?, failure_reason = ?, last_run_summary = ?
WHERE id = ? AND status = ?;`
	res, err := r.db.ExecContext(ctx, q,
		string(nextStatus),
		resumedAt.UTC().UnixNano(),
		"",
		summary,
		strings.TrimSpace(taskID),
		string(domain.TaskStatusCancelled),
	)
	if err != nil {
		return domain.Task{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if affected == 0 {
		return domain.Task{}, domain.ErrTaskNotFound
	}
	return r.GetTaskByID(ctx, taskID)
}

// FinishTask 封装当前模块的业务处理逻辑。
func (r *AgentRepository) FinishTask(ctx context.Context, taskID string, status domain.TaskStatus, exitCode int, stdoutText, stderrText, failureReason string, finishedAt time.Time) (domain.Task, error) {
	if !status.Valid() {
		return domain.Task{}, fmt.Errorf("invalid task status: %s", status)
	}
	current, err := r.GetTaskByID(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	nextStatus := status
	if current.TaskMode == domain.TaskModeResident && current.Status == domain.TaskStatusCancelled {
		nextStatus = domain.TaskStatusCancelled
	} else if current.TaskMode == domain.TaskModeResident && status != domain.TaskStatusCancelled {
		const queueQ = `
SELECT COUNT(1)
FROM agent_task
WHERE agent_id = ? AND id <> ? AND status IN (?, ?, ?, ?);`
		var queueCount int
		if err := r.db.QueryRowContext(ctx, queueQ,
			strings.TrimSpace(current.AgentID),
			strings.TrimSpace(taskID),
			string(domain.TaskStatusPending),
			string(domain.TaskStatusQueued),
			string(domain.TaskStatusClaimed),
			string(domain.TaskStatusRunning),
		).Scan(&queueCount); err != nil {
			return domain.Task{}, err
		}
		if queueCount > 0 {
			nextStatus = domain.TaskStatusQueued
		} else {
			nextStatus = domain.TaskStatusPending
		}
	}
	successCount := current.SuccessCount
	failureCount := current.FailureCount
	if status == domain.TaskStatusSuccess {
		successCount++
	}
	if status == domain.TaskStatusFailed {
		failureCount++
	}
	lastSummary := strings.TrimSpace(failureReason)
	if lastSummary == "" {
		if status == domain.TaskStatusSuccess {
			lastSummary = firstNonEmpty(strings.TrimSpace(firstLineFromText(stdoutText)), "执行成功")
		} else if status == domain.TaskStatusCancelled {
			lastSummary = "已取消"
		} else {
			lastSummary = firstNonEmpty(strings.TrimSpace(firstLineFromText(stderrText)), "执行失败")
		}
	}
	const q = `
UPDATE agent_task
SET status = ?, finished_at = ?, updated_at = ?, exit_code = ?, stdout_text = ?, stderr_text = ?, failure_reason = ?,
	run_count = ?, success_count = ?, failure_count = ?, last_run_status = ?, last_run_summary = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q,
		string(nextStatus),
		finishedAt.UTC().UnixNano(),
		finishedAt.UTC().UnixNano(),
		exitCode,
		stdoutText,
		stderrText,
		failureReason,
		current.RunCount+1,
		successCount,
		failureCount,
		string(status),
		lastSummary,
		strings.TrimSpace(taskID),
	)
	if err != nil {
		return domain.Task{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if affected == 0 {
		return domain.Task{}, domain.ErrTaskNotFound
	}
	return r.GetTaskByID(ctx, taskID)
}

// scanAgentInstance 封装当前模块的业务处理逻辑。
func scanAgentInstance(scanner interface{ Scan(dest ...any) error }) (domain.Instance, error) {
	var item domain.Instance
	var encryptedToken string
	var tagsJSON string
	var status string
	var lastTaskStatus string
	var lastHeartbeatAt int64
	var currentTaskStartedAt int64
	var lastTaskFinishedAt int64
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&item.ID,
		&item.MachineID,
		&item.AgentCode,
		&item.Name,
		&item.EnvironmentCode,
		&item.WorkDir,
		&encryptedToken,
		&tagsJSON,
		&item.Hostname,
		&item.HostIP,
		&item.AgentVersion,
		&item.OS,
		&item.Arch,
		&status,
		&lastHeartbeatAt,
		&item.CurrentTaskID,
		&item.CurrentTaskName,
		&item.CurrentTaskType,
		&currentTaskStartedAt,
		&lastTaskStatus,
		&item.LastTaskSummary,
		&lastTaskFinishedAt,
		&item.Remark,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Instance{}, err
	}
	token, err := decryptStoredSecret(encryptedToken)
	if err != nil {
		return domain.Instance{}, err
	}
	item.Token = token
	item.Status = domain.Status(strings.TrimSpace(status))
	item.LastTaskStatus = domain.LastTaskStatus(strings.TrimSpace(lastTaskStatus))
	item.Tags = unmarshalStringSlice(tagsJSON)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if lastHeartbeatAt > 0 {
		item.LastHeartbeatAt = time.Unix(0, lastHeartbeatAt).UTC()
	}
	if currentTaskStartedAt > 0 {
		t := time.Unix(0, currentTaskStartedAt).UTC()
		item.CurrentTaskStarted = &t
	}
	if lastTaskFinishedAt > 0 {
		t := time.Unix(0, lastTaskFinishedAt).UTC()
		item.LastTaskFinishedAt = &t
	}
	return item, nil
}

// scanAgentTask 封装当前模块的业务处理逻辑。
func scanAgentTask(scanner interface{ Scan(dest ...any) error }) (domain.Task, error) {
	var item domain.Task
	var targetAgentIDsJSON string
	var variablesJSON string
	var status string
	var taskMode string
	var claimedAt int64
	var startedAt int64
	var finishedAt int64
	var lastRunStatus string
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&item.ID,
		&item.AgentID,
		&item.AgentCode,
		&targetAgentIDsJSON,
		&item.SourceTaskID,
		&item.DispatchBatchID,
		&item.Name,
		&taskMode,
		&item.TaskType,
		&item.ShellType,
		&item.WorkDir,
		&item.ScriptID,
		&item.ScriptName,
		&item.ScriptPath,
		&item.ScriptText,
		&variablesJSON,
		&item.TimeoutSec,
		&status,
		&claimedAt,
		&startedAt,
		&finishedAt,
		&item.ExitCode,
		&item.StdoutText,
		&item.StderrText,
		&item.FailureReason,
		&item.RunCount,
		&item.SuccessCount,
		&item.FailureCount,
		&lastRunStatus,
		&item.LastRunSummary,
		&item.CreatedBy,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Task{}, err
	}
	item.TaskMode = domain.TaskMode(firstNonEmpty(strings.TrimSpace(taskMode), string(domain.TaskModeTemporary)))
	item.Status = domain.TaskStatus(strings.TrimSpace(status))
	item.LastRunStatus = domain.TaskStatus(strings.TrimSpace(lastRunStatus))
	item.TargetAgentIDs = unmarshalStringSlice(targetAgentIDsJSON)
	item.Variables = unmarshalStringMap(variablesJSON)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if claimedAt > 0 {
		t := time.Unix(0, claimedAt).UTC()
		item.ClaimedAt = &t
	}
	if startedAt > 0 {
		t := time.Unix(0, startedAt).UTC()
		item.StartedAt = &t
	}
	if finishedAt > 0 {
		t := time.Unix(0, finishedAt).UTC()
		item.FinishedAt = &t
	}
	return item, nil
}

// scanAgentScript 封装当前模块的业务处理逻辑。
func scanAgentScript(scanner interface{ Scan(dest ...any) error }) (domain.Script, error) {
	var item domain.Script
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.TaskType,
		&item.ShellType,
		&item.ScriptPath,
		&item.ScriptText,
		&item.CreatedBy,
		&item.UpdatedBy,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Script{}, err
	}
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// marshalStringSlice 封装当前模块的业务处理逻辑。
func marshalStringSlice(items []string) string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item); value != "" {
			normalized = append(normalized, value)
		}
	}
	data, _ := json.Marshal(normalized)
	return string(data)
}

// marshalStringMap 封装当前模块的业务处理逻辑。
func marshalStringMap(items map[string]string) string {
	if items == nil {
		items = map[string]string{}
	}
	data, _ := json.Marshal(items)
	return string(data)
}

// unmarshalStringSlice 封装当前模块的业务处理逻辑。
func unmarshalStringSlice(raw string) []string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}
	}
	var result []string
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return []string{}
	}
	return result
}

// unmarshalStringMap 封装当前模块的业务处理逻辑。
func unmarshalStringMap(raw string) map[string]string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return map[string]string{}
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return map[string]string{}
	}
	if result == nil {
		return map[string]string{}
	}
	return result
}

// ptrTimeToUnixNano 封装当前模块的业务处理逻辑。
func ptrTimeToUnixNano(value *time.Time) int64 {
	if value == nil || value.IsZero() {
		return 0
	}
	return value.UTC().UnixNano()
}

// timeToUnixNano 封装当前模块的业务处理逻辑。
func timeToUnixNano(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixNano()
}

// isDuplicateAgentCodeError 封装当前模块的业务处理逻辑。
func isDuplicateAgentCodeError(driver string, err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if driver == "sqlite" {
		return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "agent_instance.agent_code")
	}
	return strings.Contains(msg, "duplicate") && strings.Contains(msg, "uk_agent_instance_code") ||
		(strings.Contains(msg, "duplicate") && strings.Contains(msg, "agent_code"))
}

// isAlreadyExistsIndexError 封装当前模块的业务处理逻辑。
func isAlreadyExistsIndexError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key name") || strings.Contains(msg, "already exists")
}

// firstNonEmpty 封装当前模块的业务处理逻辑。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// firstLineFromText 封装当前模块的业务处理逻辑。
func firstLineFromText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, sep := range []string{"\n", "\r"} {
		if idx := strings.Index(value, sep); idx >= 0 {
			return strings.TrimSpace(value[:idx])
		}
	}
	return value
}

// generateBootstrapToken 封装当前模块的业务处理逻辑。
func generateBootstrapToken() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "agboot-" + fmt.Sprint(time.Now().UTC().UnixNano())
	}
	return "agboot-" + hex.EncodeToString(buf)
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *AgentRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(1)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?;`,
		strings.TrimSpace(table),
		strings.TrimSpace(column),
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// sqliteTableColumns 封装当前模块的业务处理逻辑。
func (r *AgentRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s);`, strings.TrimSpace(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			dataType   string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, err
		}
		columns[strings.TrimSpace(name)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}
