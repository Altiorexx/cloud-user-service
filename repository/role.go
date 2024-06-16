package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"user.service.altiore.io/types"
)

type RoleRepository interface {
	ReadRoles(groupId string) ([]*types.Role, error)

	UpdateRoles(roles []*types.Role, groupId string) error
	UpdateRolesWithTx(tx *sql.Tx, roles []*types.Role, groupId string) error

	CreateGroupOwnerRole(tx *sql.Tx, groupId string, userId string) error

	GetMembersWithRoles(groupId string) ([]*types.MemberRole, error)
	GetMembersWithRolesWithTx(tx *sql.Tx, groupId string) ([]*types.MemberRole, error)

	DeleteRole(roleId string) error
	DeleteRoleWithTx(tx *sql.Tx, roleId string) error

	AddMemberRole(tx *sql.Tx, userId string, roleId string) error
	RemoveMemberRole(tx *sql.Tx, userId string, roleId string) error

	ReadMemberRoles(userId string, groupId string) ([]*types.Role, error)
	ReadMemberRolesWithTx(tx *sql.Tx, userId string, groupId string) ([]*types.Role, error)
}

type RoleRepositoryOpts struct {
	Key string
}

var (
	role_repository_instance_map = make(map[string]*RoleRepositoryImpl)
	role_mu                      sync.Mutex
)

type RoleRepositoryImpl struct {
	client *sql.DB
}

func NewRoleRepository(opts *RoleRepositoryOpts) *RoleRepositoryImpl {
	role_mu.Lock()
	defer role_mu.Unlock()
	if instance, exists := role_repository_instance_map[opts.Key]; exists {
		return instance
	}
	var (
		uri                = ""
		user               = os.Getenv("DB_BUSINESS_USER")
		pass               = os.Getenv("DB_BUSINESS_PASS")
		host               = os.Getenv("DB_BUSINESS_HOST")
		port               = os.Getenv("DB_BUSINESS_PORT")
		instance_conn_name = os.Getenv("DB_BUSINESS_INSTANCE_CONN_NAME")
	)
	switch os.Getenv("ENV") {

	case "LOCAL":
		log.Println("loading connection info for local mysql server")
		uri = fmt.Sprintf("%s:%s@tcp(%s:%s)/core?parseTime=true", user, pass, host, port)

	default:
		log.Println("loading connection info for google cloud mysql server...")
		d, err := cloudsqlconn.NewDialer(context.Background())
		if err != nil {
			panic(err)
		}
		mysql.RegisterDialContext("cloudsqlconn", func(ctx context.Context, addr string) (net.Conn, error) {
			return d.Dial(ctx, instance_conn_name, []cloudsqlconn.DialOption{}...)
		})
		uri = fmt.Sprintf("%s:%s@cloudsqlconn(localhost:%s)/core?parseTime=true", user, pass, port)
	}
	print(uri)
	db, err := sql.Open("mysql", uri)
	if err != nil {
		panic(err)
	}
	if err := db.Ping(); err != nil {
		panic(err)
	}
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	log.Println("connected to core database.")

	role_repository_instance_map[opts.Key] = &RoleRepositoryImpl{
		client: db,
	}
	return role_repository_instance_map[opts.Key]
}

func (repository *RoleRepositoryImpl) ReadMemberRoles(userId string, groupId string) ([]*types.Role, error) {
	return repository.readMemberRoles(repository.client, userId, groupId)
}

func (repository *RoleRepositoryImpl) ReadMemberRolesWithTx(tx *sql.Tx, userId string, groupId string) ([]*types.Role, error) {
	return repository.readMemberRoles(tx, userId, groupId)
}

