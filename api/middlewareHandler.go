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
)

type MiddlewareHandler interface {
	RegisterRoutes(*gin.Engine)
}

type MiddlewareHandlerOpts struct {
	Firebase service.FirebaseService
}

type MiddlewareHandlerImpl struct {
	core        *repository.CoreRepository
	firebase    service.FirebaseService
	exemptPaths []*regexp.Regexp
}

func NewMiddlewareHandler(opts *MiddlewareHandlerOpts) *MiddlewareHandlerImpl {
	return &MiddlewareHandlerImpl{
		core:     repository.NewCoreRepository(),
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
	}
}

func (handler *MiddlewareHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.Use(handler.VerifyToken)
	// router.Use(handler.LogUserAction)
	// should also have a middleware to ensure only requests from recognized services go through.
}

// Logs the request whenever a user has to be verified, for documentation purposes.
func (handler *MiddlewareHandlerImpl) LogUserAction(c *gin.Context) {

	c.Next()

	// check that the request is business relevant.
	// E.g. viewing available services is not important to log, but creating a new case is.

	// store somewhere

	// go next immediately, because the user should not be affected by this at all

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
