package types

var (
	RENAME_GROUP = "RenameGroup"
	DELETE_GROUP = "DeleteGroup"

	INVITE_MEMBER = "InviteMember"
	REMOVE_MEMBER = "RemoveMember"

	CREATE_CASE          = "CreateCase"
	UPDATE_CASE_METADATA = "UpdateCaseMetadata"
	DELETE_CASE          = "DeleteCase"
	EXPORT_CASE          = "ExportCase"

	VIEW_LOGS   = "ViewLogs"
	EXPORT_LOGS = "ExportLogs"
)

type MemberRole struct {
	Id     string  `json:"id" binding:"required"`
	Member string  `json:"member" binding:"required"`
	Roles  []*Role `json:"roles" binding:"required"`
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
