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

var (
	core_once     sync.Once
	core_instance *CoreRepository
)

type CoreRepository struct {
	client   *sql.DB
	firebase *service.FirebaseService
}

var (
	USER_NOT_FOUND = "User not found"
)

func NewCoreRepository() *CoreRepository {
	core_once.Do(func() {

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

		core_instance = &CoreRepository{
			client:   db,
			firebase: service.NewFirebaseService(),
		}
	})

	return core_instance
}

func (repository *CoreRepository) NewTransaction(ctx context.Context) (*sql.Tx, error) {
	return repository.client.BeginTx(ctx, &sql.TxOptions{})
}

// Updates the group's name.
func (repository *CoreRepository) UpdateGroupName(groupId string, name string) error {
	return repository.UpdateGroupNameWithTx(nil, groupId, name)
}

// Updates the group's name.
func (repository *CoreRepository) UpdateGroupNameWithTx(tx *sql.Tx, groupId string, name string) error {
	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}
	stmt, err := c.Prepare("UPDATE organisation SET name = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	if _, err := stmt.Exec(name, groupId); err != nil {
		return err
	}
	return nil
}

// Deletes the group and all associations.
func (repository *CoreRepository) DeleteGroup(groupId string) error {
	return repository.DeleteGroupWithTx(nil, groupId)
}

// Deletes the group and all associations.
func (repository *CoreRepository) DeleteGroupWithTx(tx *sql.Tx, groupId string) error {
	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}
	stmt, err := c.Prepare("CALL GroupCleanup(?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	if _, err := stmt.Exec(groupId); err != nil {
		return err
	}
	return nil
}

