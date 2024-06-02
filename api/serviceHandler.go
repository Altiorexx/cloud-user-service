package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
)

type ServiceHandler interface {
	RegisterRoutes(*gin.Engine)
}

type ServiceHandlerOpts struct {
	Core repository.CoreRepository
}

type ServiceHandlerImpl struct {
	core repository.CoreRepository
}

func NewServiceHandler(opts *ServiceHandlerOpts) *ServiceHandlerImpl {
	return &ServiceHandlerImpl{
		core: opts.Core,
	}
}

func (h *ServiceHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.GET("/api/service/list", h.serviceList)
	router.GET("/api/service/implementationGroups", h.implementationGroups)
}

// This endpoint might be misplaced, can be relocated later on.
func (h *ServiceHandlerImpl) serviceList(c *gin.Context) {
	services, err := h.core.ReadServices()
	if err != nil {
		log.Println(err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, services)
}

func (h *ServiceHandlerImpl) implementationGroups(c *gin.Context) {
	groups, err := h.core.ImplementationGroupCount(c.Query("name"))
	if err != nil {
		log.Println(err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}
