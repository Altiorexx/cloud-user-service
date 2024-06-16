package api

import (
	"database/sql"
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
	Role     repository.RoleRepository
	Firebase service.FirebaseService
	Email    service.EmailService
}

type GroupHandlerImpl struct {
	core          repository.CoreRepository
	role          repository.RoleRepository
	case_         *service.CaseService
	email         service.EmailService
	firebase      service.FirebaseService
	domain        string
	portal_domain string
}

func NewGroupHandler(opts *GroupHandlerOpts) *GroupHandlerImpl {
	return &GroupHandlerImpl{
		core:          opts.Core,
		role:          opts.Role,
		firebase:      opts.Firebase,
		case_:         service.NewCaseService(),
		email:         opts.Email,
		domain:        os.Getenv("DOMAIN"),
		portal_domain: os.Getenv("PORTAL_DOMAIN"),
	}
}

func (handler *GroupHandlerImpl) RegisterRoutes(router *gin.Engine) {

	// steamline endpoints, so :groupId is present in the path were relevant / expected ..

	router.POST("/api/group/create", handler.createOrganisation)
	router.GET("/api/group/list", handler.organisationList)
	router.GET("/api/group/:id", handler.getGroup)
	router.PATCH("/api/group/:id/update", handler.updateMetadata)
	router.DELETE("/api/group/:id/delete", handler.deleteGroup)
	router.GET("/api/group/:id/members", handler.members)
	router.POST("/api/group/member/invite", handler.inviteMember)
	router.GET("/api/group/join", handler.joinGroup)
	router.DELETE("/api/group/member/remove", handler.removeMember)

	router.GET("/api/group/:id/role/defined_roles", handler.getDefinedRoles)
	router.POST("/api/group/:id/role/update", handler.updateRoles)
	router.POST("/api/group/:id/role/delete", handler.deleteRole)
	router.GET("/api/group/:id/role/member_roles", handler.getMemberRoles)

	router.POST("/api/group/:id/member/add_role", handler.addMemberRole)
	router.POST("/api/group/:id/member/remove_role", handler.removeMemberRole)

	router.GET("/api/group/reject", handler.rejectGroup)
}