// Reads a user's roles within a group.
func (repository *RoleRepositoryImpl) readMemberRoles(exe types.Execer, userId string, groupId string) ([]*types.Role, error) {
	rows, err := exe.Query("SELECT r.id, r.name, r.groupId, "+
		"r.renameGroup, r.deleteGroup, r.inviteMember, r.removeMember, "+
		"r.createCase, r.updateCaseMetadata, r.deleteCase, r.exportCase, "+
		"r.viewLogs, r.exportLogs "+
		"FROM user_role ur "+
		"INNER JOIN role r ON ur.roleId = r.id "+
		"INNER JOIN organisation_user ou ON ur.userId = ou.userId "+
		"WHERE ur.userId = ? AND ou.organisationId = ?", userId, groupId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []*types.Role
	for rows.Next() {
		var role types.Role
		if err := rows.Scan(
			&role.Id, &role.Name, &role.GroupId,
			&role.RenameGroup, &role.DeleteGroup, &role.InviteMember, &role.RemoveMember,
			&role.CreateCase, &role.UpdateCaseMetadata, &role.DeleteCase, &role.ExportCase,
			&role.ViewLogs, &role.ExportLogs); err != nil {
			return nil, err
		}
		roles = append(roles, &role)
	}
	return roles, nil
}

// Checks if the user has permission to an action.
func (repository *RoleRepositoryImpl) HasPermission(tx *sql.Tx, userId string, groupId string) error {

	return nil
}

// Remove a role from the specified user, by deleting the user_role mapping.
func (repository *RoleRepositoryImpl) RemoveMemberRole(tx *sql.Tx, userId string, roleId string) error {

	// check if the role being removed is "Group Owner"
	checkRoleStmt, err := tx.Prepare("SELECT name FROM role WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer checkRoleStmt.Close()
	var roleName string
	if err := checkRoleStmt.QueryRow(roleId).Scan(&roleName); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w:, role with id %s not found", types.ErrNotFound, roleId)
		}
		return fmt.Errorf("%w: failed to execute query: %v", types.ErrGenericSQL, err)
	}

	if roleName == "Group Owner" {
		// check how many users have the "Group Owner" role
		checkMembersStmt, err := tx.Prepare("SELECT COUNT(*) FROM user_role WHERE roleId = ?")
		if err != nil {
			return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
		}
		defer checkMembersStmt.Close()
		var count int
		if err := checkMembersStmt.QueryRow(roleId).Scan(&count); err != nil {
			return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot remove the last Group Owner role from the group", types.ErrForbiddenOperation)
		}
	}

	stmt, err := tx.Prepare("DELETE FROM user_role WHERE userId = ? AND roleId = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	if _, err := stmt.Exec(userId, roleId); err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

// Add a role to the specified user, by mapping role to user.
func (repository *RoleRepositoryImpl) AddMemberRole(tx *sql.Tx, userId string, roleId string) error {
	stmt, err := tx.Prepare("INSERT INTO user_role VALUES (?, ? ,?)")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	if _, err := stmt.Exec(uuid.NewString(), userId, roleId); err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

func (repository *RoleRepositoryImpl) GetMembersWithRolesWithTx(tx *sql.Tx, groupId string) ([]*types.MemberRole, error) {
	return repository.getMembersWithRoles(tx, groupId)
}

func (repository *RoleRepositoryImpl) GetMembersWithRoles(groupId string) ([]*types.MemberRole, error) {
	return repository.getMembersWithRoles(repository.client, groupId)
}

func (repository *RoleRepositoryImpl) getMembersWithRoles(exe types.Execer, groupId string) ([]*types.MemberRole, error) {
	query := "SELECT u.id AS user_id, u.name AS user_name, r.id AS role_id, r.name AS role_name " +
		"FROM user u " +
		"INNER JOIN organisation_user ou ON u.id = ou.userId " +
		"INNER JOIN user_role ur ON u.id = ur.userId " +
		"INNER JOIN role r ON ur.roleId = r.id " +
		"WHERE ou.organisationId = ?"
	rows, err := exe.Query(query, groupId)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to execute query: %v", types.ErrGenericSQL, err)
	}
	defer rows.Close()
	memberRolesMap := make(map[string]*types.MemberRole)
	for rows.Next() {
		var userId, userName, roleId, roleName string
		if err := rows.Scan(&userId, &userName, &roleId, &roleName); err != nil {
			return nil, fmt.Errorf("%w: failed to scan row: %v", types.ErrGenericSQL, err)
		}
		if _, exists := memberRolesMap[userId]; !exists {
			memberRolesMap[userId] = &types.MemberRole{
				Id:     userId,
				Member: userName,
				Roles:  []*types.Role{},
			}
		}
		memberRolesMap[userId].Roles = append(memberRolesMap[userId].Roles, &types.Role{
			Id:   roleId,
			Name: roleName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: rows iteration error: %v", types.ErrGenericSQL, err)
	}
	var memberRoles []*types.MemberRole
	for _, mr := range memberRolesMap {
		memberRoles = append(memberRoles, mr)
	}
	return memberRoles, nil
}

func (repository *RoleRepositoryImpl) CreateGroupOwnerRole(tx *sql.Tx, groupId string, userId string) error {

	// create role
	createRoleStmt, err := tx.Prepare("INSERT INTO role VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer createRoleStmt.Close()
	roleId := uuid.NewString()
	_, err = createRoleStmt.Exec(roleId, "Group Owner", groupId, true, true, true, true, true, true, true, true, true, true)
	if err != nil {
		log.Printf("error creating group owner role: %+v\n", err)
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}

	// map role to user
	mapRoleStmt, err := tx.Prepare("INSERT INTO user_role VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer mapRoleStmt.Close()
	user_role_id := uuid.NewString()
	_, err = mapRoleStmt.Exec(user_role_id, userId, roleId)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

func (repository *RoleRepositoryImpl) ReadRoles(groupId string) ([]*types.Role, error) {
	stmt, err := repository.client.Prepare("SELECT * FROM role WHERE organisationId = ?")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()

	rows, err := stmt.Query(groupId)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %v", err)
	}
	defer rows.Close()

	var roles []*types.Role
	for rows.Next() {
		var role types.Role
		if err := rows.Scan(&role.Id, &role.Name, &role.GroupId, &role.RenameGroup, &role.DeleteGroup, &role.InviteMember, &role.RemoveMember, &role.CreateCase, &role.UpdateCaseMetadata, &role.DeleteCase, &role.ExportCase, &role.ViewLogs, &role.ExportLogs); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}
		roles = append(roles, &role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %v", err)
	}

	return roles, nil
}

func (repository *RoleRepositoryImpl) DeleteRoleWithTx(tx *sql.Tx, roleId string) error {
	return repository.deleteRole(tx, roleId)
}

func (repository *RoleRepositoryImpl) DeleteRole(roleId string) error {
	return repository.deleteRole(repository.client, roleId)
}

func (repository *RoleRepositoryImpl) deleteRole(exe types.Execer, roleId string) error {
	// delete all user_role mappings
	user_role_stmt, err := exe.Prepare("DELETE FROM user_role WHERE roleId = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer user_role_stmt.Close()
	if _, err := user_role_stmt.Exec(roleId); err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	// delete role
	stmt, err := exe.Prepare("DELETE FROM role WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	if _, err := stmt.Exec(roleId); err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

func (repository *RoleRepositoryImpl) UpdateRoles(roles []*types.Role, groupId string) error {
	return repository.updateRoles(repository.client, roles, groupId)
}

func (repository *RoleRepositoryImpl) UpdateRolesWithTx(tx *sql.Tx, roles []*types.Role, groupId string) error {
	return repository.updateRoles(tx, roles, groupId)
}

func (repository *RoleRepositoryImpl) updateRoles(exe types.Execer, roles []*types.Role, groupId string) error {

	// check
	checkStmt, err := exe.Prepare("SELECT id FROM role WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer checkStmt.Close()

	// update (existing roles)
	updateStmt, err := exe.Prepare("UPDATE role SET name = ?, rename_organisation = ?, delete_organisation = ?, invite_member = ?, remove_member = ?, create_case = ?, update_case_metadata = ?, delete_case = ?, export_case = ?, view_logs = ?, export_logs = ? WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer updateStmt.Close()

	// insert (new roles)
	insertStmt, err := exe.Prepare("INSERT INTO role VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer insertStmt.Close()

	wg := sync.WaitGroup{}
	wg.Add(len(roles))
	var _err error
	for _, role := range roles {
		go func(role *types.Role) {
			defer wg.Done()

			// dont do anything to the "Group Owner" role, as this prevents lock-outs of user's own groups.
			if role.Name == "Group Owner" {
				return
			}

			// check if exists
			var id string
			err := checkStmt.QueryRow(role.Id).Scan(&id)
			if err != nil && err != sql.ErrNoRows {
				log.Printf("error reading role: %+v\n", err)
				_err = err
				return
			}
			if err == sql.ErrNoRows {
				// if not exists, insert
				_, err = insertStmt.Exec(role.Id, role.Name, groupId, role.RenameGroup, role.DeleteGroup, role.InviteMember, role.RemoveMember, role.CreateCase, role.UpdateCaseMetadata, role.DeleteCase, role.ExportCase, role.ViewLogs, role.ExportLogs)
				if err != nil {
					log.Printf("error creating role: %+v\n", err)
					_err = err
					return
				}
			} else {
				// if exists, update
				_, err = updateStmt.Exec(role.Name, role.RenameGroup, role.DeleteGroup, role.InviteMember, role.RemoveMember, role.CreateCase, role.UpdateCaseMetadata, role.DeleteCase, role.ExportCase, role.ViewLogs, role.ExportLogs, role.Id)
				if err != nil {
					log.Printf("error updating role: %+v\n", err)
					_err = err
					return
				}
			}
		}(role)
	}
	wg.Wait()
	return _err
}
