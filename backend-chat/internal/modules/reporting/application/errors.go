package application

import "errors"

var (
	ErrInvalidQuery     = errors.New("invalid reporting query")
	ErrShopNotFound     = errors.New("reporting shop not found")
	ErrInconsistentData = errors.New("inconsistent reporting data")
)
