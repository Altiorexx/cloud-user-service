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
	"user.service.altiore.io/types"
)

type LogRepository interface {
	NewEntry(entry *types.LogEntry)
	ReadByGroupId(groupId string) (any, error)
}

type LogRepositoryImpl struct {
	client    *sql.DB
	entryChan chan *types.LogEntry
}

type LogRepositoryOpts struct {
	Key string
}

var (
	log_repository_instance_map = make(map[string]*LogRepositoryImpl)
	log_mu                      sync.Mutex
)

func NewLogRepository(opts *LogRepositoryOpts) *LogRepositoryImpl {
	log_mu.Lock()
	defer log_mu.Unlock()
	if instance, exists := log_repository_instance_map[opts.Key]; exists {
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

	log_repository_instance_map[opts.Key] = &LogRepositoryImpl{
		client:    db,
		entryChan: make(chan *types.LogEntry), // set a buffer on this when going to prod, reduces the log load (but not too high, in case of errors and lost entries)
	}

	for i := 0; i < 5; i++ {
		go log_repository_instance_map[opts.Key].write_worker()
	}

	return log_repository_instance_map[opts.Key]
}

// Creates and stores a new log entry.
func (repository *LogRepositoryImpl) NewEntry(entry *types.LogEntry) {
	repository.entryChan <- entry
}

func (repository *LogRepositoryImpl) write_worker() {
	defer log.Println("log write worker stopped!")
	stmt, err := repository.client.Prepare("INSERT INTO log VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		log.Printf("write worker error: %+v\n", err)
	}
	defer stmt.Close()
	for entry := range repository.entryChan {
		if _, err := stmt.Exec(entry.GroupId, entry.Action, entry.Status, entry.UserId, entry.Email, entry.Timestamp); err != nil {
			log.Printf("error writing log entry: %+v\n", err)
		}
	}
}

// Get logs by group id.
func (repository *LogRepositoryImpl) ReadByGroupId(groupId string) (any, error) {
	return nil, nil
}