// Add role to group member.
func (handler *GroupHandlerImpl) addMemberRole(c *gin.Context) {
	ctx := c.Request.Context()
	var body struct {
		UserId string `json:"userId" binding:"required"`
		RoleId string `json:"roleId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := handler.core.WithTransaction(ctx, func(tx *sql.Tx) error {
		return handler.role.AddMemberRole(tx, body.UserId, body.RoleId)
	})
	if err != nil {
		log.Printf("error mapping role to user: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return

	}
	c.Status(http.StatusOK)
}

func (handler *GroupHandlerImpl) removeMemberRole(c *gin.Context) {
	ctx := c.Request.Context()
	var body struct {
		UserId string `json:"userId" binding:"required"`
		RoleId string `json:"roleId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := handler.core.WithTransaction(ctx, func(tx *sql.Tx) error {
		return handler.role.RemoveMemberRole(tx, body.UserId, body.RoleId)
	})
	if err != nil {
		log.Printf("error removing member role: %+v\n", err)
		switch {
		case errors.Is(err, types.ErrForbiddenOperation):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case errors.Is(err, types.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.Status(http.StatusOK)
}

// Get all members with their associated roles within a group.
func (handler *GroupHandlerImpl) getMemberRoles(c *gin.Context) {
	_ = c.Request.Context()
	groupId := c.Param("id")
	member_roles, err := handler.role.GetMembersWithRoles(groupId)
	if err != nil {
		log.Printf("error getting member roles: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, member_roles)
}

func (handler *GroupHandlerImpl) getDefinedRoles(c *gin.Context) {
	groupId := c.Param("id")
	if groupId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no group id"})
		return
	}
	roles, err := handler.role.ReadRoles(groupId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(roles) == 0 {
		roles = make([]*types.Role, 0)
	}
	c.JSON(http.StatusOK, roles)
}

// Update the roles for a group.
func (handler *GroupHandlerImpl) updateRoles(c *gin.Context) {
	var body []*types.Role
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := handler.core.WithTransaction(c.Request.Context(), func(tx *sql.Tx) error {
		return handler.role.UpdateRolesWithTx(tx, body, c.Param("id"))
	})
	if err != nil {
		log.Printf("error updating roles: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Status(http.StatusOK)
}

func (handler *GroupHandlerImpl) deleteRole(c *gin.Context) {
	var body struct {
		RoleId string `json:"roleId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := handler.core.WithTransaction(c.Request.Context(), func(tx *sql.Tx) error {
		return handler.role.DeleteRoleWithTx(tx, body.RoleId)
	})
	if err != nil {
		log.Printf("error deleting group role: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Status(http.StatusOK)
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
	groupId := c.Param("id")
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	err := handler.core.WithTransaction(c.Request.Context(), func(tx *sql.Tx) error {
		// update name if requested
		if body.Name != "" {
			if err := handler.core.UpdateGroupNameWithTx(tx, groupId, body.Name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("failed to commit group metadata changes: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Status(http.StatusOK)
}

// Delete a group and related data.
func (handler *GroupHandlerImpl) deleteGroup(c *gin.Context) {
	err := handler.core.WithTransaction(c.Request.Context(), func(tx *sql.Tx) error {
		return handler.core.DeleteGroupWithTx(tx, c.GetString("userId"), c.Param("id"))
	})
	if err != nil {
		log.Printf("error deleting group: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
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
	err := handler.core.WithTransaction(c.Request.Context(), func(tx *sql.Tx) error {
		return handler.core.CreateOrganisationWithTx(tx, body.Name, c.GetString("userId"))
	})
	if err != nil {
		log.Printf("error creating group: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Status(http.StatusOK)
}

func (handler *GroupHandlerImpl) members(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no group id set"})
		return
	}
	members, err := handler.core.ReadOrganisationMembers(id)
	if err != nil {
		log.Printf("error reading group members: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, members)
}

// Get a list of groups the user is associated with.
func (handler *GroupHandlerImpl) organisationList(c *gin.Context) {
	organisationList, err := handler.core.OrganisationList(c.GetString("userId"))
	if err != nil {
		log.Printf("error reading list of groups: %+v\n", err)
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, organisationList)
}

func (handler *GroupHandlerImpl) inviteMember(c *gin.Context) {

	var body struct {
		Email   string `json:"email" binding:"required"`
		GroupId string `json:"groupId" binding:"required"`
		Name    string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
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
			c.JSON(http.StatusConflict, gin.H{"error": "user is already a member of the group"})
			return
		}
	}

	// generate link
	invitationId, err := handler.core.CreateInvitation(userId, body.Email, body.GroupId)
	if err != nil {
		log.Printf("error creating invitation: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error creating invitation"})
		return
	}

	var link string
	if userId == "" {
		link = fmt.Sprintf("%s/signup?inv=%s", handler.portal_domain, invitationId)
	} else {
		link = fmt.Sprintf("%s/api/group/join?inv=%s", handler.domain, invitationId)
	}

	// if no user was found, send an signin invitation flow
	// else send a simple accept / reject invitation flow
	var message string
	if userId == "" {
		message = handler.email.CreateSignupAndInvitationMail(body.Email, body.Name, link)
	} else {
		message = handler.email.CreateInvitationMail(body.Email, body.Name, link)
	}
	if err := handler.email.Send([]string{body.Email}, message); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusOK)
}

func (handler *GroupHandlerImpl) joinGroup(c *gin.Context) {
	ctx := c.Request.Context()
	invitationId := c.Query("inv")
	if invitationId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no invitation id set"})
		return
	}

	// lookup invitation
	userId, groupId, email, err := handler.core.LookupInvitation(invitationId)
	if err != nil {
		log.Printf("error looking up invitation: %+v\n", err)
		switch {
		case errors.Is(err, types.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "no invitation found for the given invitationId"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error looking up invitation"})
		}
		return
	}

	// if invitation was for a user, not yet registered and only the email were provided,
	// then lookup the user as they have only registered after receiving the invite.
	user, err := handler.core.ReadUserByEmail(email)
	if err != nil {
		switch {
		case errors.Is(err, types.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error reading user"})
		}
		return
	}

	// bind to user only known (registered in our system) after the invitation was sent
	if userId == "" {
		userId = user.Id
	}

	err = handler.core.WithTransaction(ctx, func(tx *sql.Tx) error {

		// add user to group
		if err := handler.core.AddUserToOrganisationWithTx(tx, userId, groupId); err != nil {
			return err
		}

		// delete invitation
		if err := handler.core.DeleteInvitationWithTx(tx, invitationId); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.Printf("error: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// redirect to an error page if things went wrong -> the user should not experience an 'error' http blank page thing..

	// indicate to the user that things went well, by redirecting to a success page
	c.Redirect(http.StatusPermanentRedirect, fmt.Sprintf("%s/invited", handler.portal_domain))
}

func (handler *GroupHandlerImpl) rejectGroup(c *gin.Context) {

	invitationId := c.Query("inv")
	if invitationId == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no invitation id given"})
		return
	}

	// remove invitation from db
	if err := handler.core.DeleteInvitation(invitationId); err != nil {
		log.Printf("error deleting invitation: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error deleting invitation"})
		return
	}

	// redirect to rejected page (use reset, invited page layout)
	c.Redirect(http.StatusPermanentRedirect, fmt.Sprintf("%s/rejected", handler.portal_domain))
}

func (handler *GroupHandlerImpl) removeMember(c *gin.Context) {
	ctx := c.Request.Context()
	var body struct {
		UserId  string `json:"userId" binding:"required"`
		GroupId string `json:"groupId" binding:"required"`
		Name    string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := handler.core.WithTransaction(ctx, func(tx *sql.Tx) error {
		return handler.core.RemoveUserFromOrganisationWithTx(tx, body.UserId, body.GroupId)
	})
	if err != nil {
		log.Printf("error removing user from group: %+v\n", err)
		switch err {
		case types.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// read user's email, to send a notification
	user, err := handler.core.ReadUserById(body.UserId)
	if err != nil {
		log.Printf("error reading user by id: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error reading user email"})
		return
	}
	if err := handler.email.Send([]string{user.Email}, handler.email.CreateRemovedFromGroup(user.Email, body.Name)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error sending email"})
		return
	}
	c.Status(http.StatusOK)
}
