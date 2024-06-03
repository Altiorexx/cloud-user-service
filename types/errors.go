package types

import "errors"

// core repository
var (
	ErrTxCommit         = errors.New("error committing transaction")
	ErrRollback         = errors.New("unable to perform rollback")
	ErrPrepareStatement = errors.New("error preparing statement")
	ErrNotFound         = errors.New("not found")

	ErrTxCancelled = errors.New("transaction was cancelled")

	ErrInvitationNotFound = errors.New("invitation not found")
	ErrGenericSQL         = errors.New("generic sql error")
)

// firebase service
var (
	ErrFirebaseError = errors.New("firebase error")
)
