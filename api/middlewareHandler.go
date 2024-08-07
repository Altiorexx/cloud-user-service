package api

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
	"user.service.altiore.io/service"
	"user.service.altiore.io/types"
)

type MiddlewareHandler interface {
	RegisterRoutes(*gin.Engine)
}

type MiddlewareHandlerOpts struct {
	Core     repository.CoreRepository
	Role     repository.RoleRepository
	Log      repository.LogRepository
	Firebase service.FirebaseService
	Token    service.TokenService
}

type MiddlewareHandlerImpl struct {
	core     repository.CoreRepository
	role     repository.RoleRepository
	log      repository.LogRepository
	firebase service.FirebaseService
	token    service.TokenService
	cache    map[string]*types.User

	exemptPaths   []*regexp.Regexp
	permissionMap map[string]string
}

func NewMiddlewareHandler(opts *MiddlewareHandlerOpts) *MiddlewareHandlerImpl {
	h := &MiddlewareHandlerImpl{
		core:     opts.Core,
		role:     opts.Role,
		log:      opts.Log,
		firebase: opts.Firebase,
		token:    opts.Token,
		cache:    make(map[string]*types.User),
		exemptPaths: []*regexp.Regexp{
			regexp.MustCompile("/api/token/verify"),
			regexp.MustCompile("^/api/user/([a-zA-Z0-9]+)/exists$"),
			regexp.MustCompile("/api/user/registerServiceUsed"),
			regexp.MustCompile("/api/user/signup"),
			regexp.MustCompile("/api/user/signup/email_password"),
			regexp.MustCompile("/api/user/login"),
			regexp.MustCompile("/api/user/start_password_reset"),
			regexp.MustCompile("/api/user/reset_password"),
			regexp.MustCompile("/api/group/join"),
		},
		permissionMap: map[string]string{

			"PATCH /api/group/:id/update":  "RenameGroup",
			"DELETE /api/group/:id/delete": "DeleteGroup",

			"POST /api/group/member/invite":   "InviteMember",
			"DELETE /api/group/member/remove": "RemoveMember",

			"": "",
			/*
				CREATE_CASE          = "CreateCase"
				UPDATE_CASE_METADATA = "UpdateCaseMetadata"
				DELETE_CASE          = "DeleteCase"
				EXPORT_CASE          = "ExportCase"

				VIEW_LOGS   = "ViewLogs"
				EXPORT_LOGS = "ExportLogs"
			*/
		},
	}
	go h.cacheFlushWorker()
	return h
}

func (handler *MiddlewareHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.Use(handler.verifyInternalServiceToken)
	router.Use(handler.verifyToken)
	router.Use(handler.checkPermission)
	router.Use(handler.logUserAction)
}

// Flushes the handler cache periodically.
func (handler *MiddlewareHandlerImpl) cacheFlushWorker() {
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

func (handler *MiddlewareHandlerImpl) verifyInternalServiceToken(c *gin.Context) {
	if token := c.GetHeader("X-Internal-Token"); token != "" {
		if err := handler.token.CheckToken(token); err != nil {
			log.Printf("internal token check resulted in error: %+v\n", err)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid token"})
			return
		}
		// set this to skip other middleware (they are user minded, not service minded)
		c.Set("internal-service", true)
	}
}

// Verifies the token for every incoming request.
func (handler *MiddlewareHandlerImpl) verifyToken(c *gin.Context) {

	// skip if it's a service request
	if c.GetBool("internal-service") {
		c.Next()
		return
	}

	// don't verify on specified paths
	for _, path := range handler.exemptPaths {
		if path.MatchString(c.Request.URL.Path) {
			c.Next()
			return
		}
	}

	// check if the authorization header is set
	authorization := c.GetHeader("Authorization")
	if authorization == "" {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("no Authorization header set"))
		return
	}

	// check if the authorization header format is correct
	if !strings.HasPrefix(authorization, "Bearer ") {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("incorrect authorization header format"))
		return
	}

	// extract token from header
	token := strings.TrimPrefix(authorization, "Bearer ")
	if token == "" {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("no token set in header"))
		return
	}

	// decode and verify token through firebase
	decodedToken, err := handler.firebase.VerifyToken(token)
	if err != nil {
		log.Printf("%+v\t%+v\n", decodedToken, err)
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// check that user exists in our database
	if err := handler.core.UserExists(decodedToken.UID); err != nil {
		println(err)
		c.AbortWithStatus(http.StatusForbidden)
		handler.firebase.RevokeToken(decodedToken.UID)
		return
	}

	// set userId for request and continue
	c.Set("userId", decodedToken.UID)
	c.Next()
}

