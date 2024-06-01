package api

import (
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

type UserHandler struct {
	core          *repository.CoreRepository
	token         *service.TokenService
	firebase      *service.FirebaseService
	email         *service.EmailService
	portal_domain string
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		core:          repository.NewCoreRepository(),
		token:         service.NewTokenService(),
		firebase:      service.NewFirebaseService(),
		email:         service.NewEmailService(),
		portal_domain: os.Getenv("PORTAL_DOMAIN"),
	}
}

func (handler *UserHandler) RegisterRoutes(router *gin.Engine) {
	router.GET("/api/user/:userId/exists", handler.userExists)
	router.POST("/api/user/registerServiceUsed", handler.registerServiceUsed)

	router.POST("/api/user/login", handler.login)
	router.POST("/api/user/signup", handler.signup_PROVIDER)
	router.POST("/api/user/signup/email_password", handler.signup_EMAIL_PASSWORD)
	router.GET("/api/user/signup/verify", handler.SignupVerify)

	router.POST("/api/user/start_password_reset", handler.startPasswordReset)
	router.POST("/api/user/reset_password", handler.resetPassword)
}

func (handler *UserHandler) login(c *gin.Context) {

	var body struct {
		UID      string `json:"uid" binding:"required"`
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := types.Validate.Struct(body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// check user exists with the things provided
	if err := handler.core.Login(body.UID, body.Email, body.Password); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	c.Status(http.StatusOK)
}

func (handler *UserHandler) startPasswordReset(c *gin.Context) {

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

func (handler *UserHandler) resetPassword(c *gin.Context) {

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

func (handler *UserHandler) SignupVerify(c *gin.Context) {

	// check userId exists (get by query param or smthing)
	userId := c.Query("u")

	// update user's verified field to true
	if err := handler.core.VerifyUser(userId); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// redirect to login
	c.Redirect(http.StatusPermanentRedirect, "http://localhost:3000/login")
}

// Sign up using email, password.
func (handler *UserHandler) signup_EMAIL_PASSWORD(c *gin.Context) {

	var body struct {
		UID      string `json:"uid" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := types.Validate.Struct(body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// start new db transaction
	tx, err := handler.core.NewTransaction(c)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	defer func() {

		// rollback in case of panic
		if r := recover(); r != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, fmt.Sprintf("Internal server error: %v", r))
			log.Printf("(signup) panic: %+v\n", r)
			return
		}

		// rollback in case of error during transaction,
		// this also includes removing the user from firebase
		if err != nil {
			tx.Rollback()
			handler.firebase.DeleteUser(body.UID)
			c.String(http.StatusInternalServerError, fmt.Sprintf("Error processing request: %s", err.Error()))
			log.Printf("(signup) error during transaction: %+v\n", err)
			return
		}

	}()

	// create user
	if err := handler.core.CreateUserWithTx(tx, body.UID, body.Name, body.Email, body.Password); err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			log.Println("user already exists")
			c.Status(http.StatusConflict)
			return
		} else {
			log.Printf("error occured while creating user: %+v\n", err)
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	}

	// create organisation and map user to it
	if err := handler.core.CreateOrganisationWithTx(tx, "My Group", body.UID); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// try to commit
	if err := tx.Commit(); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("error committing transaction: %s", err.Error()))
		log.Printf("(signup) error committing transaction: %+v\n", err)
		return
	}

	// send verification email
	if err := handler.email.Send([]string{body.Email}, handler.email.CreateSignupVerification(body.Email, fmt.Sprintf("http://localhost:4000/api/user/signup/verify?u=%s", body.UID))); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// send response
	c.Status(http.StatusCreated)
}

// Signup using a third party provider, Google, Microsoft etc.
func (handler *UserHandler) signup_PROVIDER(c *gin.Context) {

	// parse and validate body
	var body struct {
		UID   string `json:"uid" binding:"required"`
		Name  string `json:"name"`
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

	// start new db transaction
	tx, err := handler.core.NewTransaction(c)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	defer func() {

		// rollback in case of panic
		if r := recover(); r != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, fmt.Sprintf("Internal server error: %v", r))
			log.Printf("(signup) panic: %+v\n", r)
			return
		}

		// rollback in case of error during transaction
		if err != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, fmt.Sprintf("Error processing request: %s", err.Error()))
			log.Printf("(signup) error during transaction: %+v\n", err)
			return
		}

	}()

	// create user
	if err := handler.core.CreateUserWithTx(tx, body.UID, body.Name, body.Email, ""); err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			log.Println("user already exists")
			c.Status(http.StatusConflict)
			return
		} else {
			log.Printf("error occured while creating user: %+v\n", err)
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	}

	// create organisation and map user to it
	if err := handler.core.CreateOrganisationWithTx(tx, "My Group", body.UID); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// try to commit
	if err := tx.Commit(); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("error committing transaction: %s", err.Error()))
		log.Printf("(createUser) error committing transaction: %+v\n", err)
		return
	}

	// send response
	c.Status(http.StatusCreated)
}

// Checks whether a user exists in database.
func (handler *UserHandler) userExists(c *gin.Context) {
	if err := handler.core.UserExists(c.Param("userId")); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

/* ///////// This method has signup + invitation logic, steal from here..
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
}*/

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
