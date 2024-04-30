package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
	"user.service.altiore.io/service"
)

type TokenHandler struct {
	core     *repository.CoreRepository
	token    *service.TokenService
	firebase *service.FirebaseService
}

func NewTokenHandler() *TokenHandler {
	return &TokenHandler{
		core:     repository.NewCoreRepository(),
		token:    service.NewTokenService(),
		firebase: service.NewFirebaseService(),
	}
}

func (handler *TokenHandler) RegisterRoutes(router *gin.Engine) {
	router.POST("/api/token/verify", handler.verify)
}

// Verify a user's token.
func (handler *TokenHandler) verify(c *gin.Context) {

	// parse and validate body
	var body struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// decode token
	decodedToken, err := handler.firebase.VerifyToken(body.Token)
	if err != nil {
		log.Println("invalid token according to firebase")
		c.String(http.StatusForbidden, "invalid token")
		return
	}

	// check user exists in db(?)
	if err := handler.core.UserExists(decodedToken.UID); err != nil {
		log.Println("user not found in database")
		c.String(http.StatusBadRequest, "user does not exist")
		return
	}

	// send response
	c.Status(http.StatusOK)
}
