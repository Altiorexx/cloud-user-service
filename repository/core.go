package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"user.service.altiore.io/service"
	"user.service.altiore.io/types"
)

type CoreRepository interface {
	WithTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error

	NewTransaction(ctx context.Context, readOnly bool) (*sql.Tx, error)
	CommitTransaction(tx *sql.Tx) error
	ReadUserById(userId string) (*types.User, error)
	UpdateGroupName(groupId string, name string) error
	UpdateGroupNameWithTx(tx *sql.Tx, groupId string, name string) error
	DeleteGroupWithTx(tx *sql.Tx, userId string, groupId string) error
	UpdatePassword(uid string, password string) error
	Login(uid string, email string, password string) error
	Signup(userId string, name string) error
	ReadUserByEmail(email string) (*types.User, error)
	VerifyUser(userId string) error
	CreateUser(tx *sql.Tx, userId string, name string) error
	CreateUserWithTx(tx *sql.Tx, userId string, name string, email string, password string) error
	UserExists(uid string) error
	ReadServices() ([]*types.Service, error)
	ImplementationGroupCount(serviceName string) (int, error)
	RegisterUsedService(serviceName string, implementationGroup *int, organisationId string, userId string) error
	RegisterUsedServiceWithTx(tx *sql.Tx, serviceName string, implementationGroup *int, organisationId string, userId string) error
	OrganisationList(userId string) ([]*types.Organisation, error)
	ReadOrganisationMembers(id string) ([]*types.OrganisationMember, error)
	CreateInvitation(userId string, email string, groupId string) (string, error)
	IsUserAlreadyMember(userId string, groupId string) error
	ReadGroup(ctx context.Context, groupId string) (*types.Organisation, error)
	LookupInvitation(invitationId string) (string, string, string, error)
	DeleteInvitation(id string) error
	DeleteInvitationWithTx(tx *sql.Tx, id string) error
	AddUserToOrganisationWithTx(tx *sql.Tx, userId string, groupId string) error
	AddUserToOrganisation(userId string, organisationId string) error
	InvitationSignup(invitationId string, email string, password string, name string) error
	DeleteUser(userId string) error
	DeleteUserWithTx(tx *sql.Tx, userId string) error
	RemoveUserFromOrganisationWithTx(tx *sql.Tx, userId string, organisationId string) error
	CreateOrganisationWithTx(tx *sql.Tx, name string, userId string) error
}

type CoreRepositoryOpts struct {
	Firebase service.FirebaseService
	Role     RoleRepository
}

var (
	core_repository_instance_map = make(map[string]*CoreRepositoryImpl)
	mu                           sync.Mutex
)

type CoreRepositoryImpl struct {
	client   *sql.DB
	firebase service.FirebaseService
	role     RoleRepository
}

func NewCoreRepository(opts *CoreRepositoryOpts, key string) *CoreRepositoryImpl {
	mu.Lock()
	defer mu.Unlock()
	if instance, exists := core_repository_instance_map[key]; exists {
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

	core_repository_instance_map[key] = &CoreRepositoryImpl{
		client:   db,
		firebase: opts.Firebase,
		role:     opts.Role,
	}
	log.Println("initialized core repository")
	return core_repository_instance_map[key]
}

// Constructs and wraps a callback with a transaction, ensuring proper commit and rollback handling.
func (repository *CoreRepositoryImpl) WithTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error {

	// create tx
	tx, err := repository.NewTransaction(ctx, false)
	if err != nil {
		return err
	}

	// define commit and rollback handling (defer)
	defer func() {
		if r := recover(); r != nil {
			repository.RollbackTransaction(tx)
			panic(r)
		} else if err != nil {
			repository.RollbackTransaction(tx)
		} else {
			err = repository.CommitTransaction(tx)
		}
	}()

	// invoke callback
	err = fn(tx)

	// return error
	return err
}

func (repository *CoreRepositoryImpl) RollbackTransaction(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		log.Printf("transaction rollback failed: %+v\n", err)
	}
}

// Creates a new transaction.
func (repository *CoreRepositoryImpl) NewTransaction(ctx context.Context, readOnly bool) (*sql.Tx, error) {
	opts := &sql.TxOptions{}
	if readOnly {
		opts.ReadOnly = true
	}
	tx, err := repository.client.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return tx, nil
}

