package api

import (
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"os"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
	"user.service.altiore.io/service"
	"user.service.altiore.io/types"
)

type OrganisationHandler struct {
	core     *repository.CoreRepository
	firebase *service.FirebaseService
	token    *service.TokenService
	case_    *service.CaseService
	email    *service.EmailService
	domain   string
}

func NewOrganisationHandler() *OrganisationHandler {
	return &OrganisationHandler{
		core:     repository.NewCoreRepository(),
		firebase: service.NewFirebaseService(),
		token:    service.NewTokenService(),
		case_:    service.NewCaseService(),
		email:    service.NewEmailService(),
		domain:   os.Getenv("DOMAIN"),
	}
}

func (handler *OrganisationHandler) RegisterRoutes(router *gin.Engine) {

	router.POST("/api/organisation/create", handler.createOrganisation)
	router.GET("/api/organisation/list", handler.organisationList)

	router.DELETE("/api/group/:id/delete", handler.deleteGroup)

	router.GET("/api/organisation/:id/roles", handler.getRoles)

	router.GET("/api/organisation/:id/members", handler.members)

	router.POST("/api/organisation/member/invite", handler.inviteMember)
	router.DELETE("/api/organisation/member/remove", handler.removeMember)

}

// Delete a group and related data.
func (handler *OrganisationHandler) deleteGroup(c *gin.Context) {
	groupId := c.Param("id")

	// should check whether the user has permission to delete before anything

	if err := handler.core.DeleteGroup(groupId); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusOK)
}

func (handler *OrganisationHandler) getRoles(c *gin.Context) {

	orgId := c.Param("id")
	if orgId == "" {
		c.String(http.StatusBadRequest, "no id")
		return
	}

	// query db for
	// 1. defined roles for the org
	// 2. roles given for each individual user in the org

	c.Status(http.StatusOK)

}

// Create a group and adds the requesting user to it.
func (handler *OrganisationHandler) createOrganisation(c *gin.Context) {

	var body struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := types.Validate.Struct(body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// start transaction
	tx, err := handler.core.NewTransaction(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("(create group) error on commit: %+v\n", err)
			if err := tx.Rollback(); err != nil {
				log.Printf("(create group) error on rollback: %+v\n", err)
			}
		}
	}()

	// create org and add user to it
	userId, _ := c.Get("userId")
	if err := handler.core.CreateOrganisationWithTx(tx, body.Name, userId.(string)); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// commit transaction
	if err := tx.Commit(); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		panic(err)
	}

	c.Status(http.StatusOK)
}

func (handler *OrganisationHandler) members(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.String(http.StatusBadRequest, "empty id path parameter")
		return
	}
	members, err := handler.core.ReadOrganisationMembers(id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, members)
}

func (handler *OrganisationHandler) organisationList(c *gin.Context) {

	// retrieve token
	userId, exists := c.Get("userId")
	if !exists {
		c.Status(http.StatusForbidden)
		return
	}

	// get organisations user is associated with
	organisationList, err := handler.core.OrganisationList(userId.(string))
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// send response
	c.JSON(http.StatusOK, organisationList)
}

func (handler *OrganisationHandler) inviteMember(c *gin.Context) {

	// parse and validate body
	var body struct {
		Email          string `json:"email" binding:"required"`
		OrganisationId string `json:"organisationId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil {
		log.Println("tried to register using a bad email.")
		c.String(http.StatusBadRequest, "invalid mail")
		return
	}

	// generate link
	invitationId, err := handler.core.CreateInvitation(body.Email, body.OrganisationId)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	link := fmt.Sprintf("%s/signup?inv=%s", handler.domain, invitationId)

	// generate template and send mail
	message := handler.email.CreateInvitationMail(body.Email, link)
	if err := handler.email.Send([]string{body.Email}, message); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// send response
	c.Status(http.StatusOK)
}

func (handler *OrganisationHandler) removeMember(c *gin.Context) {

	// parse body
	var body struct {
		UserId         string `json:"userId" binding:"required"`
		OrganisationId string `json:"organisationId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	if err := handler.core.RemoveUserFromOrganisation(body.UserId, body.OrganisationId); err != nil {
		switch err {
		case types.ErrNotFound:
			c.String(http.StatusNotFound, err.Error())
			return
		default:
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	}

	c.Status(http.StatusOK)
}
