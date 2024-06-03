package api

import (
	"context"
	"errors"
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

type GroupHandler interface {
	RegisterRoutes(c *gin.Engine)
}

type GroupHandlerOpts struct {
	Core     repository.CoreRepository
	Firebase service.FirebaseService
	Email    service.EmailService
}

type GroupHandlerImpl struct {
	core          repository.CoreRepository
	case_         *service.CaseService
	email         service.EmailService
	firebase      service.FirebaseService
	domain        string
	portal_domain string
}

func NewGroupHandler(opts *GroupHandlerOpts) *GroupHandlerImpl {
	return &GroupHandlerImpl{
		core:          opts.Core,
		firebase:      opts.Firebase,
		case_:         service.NewCaseService(),
		email:         opts.Email,
		domain:        os.Getenv("DOMAIN"),
		portal_domain: os.Getenv("PORTAL_DOMAIN"),
	}
}

func (handler *GroupHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.POST("/api/group/create", handler.createOrganisation)
	router.GET("/api/group/list", handler.organisationList)
	router.GET("/api/group/:id", handler.getGroup)
	router.PATCH("/api/group/:id/update", handler.updateMetadata)
	router.DELETE("/api/group/:id/delete", handler.deleteGroup)
	router.GET("/api/group/:id/members", handler.members)
	router.POST("/api/group/member/invite", handler.inviteMember)
	router.GET("/api/group/join", handler.joinGroup)

	router.DELETE("/api/organisation/member/remove", handler.removeMember)
	router.GET("/api/organisation/:id/roles", handler.getRoles)
}

// Gets a group's metadata.
func (handler *GroupHandlerImpl) getGroup(c *gin.Context) {
	ctx := c.Request.Context()
	groupId := c.Param("id")
	group, err := handler.core.ReadGroup(ctx, groupId)
	if err != nil {
		log.Printf("failed to read group %s: %v\n", groupId, err)
		switch {
		case errors.Is(err, types.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, group)
}

func (handler *GroupHandlerImpl) updateMetadata(c *gin.Context) {
	ctx := c.Request.Context()
	groupId := c.Param("id")
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// start transactions
	tx, err := handler.core.NewTransaction(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// update name if requested
	if body.Name != "" {
		if err := handler.core.UpdateGroupNameWithTx(tx, groupId, body.Name); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
	}

	// commit changes
	if err := handler.core.CommitTransaction(tx); err != nil {
		log.Printf("failed to commit group metadata changes: %+v\n", err)
		switch {
		case errors.Is(err, types.ErrTxCommit):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		case errors.Is(err, types.ErrRollback):
			return
		}
	}

	c.Status(http.StatusOK)
}

// Delete a group and related data.
func (handler *GroupHandlerImpl) deleteGroup(c *gin.Context) {
	groupId := c.Param("id")

	// should check whether the user has permission to delete before anything

	if err := handler.core.DeleteGroup(groupId); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusOK)
}

func (handler *GroupHandlerImpl) getRoles(c *gin.Context) {

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
func (handler *GroupHandlerImpl) createOrganisation(c *gin.Context) {

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

func (handler *GroupHandlerImpl) members(c *gin.Context) {
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

func (handler *GroupHandlerImpl) organisationList(c *gin.Context) {

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

func (handler *GroupHandlerImpl) inviteMember(c *gin.Context) {

	// parse and validate body
	var body struct {
		UserId  string `json:"userId" binding:"required"`
		Email   string `json:"email" binding:"required"`
		GroupId string `json:"groupId" binding:"required"`
		Name    string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := types.Validate.Struct(body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil {
		log.Println("tried to invite using a bad email.")
		c.String(http.StatusBadRequest, "invalid mail")
		return
	}

	// attempt to get userId from firebase,
	// if the user doesn't exist, keep going, but make a signup invitation instead
	userId, err := handler.firebase.GetUserIdByEmail(body.Email)
	if err == nil && userId != "" {
		// if a user was found in firebase, check whether they are already a part of the group
		if err := handler.core.IsUserAlreadyMember(userId, body.GroupId); err != nil {
			c.String(http.StatusConflict, "user is already a member of the group")
			return
		}
	}

	// if no user was found in firebase, do a signup + invitation flow
	if userId == "" {
		log.Printf("%+v\n", body)
	}

	// if a user was found in firebase, do a simple invitation flow
	if userId != "" {

		// generate link
		invitationId, err := handler.core.CreateInvitation(userId, body.Email, body.GroupId)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		link := fmt.Sprintf("%s/api/group/join?inv=%s", handler.domain, invitationId)

		// generate template and send mail
		message := handler.email.CreateInvitationMail(body.Email, body.Name, link)
		if err := handler.email.Send([]string{body.Email}, message); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		// send response
		c.Status(http.StatusOK)

		// specify user to invite
		// create link including invitationId
		// user clicks link in email
		// points to user.service/api/group/join, which adds the user to the group and removes the invitation
		// then user is redirected to portal.app/invitation_ok (if things are ok, later inviter can retract the invite, but not now christ)

	}
}

func (handler *GroupHandlerImpl) joinGroup(c *gin.Context) {

	invitationId := c.Query("inv")
	if invitationId == "" {
		c.String(http.StatusBadRequest, "no invitation id found")
		return
	}

	userId, groupId, err := handler.core.LookupInvitation(invitationId)
	if err != nil {
		c.String(http.StatusNotFound, "no invitation found for the given invitationId")
		return
	}

	tx, err := handler.core.NewTransaction(context.Background())
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	// rollback
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				panic(types.ErrRollback)
			}
		}
	}()

	// add user to group
	if err = handler.core.AddUserToOrganisationWithTx(tx, userId, groupId); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// delete invitation
	if err = handler.core.DeleteInvitationWithTx(tx, invitationId); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// commit changes
	if err := tx.Commit(); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		panic(types.ErrTxCommit)
	}

	// then redirect to /invited, to show the user that they were sucessfully invited to the group (just a one-liner page -> use reset pw page)
	c.Redirect(http.StatusPermanentRedirect, fmt.Sprintf("%s/invited", handler.portal_domain))
}

func (handler *GroupHandlerImpl) removeMember(c *gin.Context) {

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
