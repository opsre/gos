package pipelinescan

import "errors"

var (
	ErrRuleNotFound   = errors.New("pipeline scan rule not found")
	ErrResultNotFound = errors.New("pipeline scan result not found")
	ErrRuleDuplicated = errors.New("pipeline scan rule already exists")
)
