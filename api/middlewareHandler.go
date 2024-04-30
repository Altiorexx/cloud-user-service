package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
	"user.service.altiore.io/service"
)

type MiddlewareHandler struct {
	core     *repository.CoreRepository
	firebase *service.FirebaseService
}

func NewMiddlewareHandler() *MiddlewareHandler {
	return &MiddlewareHandler{
		core:     repository.NewCoreRepository(),
		firebase: service.NewFirebaseService(),
	}
}

func (handler *MiddlewareHandler) RegisterRoutes(router *gin.Engine) {
	router.Use(handler.VerifyToken)
	// should also have a middleware to ensure only requests from recognized services go through.
}

// Verifies the token for every incoming request.
func (handler *MiddlewareHandler) VerifyToken(c *gin.Context) {

	// when an internal service sends a request, some kind of allowance should maybe
	// decided? allow origin or some secret key? -> services should not have the same
	// token verification that users has..
	//
	// this is a simple fix for now, that whitelists specific endpoints, requested by other services
	TOKEN_VERIFY_PATTERN, _ := regexp.Compile("/api/token/verify")
	if TOKEN_VERIFY_PATTERN.MatchString(c.Request.URL.Path) {
		c.Next()
		return
	}
	USER_EXISTS_PATTERN, _ := regexp.Compile("^/api/user/([a-zA-Z0-9]+)/exists$")
	if USER_EXISTS_PATTERN.MatchString(c.Request.URL.Path) {
		c.Next()
		return
	}
	REGISTER_SERVICE_USED_PATTERN, _ := regexp.Compile("/api/user/registerServiceUsed")
	if REGISTER_SERVICE_USED_PATTERN.MatchString(c.Request.URL.Path) {
		c.Next()
		return
	}

	SIGNUP_PATTERN, _ := regexp.Compile("/api/user/signup")
	if SIGNUP_PATTERN.MatchString(c.Request.URL.Path) {
		c.Next()
		return
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
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// check that user exists in our database
	if err := handler.core.UserExists(decodedToken.UID); err != nil {
		c.AbortWithStatus(http.StatusForbidden)
		handler.firebase.RevokeToken(decodedToken.UID)
		return
	}

	// set userId for request and continue
	c.Set("userId", decodedToken.UID)
	c.Next()
}
