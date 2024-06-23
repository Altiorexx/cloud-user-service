package api

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
	"user.service.altiore.io/service"
	"user.service.altiore.io/types"
)

type UserHandler interface {
	RegisterRoutes(*gin.Engine)
}

type UserHandlerOpts struct {
	Core     repository.CoreRepository
	Firebase service.FirebaseService
	Email    service.EmailService
}

type UserHandlerImpl struct {
	core          repository.CoreRepository
	firebase      service.FirebaseService
	email         service.EmailService
	portal_domain string
	domain        string
}

func NewUserHandler(opts *UserHandlerOpts) *UserHandlerImpl {
	return &UserHandlerImpl{
		core:          opts.Core,
		firebase:      opts.Firebase,
		email:         opts.Email,
		portal_domain: os.Getenv("PORTAL_DOMAIN"),
		domain:        os.Getenv("DOMAIN"),
	}
}

func (handler *UserHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.GET("/api/user/:userId/exists", handler.userExists)
	router.POST("/api/user/registerServiceUsed", handler.registerServiceUsed)

	router.POST("/api/user/login", handler.login)
	router.POST("/api/user/signup", handler.signup_PROVIDER)
	router.POST("/api/user/signup/email_password", handler.signup_EMAIL_PASSWORD)
	router.GET("/api/user/signup/verify", handler.SignupVerify)

	router.POST("/api/user/start_password_reset", handler.startPasswordReset)
	router.POST("/api/user/reset_password", handler.resetPassword)
}

func (handler *UserHandlerImpl) login(c *gin.Context) {
	var body struct {
		UID      string `json:"uid" binding:"required"`
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := handler.core.Login(body.UID, body.Email, body.Password); err != nil {
		log.Printf("error logging in: %+v\n", err)
		switch {
		case errors.Is(err, types.ErrUserNotVerified):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user is not verified"})
			return
		case errors.Is(err, types.ErrInvalidPassword):
			c.JSON(http.StatusNotFound, gin.H{"error": "invalid credentials"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.Status(http.StatusOK)
}

func (handler *UserHandlerImpl) startPasswordReset(c *gin.Context) {

	var body struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := types.Validate.Struct(body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// check user with email exists, only in our system, firebase emails are not relevant (we shouldnt have to reset google, microsoft email passwords!)
	user, err := handler.core.ReadUserByEmail(body.Email)
	if err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}

	// send email
	link := fmt.Sprintf("%s/reset?u=%s", handler.portal_domain, user.Id)
	if err := handler.email.Send([]string{body.Email}, handler.email.CreateResetPassword(body.Email, link)); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Status(http.StatusOK)
}

func (handler *UserHandlerImpl) resetPassword(c *gin.Context) {

	var body struct {
		UID         string `json:"uid" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := types.Validate.Struct(body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// check user exists with given uid
	if err := handler.core.UserExists(body.UID); err != nil {
		c.String(http.StatusNotFound, "user not found")
		return
	}

	// hash and update their password
	if err := handler.core.UpdatePassword(body.UID, body.NewPassword); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// update password in firebase
	if err := handler.firebase.SetNewPassword(body.UID, body.NewPassword); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	c.Status(http.StatusOK)
}

func (handler *UserHandlerImpl) SignupVerify(c *gin.Context) {

	// check userId exists (get by query param or smthing)
	userId := c.Query("u")

	// update user's verified field to true
	if err := handler.core.VerifyUser(userId); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// redirect to login
	c.Redirect(http.StatusPermanentRedirect, fmt.Sprintf("%s/login", handler.portal_domain))
}

// Sign up using email, password.
func (handler *UserHandlerImpl) signup_EMAIL_PASSWORD(c *gin.Context) {
	var body struct {
		UID          string  `json:"uid" binding:"required"`
		Email        string  `json:"email" binding:"required"`
		Password     string  `json:"password" binding:"required"`
		InvitationId *string `json:"invitationId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	err := handler.core.WithTransaction(c.Request.Context(), func(tx *sql.Tx) error {
		if err := handler.core.CreateUserWithTx(tx, body.UID, body.Email, body.Password); err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				return types.ErrUserAlreadyExists
			} else {
				log.Printf("error occured while creating user: %+v\n", err)
				return err
			}
		}
		// create default group and map user to it
		if err := handler.core.CreateOrganisationWithTx(tx, "My Group", body.UID); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("error signing up: %+v\n", err)
		switch {
		case errors.Is(err, types.ErrUserAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	// send verification email
	go func() {
		if err := handler.email.Send([]string{body.Email}, handler.email.CreateSignupVerification(body.Email, fmt.Sprintf("%s/api/user/signup/verify?u=%s", handler.domain, body.UID))); err != nil {
			log.Printf("error sending verification email to %s\n", body.Email)
		}
	}()
	c.Status(http.StatusCreated)

}

// Signup using a third party provider, Google, Microsoft etc.
func (handler *UserHandlerImpl) signup_PROVIDER(c *gin.Context) {
	var body struct {
		UID   string `json:"uid" binding:"required"`
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := handler.core.WithTransaction(c.Request.Context(), func(tx *sql.Tx) error {
		if err := handler.core.CreateUserWithTx(tx, body.UID, body.Email, "dawoidjawodijawodijawodijawdoaidoawijda120ei12090#01310"); err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				return types.ErrUserAlreadyExists
			} else {
				log.Printf("error occured while creating user: %+v\n", err)
				return err
			}
		}
		// create default group and map user to it
		if err := handler.core.CreateOrganisationWithTx(tx, "My Group", body.UID); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("error signing up: %+v\n", err)
		switch {
		case errors.Is(err, types.ErrUserAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.Status(http.StatusCreated)
}

// Checks whether a user exists in database.
func (handler *UserHandlerImpl) userExists(c *gin.Context) {
	if err := handler.core.UserExists(c.Param("userId")); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

// Logs when a user uses a service, is triggered by create case.
func (handler *UserHandlerImpl) registerServiceUsed(c *gin.Context) {

	// parse body
	var body *types.RegisterServiceUsedBody
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Println(err)
		c.Status(http.StatusBadRequest)
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