// Attempts to commit the transaction and performs a rollback if an error occurs.
func (repository *CoreRepositoryImpl) CommitTransaction(tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		log.Printf("transaction commit failed: %+v\n", err)
		if err := tx.Rollback(); err != nil {
			log.Printf("transaction rollback failed: %+v\n", err)
			return fmt.Errorf("%w: %v", types.ErrRollback, err)
		}
		return fmt.Errorf("%w: %v", types.ErrTxCommit, err)
	}
	return nil
}

func (repository *CoreRepositoryImpl) ReadUserById(userId string) (*types.User, error) {
	stmt, err := repository.client.Prepare("SELECT * FROM user WHERE id = ? LIMIT 1")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()

	var user types.User
	if err := stmt.QueryRow(userId).Scan(&user.Id, &user.Name, &user.Email, &user.Password, &user.LastLogin, &user.Verified); err != nil {
		return nil, fmt.Errorf("error scanning data into variable: %v", err)
	}
	return &user, nil
}

// Updates the group's name.
func (repository *CoreRepositoryImpl) UpdateGroupName(groupId string, name string) error {
	return repository.UpdateGroupNameWithTx(nil, groupId, name)
}

// Updates the group's name.
func (repository *CoreRepositoryImpl) UpdateGroupNameWithTx(tx *sql.Tx, groupId string, name string) error {
	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}
	stmt, err := c.Prepare("UPDATE organisation SET name = ? WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	if _, err := stmt.Exec(name, groupId); err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

// Deletes the group and all associations, if the user deleting it has no groups left, this creates a default group afterwards.
func (repository *CoreRepositoryImpl) DeleteGroupWithTx(tx *sql.Tx, userId string, groupId string) error {

	stmt, err := tx.Prepare("CALL GroupCleanup(?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	if _, err := stmt.Exec(groupId); err != nil {
		return err
	}

	// check if user is associated with atleast one group, if not, create a default
	stmt2, err := tx.Prepare("CALL GetUserOrganisations(?)")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt2.Close()
	rows, err := stmt2.Query(userId)
	if err != nil {
		log.Printf("error reading user groups: %+v\n", err)
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	defer rows.Close()

	// otherwise create a default group for the user
	if !rows.Next() {
		rows.Close()
		if err = repository.CreateOrganisationWithTx(tx, "My organisation", userId); err != nil {
			return err
		}
	}

	return nil
}

// Updates the password for a user.
func (repository *CoreRepositoryImpl) UpdatePassword(uid string, password string) error {
	stmt, err := repository.client.Prepare("UPDATE user SET password = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(hash, uid)
	if err != nil {
		return err
	}
	return nil
}

func (repository *CoreRepositoryImpl) Login(uid string, email string, password string) error {
	stmt, err := repository.client.Prepare("SELECT id, name, email, password, verified FROM user WHERE id = ? AND email = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	var user struct {
		Id       string
		Name     string
		Email    string
		Password string
		Verified bool
	}
	if err := stmt.QueryRow(uid, email).Scan(&user.Id, &user.Name, &user.Email, &user.Password, &user.Verified); err != nil {
		return err
	}
	// check verified status
	if !user.Verified {
		return fmt.Errorf("user hasn't verified their account")
	}
	// check password hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return err
	}
	return nil
}

func (repository *CoreRepositoryImpl) Signup(userId string, name string) error {
	tx, err := repository.client.Begin()
	if err != nil {
		return types.ErrTxCancelled
	}

	defer func() {
		r := recover()
		if err != nil {
			log.Printf("(signup) error: %+v\n", r)
			tx.Rollback()
		}
	}()

	// create user
	if err := repository.CreateUserWithTx(tx, userId, name, "", ""); err != nil {
		return err
	}

	// create organisation and map user to it
	if err := repository.CreateOrganisationWithTx(tx, name, userId); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// Read a user by their given email.
func (repository *CoreRepositoryImpl) ReadUserByEmail(email string) (*types.User, error) {
	stmt, err := repository.client.Prepare("SELECT id, email FROM user WHERE email = ?")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	var user types.User
	if err := stmt.QueryRow(email).Scan(&user.Id, &user.Email); err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrNotFound, err)
	}
	return &user, nil
}

// Allow the user to verify their account by link in mail.
func (repository *CoreRepositoryImpl) VerifyUser(userId string) error {
	stmt, err := repository.client.Prepare("UPDATE user SET verified = true WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(userId)
	if err != nil {
		return err
	}
	return nil
}

// Create a user in our system.
func (repository *CoreRepositoryImpl) CreateUser(tx *sql.Tx, userId string, name string) error {
	return repository.CreateUserWithTx(nil, userId, name, "", "")
}

func (repository *CoreRepositoryImpl) CreateUserWithTx(tx *sql.Tx, userId string, name string, email string, password string) error {
	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}
	stmt, err := c.Prepare("INSERT INTO user (id, name, email, password, lastLogin, verified) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return types.ErrPrepareStatement
	}
	defer stmt.Close()
	hash_password, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(userId, name, email, hash_password, "", false)
	if err != nil {
		return err
	}
	return nil
}

func (repository *CoreRepositoryImpl) UserExists(uid string) error {
	stmt, err := repository.client.Prepare("SELECT * FROM user where id = ?")
	if err != nil {
		return err
	}
	_, err = stmt.Exec(uid)
	if err != nil {
		return err
	}
	return nil
}

func (repository *CoreRepositoryImpl) ReadServices() ([]*types.Service, error) {
	rows, err := repository.client.Query("SELECT * FROM service ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var services []*types.Service
	for rows.Next() {
		service := &types.Service{}
		err := rows.Scan(&service.Id, &service.Name, &service.ImplementationGroup, &service.Description)
		if err != nil {
			return nil, err
		}
		services = append(services, service)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return services, nil
}

func (repository *CoreRepositoryImpl) ImplementationGroupCount(serviceName string) (int, error) {
	stmt, err := repository.client.Prepare("SELECT COUNT(*) FROM service WHERE name = ?")
	if err != nil {
		return 0, nil
	}
	defer stmt.Close()
	var count int
	if err := stmt.QueryRow(serviceName).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (repository *CoreRepositoryImpl) RegisterUsedService(serviceName string, implementationGroup *int, organisationId string, userId string) error {
	return repository.RegisterUsedServiceWithTx(nil, serviceName, implementationGroup, organisationId, userId)
}

// Register a user has used a service.
func (repository *CoreRepositoryImpl) RegisterUsedServiceWithTx(tx *sql.Tx, serviceName string, implementationGroup *int, organisationId string, userId string) error {

	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}

	// dynamically create query, as not all services has implementation groups
	var query string
	var args []interface{}
	if implementationGroup == nil || *implementationGroup == 0 {
		query = "SELECT id FROM service WHERE name = ? AND implementationGroup IS NULL LIMIT 1"
		args = []interface{}{serviceName}
	} else {
		query = "SELECT id FROM service WHERE name = ? AND implementationGroup = ? LIMIT 1"
		args = []interface{}{serviceName, implementationGroup}
	}

	// get serviceId by name and implementationGroup
	stmt, err := c.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	var serviceId string
	if err := stmt.QueryRow(args...).Scan(&serviceId); err != nil {
		return err
	}

	// insert into used_services (id, userId, serviceId)
	if _, err = c.Exec("INSERT INTO used_service (id, organisationId, serviceId, userId) VALUES (?, ?, ?, ?)", uuid.NewString(), organisationId, serviceId, userId); err != nil {
		return err
	}
	return nil
}

// Read organisations for the user
func (repository *CoreRepositoryImpl) OrganisationList(userId string) ([]*types.Organisation, error) {
	stmt, err := repository.client.Prepare("CALL GetUserOrganisations(?)")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	rows, err := stmt.Query(userId)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	defer rows.Close()
	var organisations []*types.Organisation
	for rows.Next() {
		var org types.Organisation
		if err := rows.Scan(&org.Id, &org.Name); err != nil {
			return nil, fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
		}
		organisations = append(organisations, &org)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return organisations, nil
}

// Get all members associated with an organisation.
func (repository *CoreRepositoryImpl) ReadOrganisationMembers(id string) ([]*types.OrganisationMember, error) {
	stmt, err := repository.client.Prepare("CALL GetOrganisationMembers(?)")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	result, err := stmt.Query(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	defer result.Close()
	var members []*types.OrganisationMember
	for result.Next() {
		var org types.OrganisationMember
		if err := result.Scan(&org.Id, &org.Name); err != nil {
			return nil, fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
		}
		members = append(members, &org)
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return members, nil
}

// Create an invitation.
func (repository *CoreRepositoryImpl) CreateInvitation(userId string, email string, groupId string) (string, error) {
	// identifier for the mapping between org and email
	id := uuid.NewString()
	stmt, err := repository.client.Prepare("INSERT INTO invitation (id, userId, email, organisationId) VALUES (?, ?, ?, ?)")
	if err != nil {
		return "", fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(id, userId, email, groupId)
	if err != nil {
		return "", fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return id, nil
}

// Checks whether a user is already a part of the group.
func (repository *CoreRepositoryImpl) IsUserAlreadyMember(userId string, groupId string) error {
	stmt, err := repository.client.Prepare("CALL GetUserOrganisations(?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	rows, err := stmt.Query(userId)
	if err != nil {
		return err
	}
	defer rows.Close()
	var isMember bool
	for rows.Next() {
		var organisation types.Organisation
		if err := rows.Scan(&organisation.Id, &organisation.Name); err != nil {
			return err
		}
		if organisation.Id == groupId {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil
	} else {
		return fmt.Errorf("user is already member of the group")
	}
}

// Read a group.
func (repository *CoreRepositoryImpl) ReadGroup(ctx context.Context, groupId string) (*types.Organisation, error) {
	stmt, err := repository.client.PrepareContext(ctx, "SELECT * FROM organisation WHERE id = ?")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	var group types.Organisation
	if err := stmt.QueryRow(groupId).Scan(&group.Id, &group.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: group %s not found", types.ErrNotFound, groupId)
		}
		return nil, fmt.Errorf("failed to read group %s: %w", groupId, err)
	}
	return &group, nil
}

// Looks up an invitation, ensuring the invitationId is intended for the email.
func (repository *CoreRepositoryImpl) LookupInvitation(invitationId string) (string, string, string, error) {
	stmt, err := repository.client.Prepare("SELECT * FROM invitation WHERE id = ?")
	if err != nil {
		return "", "", "", types.ErrPrepareStatement
	}
	defer stmt.Close()
	var inv struct {
		id     string
		userId string
		email  string
		orgId  string
	}
	if err := stmt.QueryRow(invitationId).Scan(&inv.id, &inv.userId, &inv.email, &inv.orgId); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", "", types.ErrInvitationNotFound
		}
		return "", "", "", types.ErrGenericSQL
	}
	return inv.userId, inv.orgId, inv.email, nil
}

// Delete an invitation.
func (repository *CoreRepositoryImpl) DeleteInvitation(id string) error {
	return repository.DeleteInvitationWithTx(nil, id)
}

func (repository *CoreRepositoryImpl) DeleteInvitationWithTx(tx *sql.Tx, id string) error {
	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}
	stmt, err := c.Prepare("DELETE FROM invitation WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

func (repository *CoreRepositoryImpl) AddUserToOrganisationWithTx(tx *sql.Tx, userId string, groupId string) error {
	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}
	stmt, err := c.Prepare("INSERT INTO organisation_user (id, userId, organisationId) VALUES (?, ?, ?)")
	if err != nil {
		return types.ErrPrepareStatement
	}
	defer stmt.Close()
	if _, err = stmt.Exec(uuid.NewString(), userId, groupId); err != nil {
		return types.ErrGenericSQL
	}
	return nil
}

func (repository *CoreRepositoryImpl) AddUserToOrganisation(userId string, organisationId string) error {
	return repository.AddUserToOrganisationWithTx(nil, userId, organisationId)
}

// This should probably be deleted, as the transaction flows has generally been moved to the api layer. (already implemented in invite/join)
func (repository *CoreRepositoryImpl) InvitationSignup(invitationId string, email string, password string, name string) error {

	var userId string

	// new transaction
	tx, err := repository.client.Begin()
	if err != nil {
		return types.ErrTxCancelled
	}

	// rollback
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				panic(types.ErrRollback)
			}
			// also remove user from firebase, skip if no userId was set
			if userId == "" {
				return
			}
			if err := repository.firebase.DeleteUser(userId); err != nil {
				log.Println(err)
			}
		}
	}()

	// check for invitation
	userId, organisationId, _, err := repository.LookupInvitation(invitationId)
	if err != nil {
		return err
	}

	// create firebase user
	userId, err = repository.firebase.CreateUser(email, password, name)
	if err != nil {
		return err
	}

	// create user in database
	if err = repository.CreateUserWithTx(tx, userId, name, "", ""); err != nil {
		return err
	}

	// add user to organisation
	if err = repository.AddUserToOrganisationWithTx(tx, userId, organisationId); err != nil {
		return err
	}

	// delete invitation
	if err = repository.DeleteInvitationWithTx(tx, invitationId); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return types.ErrTxCommit
	}

	return nil
}

// Non-tx method for deleting a user.
func (repository *CoreRepositoryImpl) DeleteUser(userId string) error {
	return repository.DeleteInvitationWithTx(nil, userId)
}

// Cleanup method to delete everything associated with the userId (user and organisation relations).
func (repository *CoreRepositoryImpl) DeleteUserWithTx(tx *sql.Tx, userId string) error {

	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}

	// delete user from organisation_user
	stmt, err := c.Prepare("DELETE FROM organisation_user WHERE userId = ?")
	if err != nil {
		return types.ErrPrepareStatement
	}
	if _, err = stmt.Exec(userId); err != nil {
		return types.ErrGenericSQL
	}

	// delete user from user
	stmt, err = c.Prepare("DELETE FROM user WHERE id = ?")
	if err != nil {
		return types.ErrPrepareStatement
	}
	if _, err = stmt.Exec(userId); err != nil {
		return types.ErrGenericSQL
	}

	return nil
}

// Remove a user from a group, if user has no group left after removal, create a default one.
func (repository *CoreRepositoryImpl) RemoveUserFromOrganisationWithTx(tx *sql.Tx, userId string, organisationId string) error {

	// delete from group
	stmt1, err := tx.Prepare("DELETE FROM organisation_user WHERE userId = ? AND organisationId = ?")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt1.Close()
	result, err := stmt1.Exec(userId, organisationId)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}

	// check if the mapping actually did exist, if not, return with not found
	count, err := result.RowsAffected()
	if err != nil {
		log.Printf("error checking rows affected: %+v\n", err)
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	if count == 0 {
		return fmt.Errorf("%w: %v", types.ErrNotFound, err)
	}

	// check if user is associated with atleast one group, if not, create a default
	stmt2, err := tx.Prepare("CALL GetUserOrganisations(?)")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt2.Close()
	rows, err := stmt2.Query(userId)
	if err != nil {
		log.Printf("error reading user groups: %+v\n", err)
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	defer rows.Close()

	// otherwise create a default group for the user
	if !rows.Next() {
		rows.Close()
		if err = repository.CreateOrganisationWithTx(tx, "My organisation", userId); err != nil {
			return err
		}
	}
	return nil
}

func (repository *CoreRepositoryImpl) CreateOrganisationWithTx(tx *sql.Tx, name string, userId string) error {

	// create organisation
	stmt1, err := tx.Prepare("INSERT INTO organisation (id, name) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("%w: error creating group: %v", types.ErrGenericSQL, err)
	}
	defer stmt1.Close()
	organisationId := uuid.NewString()
	if _, err := stmt1.Exec(organisationId, name); err != nil {
		return fmt.Errorf("%w: error inserting into organisation: %v", types.ErrGenericSQL, err)
	}

	// map user to organisation
	stmt2, err := tx.Prepare("INSERT INTO organisation_user (id, organisationId, userId) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrPrepareStatement, err)
	}
	defer stmt2.Close()
	if _, err = stmt2.Exec(uuid.NewString(), organisationId, userId); err != nil {
		return fmt.Errorf("%w: error inserting into organisation_user: %v", types.ErrGenericSQL, err)
	}

	// create group owner role for the group
	if err := repository.role.CreateGroupOwnerRole(tx, organisationId, userId); err != nil {
		log.Printf("create owner role error: %+v\n", err)
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}

	return nil
}
