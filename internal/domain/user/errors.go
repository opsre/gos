package user

import "errors"

var (
	ErrUserNotFound            = errors.New("user not found")
	ErrUsernameDuplicated      = errors.New("username duplicated")
	ErrPermissionNotFound      = errors.New("permission not found")
	ErrSessionNotFound         = errors.New("session not found")
	ErrSessionRevoked          = errors.New("session revoked")
	ErrParamPermissionNotFound = errors.New("param permission not found")
	ErrUserManagerNotFound     = errors.New("user manager not found")
	ErrUserManagerCycle        = errors.New("user manager cycle")
)
