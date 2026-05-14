package artifactrepo

import "errors"

var (
	ErrNotFound       = errors.New("artifact repository not found")
	ErrNameDuplicated = errors.New("artifact repository name already exists")
)
