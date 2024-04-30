package types

import "errors"

// core repository
var (
	ErrTxCancelled        = errors.New("transaction was cancelled")
	ErrRollback           = errors.New("unable to perform rollback")
	ErrInvitationNotFound = errors.New("invitation not found")
	ErrPrepareStatement   = errors.New("error preparing statement")
	ErrGenericSQL         = errors.New("generic sql error")
	ErrTxCommit           = errors.New("error committing transaction")
	ErrNotFound           = errors.New("not found")
)

// firebase service
var (
	ErrFirebaseError = errors.New("firebase error")
)
