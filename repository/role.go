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
	CreateRole(tx *sql.Tx, role *types.Role) error
	ReadRoles()
	DeleteRole()
	UpdateRole()
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

// Maybe create the frontend first -> this current approach seems wrong,
// as the user should not 'create' a role, but rather press new role and then
// click save, which then should attempt to update a role and if it doesn't exist,
// the service should interpret this as creating a new role. (?)

func (repository *RoleRepositoryImpl) CreateRole(tx *sql.Tx, role *types.Role) error {
	stmt, err := tx.Prepare("INSERT INTO roles VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? )")
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	defer stmt.Close()
	id := uuid.NewString()
	if _, err := stmt.Exec(id, role.Name, role.GroupId, role.RenameGroup, role.DeleteGroup, "... rest of roles here..?"); err != nil {
		return fmt.Errorf("%w: %v", types.ErrGenericSQL, err)
	}
	return nil
}

func (repository *RoleRepositoryImpl) ReadRoles()  {}
func (repository *RoleRepositoryImpl) DeleteRole() {}
func (repository *RoleRepositoryImpl) UpdateRole() {}
