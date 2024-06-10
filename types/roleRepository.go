package types

type Role struct {
	Id      string
	Name    string
	GroupId string

	// Group
	RenameGroup bool
	DeleteGroup bool

	// Members
	InviteMember bool
	RemoveMember bool

	// Case
	CreateCase         bool
	UpdateCaseMetadata bool
	DeleteCase         bool
	ExportCase         bool

	// Logs
	ViewLogs   bool
	ExportLogs bool
}
