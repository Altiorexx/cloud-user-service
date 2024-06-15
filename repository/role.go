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
	UpdateRoles(tx *sql.Tx, roles []*types.Role, groupId string) error
	DeleteRole(tx *sql.Tx, roleId string) error
	CreateGroupOwnerRole(tx *sql.Tx, groupId string, userId string) error
	GetMembersWithRoles(groupId string) ([]*types.MemberRole, error)
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

func (repository *RoleRepositoryImpl) GetMembersWithRoles(groupId string) ([]*types.MemberRole, error) {
	query := "SELECT u.id AS user_id, u.name AS user_name, r.id AS role_id, r.name AS role_name " +
		"FROM user u " +
		"INNER JOIN organisation_user ou ON u.id = ou.userId " +
		"INNER JOIN user_role ur ON u.id = ur.userId " +
		"INNER JOIN role r ON ur.roleId = r.id " +
		"WHERE ou.organisationId = ?"

	rows, err := repository.client.Query(query, groupId)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %v", err)
	}
	defer rows.Close()

	memberRolesMap := make(map[string]*types.MemberRole)
	for rows.Next() {
		var userId, userName, roleId, roleName string
		if err := rows.Scan(&userId, &userName, &roleId, &roleName); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
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
		return nil, fmt.Errorf("rows iteration error: %v", err)
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
		return err
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

func (repository *RoleRepositoryImpl) DeleteRole(tx *sql.Tx, roleId string) error {
	stmt, err := tx.Prepare("DELETE FROM role WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	if _, err := stmt.Exec(roleId); err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

func (repository *RoleRepositoryImpl) UpdateRoles(tx *sql.Tx, roles []*types.Role, groupId string) error {

	// check
	checkStmt, err := tx.Prepare("SELECT id FROM role WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer checkStmt.Close()

	// update (existing roles)
	updateStmt, err := tx.Prepare("UPDATE role SET name = ?, rename_organisation = ?, delete_organisation = ?, invite_member = ?, remove_member = ?, create_case = ?, update_case_metadata = ?, delete_case = ?, export_case = ?, view_logs = ?, export_logs = ? WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer updateStmt.Close()

	// insert (new roles)
	insertStmt, err := tx.Prepare("INSERT INTO role VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
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
