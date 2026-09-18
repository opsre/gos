package releaseautomation

import (
	"context"
	"time"
)

type Repository interface {
	InitSchema(ctx context.Context) error
	Create(ctx context.Context, item Automation) error
	GetByID(ctx context.Context, id string) (Automation, error)
	List(ctx context.Context, filter ListFilter) ([]Automation, int64, error)
	Update(ctx context.Context, item Automation) error
	Delete(ctx context.Context, id string) error

	// ListEnabled 返回启用中的配置，按 last_checked_at 升序（从未检查过的排最前），
	// 让轮询在 limit 受限时优先覆盖很久没看过的配置。limit 由调用方给出上限。
	ListEnabled(ctx context.Context, limit int) ([]Automation, error)

	// ResetBaseline 在应用/环境/分支变更后重设基线（旧 sha 属于旧分支，留着会被当成
	// 「新提交已处理」而漏发，或反过来立刻误发一单）。
	//
	// 它是一条条件更新：只有当前身份与新身份一致、且 last_seen_sha 仍是保存前读到的
	// 值时才会写回。返回 false 表示轮询（或另一个编辑者）已经改过这一行，本次不覆盖，
	// 由调用方读取最新状态。
	ResetBaseline(
		ctx context.Context,
		id string,
		applicationID string,
		envCode string,
		gitRef string,
		expectedSeenSHA string,
		seenSHA string,
		updatedAt time.Time,
	) (bool, error)

	// UpdateCheckState 只刷新轮询结果列（last_checked_at / last_error），
	// 有意不动 last_seen_sha：没有新提交或本轮不建单时基线必须保持原值。
	UpdateCheckState(ctx context.Context, id string, checkedAt time.Time, lastError string) error

	// CommitTrigger 是抢占式（CAS）写回：只有当前 last_seen_sha 仍等于 expectedSeenSHA
	// 时才推进基线。返回值 false 表示别的副本已经处理过这一轮，调用方必须丢弃本次结果
	// （发布单已经建好了，回滚它比重复建单更糟）。
	CommitTrigger(
		ctx context.Context,
		id string,
		expectedSeenSHA string,
		seenSHA string,
		triggeredSHA string,
		orderID string,
		checkedAt time.Time,
		lastError string,
	) (bool, error)
}