func (handler *MiddlewareHandlerImpl) checkPermission(c *gin.Context) {

	// skip if it's a service request
	if c.GetBool("internal-service") {
		c.Next()
		return
	}

	// create a key and retrieve needed permission
	neededPermission, exists := handler.permissionMap[fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())]
	if !exists {
		// this means that the endpoint has no required perms, and therefore isn't a group-related endpoint either;
		// -> permissions are related to group user management, nothing else.
		// therefore it is most likely that the path includes a :id parameter, being the groupId
		c.Next()
		return
	}

	// set this, so other middleware can differ requests requiring perms
	c.Set("needsPermission", true)

	// ensure the 'id' path parameter exists in the path
	groupId, exists := c.Params.Get("id")
	if !exists {
		log.Printf("assumed groupId was present, but wasn't.\n")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	memberRoles, err := handler.role.ReadMemberRoles(c.GetString("userId"), groupId)
	if err != nil {
		log.Printf("error reading member roles: %+v\n", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// permission status is set for later use, so the logging handler can
	// register the request.
	c.Set("hasPermission", EvaluatePermission(memberRoles, neededPermission))
}

// Logs the request whenever a user has to be verified, for documentation purposes.
// This handler is a bit messy, final implementation is yet to be decided.
func (handler *MiddlewareHandlerImpl) logUserAction(c *gin.Context) {

	// skip if it's a service request
	if c.GetBool("internal-service") {
		c.Next()
		return
	}

	// skip requests that doesn't need permission
	if !c.GetBool("needsPermission") {
		c.Next()
		return
	}

	// should access the permission state, and include it in the log entry
	hasPermission := c.GetBool("hasPermission")

	// evaluate what to do with the request
	// go next immediately, because the user should not be affected by this at all (good point?)
	switch hasPermission {
	case true:
		c.Next()
	case false:
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing permission"})
	}

	// only log events for group use cases, anything else is meaningless..
	groupId, exists := c.Params.Get("id")
	if !exists {
		return
	}

	// transform path to use case, end users are most interested in user actions (rename group, invite member etc)
	action, exists := handler.permissionMap[fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())]
	if !exists {
		action = c.FullPath()
	}

	// check if there was a userId bound to the request
	userId := c.GetString("userId")
	if userId == "" {
		userId = "None"
	}

	// get email by userId
	var email string
	user, exists := handler.cache[userId]
	if !exists {
		user, err := handler.core.ReadUserById(userId)
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
		GroupId:   groupId,
		Action:    action,
		Status:    status,
		UserId:    userId,
		Email:     email,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// checkPermission checks if a user has the necessary permission
func EvaluatePermission(roles []*types.Role, neededPermission string) bool {
	for _, role := range roles {
		switch neededPermission {
		case types.RENAME_GROUP:
			if role.RenameGroup {
				return true
			}
		case types.DELETE_GROUP:
			if role.DeleteGroup {
				return true
			}
		case types.INVITE_MEMBER:
			if role.InviteMember {
				return true
			}
		case types.REMOVE_MEMBER:
			if role.RemoveMember {
				return true
			}
		case types.CREATE_CASE:
			if role.CreateCase {
				return true
			}
		case types.UPDATE_CASE_METADATA:
			if role.UpdateCaseMetadata {
				return true
			}
		case types.DELETE_CASE:
			if role.DeleteCase {
				return true
			}
		case types.EXPORT_CASE:
			if role.ExportCase {
				return true
			}
		case types.VIEW_LOGS:
			if role.ViewLogs {
				return true
			}
		case types.EXPORT_LOGS:
			if role.ExportLogs {
				return true
			}
		}
	}
	return false
}
