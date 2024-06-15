package types

type MemberRole struct {
	Id     string  `json:"id"`
	Member string  `json:"member"`
	Roles  []*Role `json:"roles"`
}

type Role struct {
	Id      string `json:"id" binding:"required"`
	Name    string `json:"name" binding:"required"`
	GroupId string `json:"groupId" binding:"required"`

	// Group
	RenameGroup bool `json:"renameGroup"`
	DeleteGroup bool `json:"deleteGroup"`

	// Members
	InviteMember bool `json:"inviteMember"`
	RemoveMember bool `json:"removeMember"`

	// Case
	CreateCase         bool `json:"createCase"`
	UpdateCaseMetadata bool `json:"updateCaseMetadata"`
	DeleteCase         bool `json:"deleteCase"`
	ExportCase         bool `json:"exportCase"`

	// Logs
	ViewLogs   bool `json:"viewLogs"`
	ExportLogs bool `json:"exportLogs"`
}