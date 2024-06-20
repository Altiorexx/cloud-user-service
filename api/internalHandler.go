package api

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
	"user.service.altiore.io/service"
	"user.service.altiore.io/types"
)

type InternalHandler interface {
	RegisterRoutes(router *gin.Engine)
}

type InternalHandlerImpl struct {
	core          repository.CoreRepository
	role          repository.RoleRepository
	log           repository.LogRepository
	firebase      service.FirebaseService
	cache         map[string]*types.User
	permissionMap map[string]string
}

type InternalHandlerOpts struct {
	Core     repository.CoreRepository
	Role     repository.RoleRepository
	Log      repository.LogRepository
	Firebase service.FirebaseService
}

func NewInternalHandler(opts *InternalHandlerOpts) InternalHandler {
	h := &InternalHandlerImpl{
		core:     opts.Core,
		role:     opts.Role,
		log:      opts.Log,
		firebase: opts.Firebase,
		cache:    make(map[string]*types.User),
		permissionMap: map[string]string{
			"/api/case/cis18/create": "CreateCase",
			"/api/case/nis2/create":  "CreateCase",

			"/api/case/updateMetadata": "UpdateCaseMetadata",
			"/api/case/delete":         "DeleteCase",
		},
	}
	go h.cacheFlushWorker()
	return h
}

func (handler *InternalHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.POST("/api/internal/check_user", handler.checkUser)
	router.POST("/api/internal/strict_check_user", handler.strictCheckUser)
}

// Flushes the handler cache periodically.
func (handler *InternalHandlerImpl) cacheFlushWorker() {
	log.Println("middlware cache flush worker started.")
	ticker := time.NewTicker(time.Minute * 30)
	defer func() {
		ticker.Stop()
		log.Println("middleware cache flush worker stopped.")
	}()
	for {
		<-ticker.C
		handler.cache = make(map[string]*types.User)
	}
}

func (handler *InternalHandlerImpl) checkUser(c *gin.Context) {
	var body struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if _, err := handler.firebase.VerifyToken(body.Token); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

// Checks the user is OK with respect to their token (firebase) and the requested action (permission).
func (handler *InternalHandlerImpl) strictCheckUser(c *gin.Context) {
	var body struct {
		Token   string `json:"token" binding:"required"`
		GroupId string `json:"groupId" binding:"required"`
		Action  string `json:"action" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, err.Error())
		return
	}

	// check token
	// -> check token using firebase service
	decodedToken, err := handler.firebase.VerifyToken(body.Token)
	if err != nil {
		log.Printf("%+v\t%+v\n", decodedToken, err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// check that user exists in our database
	if err := handler.core.UserExists(decodedToken.UID); err != nil {
		println(err)
		c.AbortWithStatus(http.StatusForbidden)
		handler.firebase.RevokeToken(decodedToken.UID)
		return
	}

	// check permissions
	// if no permission is needed for the action, dont do anything..
	action, exists := handler.permissionMap[c.FullPath()]
	if !exists {
		c.Status(http.StatusOK)
		return
	}
	memberRoles, err := handler.role.ReadMemberRoles(decodedToken.UID, body.GroupId)
	if err != nil {
		log.Printf("error reading member roles: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if !EvaluatePermission(memberRoles, action) {
		log.Printf("user doesnt have permission for %s\n", action)
		c.JSON(http.StatusForbidden, gin.H{"error": "missing permissions"})
		return
	}

	// log entry here

	// get email by userId
	var email string
	user, exists := handler.cache[decodedToken.UID]
	if !exists {
		user, err := handler.core.ReadUserById(decodedToken.UID)
		if err != nil {
			log.Printf("error reading user by id to get mail for logging: %+v\n", err)
			// Set a default value in case of error
			email = "Error reading email"
		} else {
			email = user.Email
		}
	} else {
		email = user.Email
	}

	// Transform status code to business comprehendable
	var status string
	switch c.Writer.Status() {
	case http.StatusOK:
		status = "OK"
	case http.StatusInternalServerError:
		status = "Error"
	case http.StatusForbidden:
		status = "Forbidden"
	case http.StatusConflict:
		status = "OK"
	case http.StatusBadRequest:
		status = "Error"
	case http.StatusUnauthorized:
		status = "Unauthorized"
	}

	handler.log.NewEntry(&types.LogEntry{
		GroupId:   body.GroupId,
		Action:    action,
		Status:    status,
		UserId:    decodedToken.UID,
		Email:     email,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	c.Status(http.StatusOK)
}
