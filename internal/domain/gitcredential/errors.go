package gitcredential

import "errors"

var (
	ErrNotFound       = errors.New("git credential not found")
	ErrNameDuplicated = errors.New("git credential name already exists")
)
