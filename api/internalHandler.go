package api

import "github.com/gin-gonic/gin"

type InternalHandler interface {
	RegisterRoutes(router *gin.Engine)
}

type InternalHandlerImpl struct{}
type InternalHandlerOpts struct{}

func NewInternalHandler() InternalHandler {
	return &InternalHandlerImpl{}
}

func (handler *InternalHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.POST("/api/internal/check_user", handler.checkUser)
}

// Checks the user is OK with respect to their token (firebase) and the requested action (permission).
func (handler *InternalHandlerImpl) checkUser(c *gin.Context) {

	// check token
	// -> check token using firebase service

	// check permissions
	// -> expect method, path and map to permission

}
