package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/repository"
)

type LogHandler interface {
	RegisterRoutes(router *gin.Engine)
}

type LogHandlerImpl struct {
	log repository.LogRepository
}

type LogHandlerOpts struct {
	Log repository.LogRepository
}

func NewLogHandler(opts *LogHandlerOpts) LogHandler {
	return &LogHandlerImpl{
		log: opts.Log,
	}
}

func (handler *LogHandlerImpl) RegisterRoutes(router *gin.Engine) {
	router.GET("/api/logs/:groupId", handler.getGroupLogs)
}

// Gets all logs associated with the group by id.
func (handler *LogHandlerImpl) getGroupLogs(c *gin.Context) {
	groupId := c.Param("groupId")
	logs, err := handler.log.ReadByGroupId(c.Request.Context(), groupId)
	if err != nil {
		log.Printf("error reading group logs: %+v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, logs)
}
