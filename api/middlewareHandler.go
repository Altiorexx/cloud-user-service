package api

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

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
	Firebase service.FirebaseService
}

type MiddlewareHandlerImpl struct {
	core          repository.CoreRepository
	role          repository.RoleRepository
	firebase      service.FirebaseService
	exemptPaths   []*regexp.Regexp
	permissionMap map[string]string
}

func NewMiddlewareHandler(opts *MiddlewareHandlerOpts) *MiddlewareHandlerImpl {
	return &MiddlewareHandlerImpl{
		core:     opts.Core,
		role:     opts.Role,
		firebase: opts.Firebase,
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
			"POST /api/group/:id/update": "RenameGroup",
		},
	}
}

func (handler *MiddlewareHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.Use(handler.VerifyToken)
	//router.Use(handler.checkPermission)
	//router.Use(handler.LogUserAction)
	// should also have a middleware to ensure only requests from recognized services go through.
}

// Verifies the token for every incoming request.
func (handler *MiddlewareHandlerImpl) VerifyToken(c *gin.Context) {

	// when an internal service sends a request, some kind of allowance should maybe
	// decided? allow origin or some secret key? -> services should not have the same
	// token verification that users has..

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

	// create a key and retrieve needed permission
	neededPermission, exists := handler.permissionMap[fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())]
	if !exists {
		// this means that the endpoint has no required perms, and therefore isn't a group-related endpoint either;
		// -> permissions are related to group user management, nothing else.
		// therefore it is most likely that the path includes a :id parameter, being the groupId
		c.Next()
		return
	}

	// ensure the 'id' path parameter exists in the path
	groupId, exists := c.Params.Get(":id")
	if !exists {
		log.Printf("assumed groupId was present, but wasn't.\n")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	memberRoles, err := handler.role.ReadMemberRoles(c.GetString("userId"), groupId)
	if err != nil {
		log.Printf("error reading member roles: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.Set("hasPermission", checkPermission(memberRoles, neededPermission))
}

// Logs the request whenever a user has to be verified, for documentation purposes.
func (handler *MiddlewareHandlerImpl) LogUserAction(c *gin.Context) {

	// should access the permission state, and include it in the log entry
	hasPermission := c.GetBool("hasPermission")

	// evaluate what to do with the request
	// go next immediately, because the user should not be affected by this at all (good point?)
	switch hasPermission {
	case true:
		c.Next()
	case false:
		c.JSON(http.StatusForbidden, gin.H{"error": "missing permission"})
	}

	// check that the request is business relevant.
	// E.g. viewing available services is not important to log, but creating a new case is.

	// store somewhere

}

// checkPermission checks if a user has the necessary permission
func checkPermission(roles []*types.Role, neededPermission string) bool {
	for _, role := range roles {
		switch neededPermission {
		case types.RENAME_GROUP:
			if role.RenameGroup {
				return true
			}
		case types.DELETE_CASE:
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
