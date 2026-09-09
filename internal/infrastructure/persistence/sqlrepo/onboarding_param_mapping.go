package sqlrepo

import (
	"context"
	"time"

	ep "gos/internal/domain/executorparam"
	ob "gos/internal/domain/onboarding"
)

// Compare-and-set avoids overwriting a mapping installed by another application
// between preview and apply. The normal management endpoint remains unchanged.
func (r *ExecutorParamRepository) MapUnmappedParameter(ctx context.Context, id, key string) (ep.ExecutorParamDef, error) {
	_, err := r.db.ExecContext(ctx, `UPDATE executor_param_def SET param_key=?,updated_at=? WHERE id=? AND param_key='' AND status='active'`, key, time.Now().UTC().UnixNano(), id)
	if err != nil {
		return ep.ExecutorParamDef{}, err
	}
	current, err := r.GetByID(ctx, id)
	if err != nil {
		return current, err
	}
	if current.ParamKey != key || current.Status != ep.StatusActive {
		return current, ob.ErrConflict
	}
	return current, nil
}
