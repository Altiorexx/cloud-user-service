package types

import "errors"

var (
	ErrAssert = errors.New("error asserting value")
)

// core repository
var (
	ErrTxCreate         = errors.New("error creating transaction")
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
