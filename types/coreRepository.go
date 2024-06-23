package types

import (
	"database/sql"
)

type Service struct {
	Id                  string `json:"id"`
	Name                string `json:"name"`
	ImplementationGroup *int   `json:"implementationGroup"`
	Description         string `json:"description"`
}

type Organisation struct {
	Id              string `json:"id"`
	Name            string `json:"name"`
	CasePermissions []any  `json:"casePermissions"`
	Members         []any  `json:"members"`
}

type OrganisationMember struct {
	Id    string `json:"id"`
	Email string `json:"email"`
}

// Interface allowing for dynamic methods differing between client and transaction use.
type Execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Prepare(query string) (*sql.Stmt, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

type User struct {
	Id        string `json:"id"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	LastLogin string `json:"lastLogin"`
	Verified  bool   `json:"verified"`
}
