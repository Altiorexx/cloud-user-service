package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
	"user.service.altiore.io/service"
	"user.service.altiore.io/types"
)

type UserHandler struct {
	core     *repository.CoreRepository
	token    *service.TokenService
	firebase *service.FirebaseService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		core:     repository.NewCoreRepository(),
		token:    service.NewTokenService(),
		firebase: service.NewFirebaseService(),
	}
}

func (handler *UserHandler) RegisterRoutes(router *gin.Engine) {
	router.GET("/api/user/:userId/exists", handler.userExists)
	router.POST("/api/user/registerServiceUsed", handler.registerServiceUsed)

	router.POST("/api/user/signup", handler.signup)
}

// Checks whether a user exists in database.
func (handler *UserHandler) userExists(c *gin.Context) {
	if err := handler.core.UserExists(c.Param("userId")); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

func (handler *UserHandler) signup(c *gin.Context) {

	// parse and validate body
	var body struct {
		Email        string `json:"email" binding:"required"`
		Password     string `json:"password" binding:"required"`
		Name         string `json:"name" binding:"required"`
		InvitationId string `json:"invitationId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Println(err)
		c.Status(http.StatusBadRequest)
		return
	}
	if err := types.Validate.Struct(body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// Sign up user by invitation
	if err := handler.core.InvitationSignup(body.InvitationId, body.Email, body.Password, body.Name); err != nil {
		switch err {

		case types.ErrInvitationNotFound:
			c.String(http.StatusForbidden, "no invitation found")

		case types.ErrPrepareStatement, types.ErrRollback, types.ErrTxCancelled, types.ErrGenericSQL, types.ErrTxCommit:
			c.String(http.StatusInternalServerError, "database error")

		default:
			log.Printf("unhandled error in FullSignup: %+v\n", err)
			c.String(http.StatusInternalServerError, err.Error())
		}
		return
	}

	// send response
	c.Status(http.StatusOK)
}

// Logs when a user uses a service, is triggered by create case.
func (handler *UserHandler) registerServiceUsed(c *gin.Context) {

	// parse body
	var body *types.RegisterServiceUsedBody
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Println(err)
		c.Status(http.StatusBadRequest)
		return
	}

	// validate body
	if err := types.Validate.Struct(body); err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, err)
		return
	}

	// register used services
	if err := handler.core.RegisterUsedService(body.ServiceName, body.ImplementationGroup, body.OrganisationId, body.UserId); err != nil {
		log.Println(err)
		c.Status(http.StatusForbidden)
		return
	}

	// send response to client
	c.Status(http.StatusOK)
}
