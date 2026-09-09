package usecase

import "context"

// Only internal orchestration may supply a recovery ID. Public HTTP payloads do
// not carry IDs; all existing callers retain their random-ID behavior.
type recoveryIDKey struct{}
type recoveryID struct{ prefix, id string }

func withRecoveryID(ctx context.Context, prefix, id string) context.Context {
	return context.WithValue(ctx, recoveryIDKey{}, recoveryID{prefix, id})
}
func creationID(ctx context.Context, prefix string) string {
	if value, ok := ctx.Value(recoveryIDKey{}).(recoveryID); ok && value.prefix == prefix && value.id != "" {
		return value.id
	}
	return generateID(prefix)
}