// Updates the password for a user.
func (repository *CoreRepository) UpdatePassword(uid string, password string) error {
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

func (repository *CoreRepository) Login(uid string, email string, password string) error {
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

func (repository *CoreRepository) Signup(userId string, name string) error {
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
func (repository *CoreRepository) ReadUserByEmail(email string) (*types.User, error) {
	stmt, err := repository.client.Prepare("SELECT id, email FROM user WHERE email = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var user types.User
	if err := stmt.QueryRow(email).Scan(&user.Id, &user.Email); err != nil {
		return nil, err
	}

	return &user, nil
}

// Allow the user to verify their account by link in mail.
func (repository *CoreRepository) VerifyUser(userId string) error {
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
func (repository *CoreRepository) CreateUser(tx *sql.Tx, userId string, name string) error {
	return repository.CreateUserWithTx(nil, userId, name, "", "")
}

func (repository *CoreRepository) CreateUserWithTx(tx *sql.Tx, userId string, name string, email string, password string) error {
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

func (repository *CoreRepository) UserExists(uid string) error {
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

func (repository *CoreRepository) ReadServices() ([]*types.Service, error) {
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

func (repository *CoreRepository) ImplementationGroupCount(serviceName string) (int, error) {
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

func (repository *CoreRepository) RegisterUsedService(serviceName string, implementationGroup *int, organisationId string, userId string) error {
	return repository.RegisterUsedServiceWithTx(nil, serviceName, implementationGroup, organisationId, userId)
}

// Register a user has used a service.
func (repository *CoreRepository) RegisterUsedServiceWithTx(tx *sql.Tx, serviceName string, implementationGroup *int, organisationId string, userId string) error {

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
func (repository *CoreRepository) OrganisationList(userId string) ([]*types.Organisation, error) {

	// prepare query
	stmt, err := repository.client.Prepare("CALL GetUserOrganisations(?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	// execute query
	rows, err := stmt.Query(userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// parse the returned results
	var organisations []*types.Organisation
	for rows.Next() {
		var org types.Organisation
		if err := rows.Scan(&org.Id, &org.Name); err != nil {
			return nil, err
		}
		organisations = append(organisations, &org)
	}

	// check for other errors
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// return organisations
	return organisations, nil
}

// Get all members associated with an organisation.
func (repository *CoreRepository) ReadOrganisationMembers(id string) ([]*types.OrganisationMember, error) {

	// prepare query
	stmt, err := repository.client.Prepare("CALL GetOrganisationMembers(?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	// execute query
	result, err := stmt.Query(id)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	// parse the returned results
	var members []*types.OrganisationMember
	for result.Next() {
		var org types.OrganisationMember
		if err := result.Scan(&org.Id, &org.Name); err != nil {
			return nil, err
		}
		members = append(members, &org)
	}

	// check for other errors
	if err := result.Err(); err != nil {
		return nil, err
	}

	// return organisations
	return members, nil
}

// Create an invitation.
func (repository *CoreRepository) CreateInvitation(userId string, email string, groupId string) (string, error) {
	// identifier for the mapping between org and email
	id := uuid.NewString()
	stmt, err := repository.client.Prepare("INSERT INTO invitation (id, userId, email, organisationId) VALUES (?, ?, ?, ?)")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id, userId, email, groupId)
	if err != nil {
		return "", err
	}
	return id, nil
}

// Checks whether a user is already a part of the group.
func (repository *CoreRepository) IsUserAlreadyMember(userId string, groupId string) error {
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
func (repository *CoreRepository) ReadGroup(groupId string) (*types.Organisation, error) {
	stmt, err := repository.client.Prepare("SELECT * FROM organisation WHERE id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var group types.Organisation
	if err := stmt.QueryRow(groupId).Scan(&group.Id, &group.Name); err != nil {
		return nil, err
	}
	return &group, nil
}

// Looks up an invitation, ensuring the invitationId is intended for the email.
func (repository *CoreRepository) LookupInvitation(invitationId string) (string, string, error) {
	stmt, err := repository.client.Prepare("SELECT * FROM invitation WHERE id = ?")
	if err != nil {
		return "", "", types.ErrPrepareStatement
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
			return "", "", types.ErrInvitationNotFound
		}
		return "", "", types.ErrGenericSQL
	}
	return inv.userId, inv.orgId, nil
}

// Delete an invitation.
func (repository *CoreRepository) DeleteInvitation(id string) error {
	return repository.DeleteInvitationWithTx(nil, id)
}

func (repository *CoreRepository) DeleteInvitationWithTx(tx *sql.Tx, id string) error {
	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}
	stmt, err := c.Prepare("DELETE FROM invitation WHERE id = ?")
	if err != nil {
		return types.ErrPrepareStatement
	}
	defer stmt.Close()
	_, err = stmt.Exec(id)
	if err != nil {
		return types.ErrGenericSQL
	}
	return nil
}

func (repository *CoreRepository) AddUserToOrganisationWithTx(tx *sql.Tx, userId string, groupId string) error {
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

func (repository *CoreRepository) AddUserToOrganisation(userId string, organisationId string) error {
	return repository.AddUserToOrganisationWithTx(nil, userId, organisationId)
}

func (repository *CoreRepository) InvitationSignup(invitationId string, email string, password string, name string) error {

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
	userId, organisationId, err := repository.LookupInvitation(invitationId)
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
func (repository *CoreRepository) DeleteUser(userId string) error {
	return repository.DeleteInvitationWithTx(nil, userId)
}

// Cleanup method to delete everything associated with the userId (user and organisation relations).
func (repository *CoreRepository) DeleteUserWithTx(tx *sql.Tx, userId string) error {

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

// Non-tx method for removing a user from an organisation.
func (repository *CoreRepository) RemoveUserFromOrganisation(userId string, organisationId string) error {
	return repository.RemoveUserFromOrganisationWithTx(nil, userId, organisationId)
}

// Remove a user from an organisation, if user has no organisation left after removal, create a default one.
func (repository *CoreRepository) RemoveUserFromOrganisationWithTx(tx *sql.Tx, userId string, organisationId string) error {

	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}

	// delete from org
	stmt1, err := c.Prepare("DELETE FROM organisation_user WHERE userId = ? AND organisationId = ?")
	if err != nil {
		return types.ErrPrepareStatement
	}
	defer stmt1.Close()
	result, err := stmt1.Exec(userId, organisationId)
	if err != nil {
		return types.ErrGenericSQL
	}

	// check if the mapping actually did exist, if not, return with not found
	count, err := result.RowsAffected()
	if err != nil {
		return types.ErrGenericSQL
	}
	if count == 0 {
		return types.ErrNotFound
	}

	// if the mapping didn't exist at all, return here. allows for 404 response

	// check if user is associated with atleast one organisation, if not, create a default
	stmt2, err := c.Prepare("CALL GetUserOrganisations(?)")
	if err != nil {
		return types.ErrPrepareStatement
	}
	defer stmt2.Close()
	rows, err := stmt2.Query(userId)
	if err != nil {
		return types.ErrGenericSQL
	}
	defer rows.Close()

	// otherwise create a default organisation for the user
	if !rows.Next() {
		_tx, ok := c.(*sql.Tx)
		if !ok {
			return err
		}
		if err = repository.CreateOrganisationWithTx(_tx, "My organisation", userId); err != nil {
			return err
		}
	}

	return nil
}

func (repository *CoreRepository) CreateOrganisation(name string, userId string) error {
	return repository.CreateOrganisationWithTx(nil, name, userId)
}

func (repository *CoreRepository) CreateOrganisationWithTx(tx *sql.Tx, name string, userId string) error {

	var c types.Execer = repository.client
	if tx != nil {
		c = tx
	}

	// create organisation
	stmt1, err := c.Prepare("INSERT INTO organisation (id, name) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt1.Close()
	organisationId := uuid.NewString()
	if _, err = stmt1.Exec(organisationId, name); err != nil {
		return err
	}

	// map user to organisation
	stmt2, err := c.Prepare("INSERT INTO organisation_user (id, organisationId, userId) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt2.Close()
	if _, err = stmt2.Exec(uuid.NewString(), organisationId, userId); err != nil {
		return err
	}

	return nil
}
