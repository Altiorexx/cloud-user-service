package api

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/types"

	"github.com/gin-contrib/cors"
)

type API struct {
	router   *gin.Engine
	handlers []types.Handler
}

func NewAPI() *API {
	return &API{
		router: gin.Default(),
		handlers: []types.Handler{
			NewMiddlewareHandler(),
			NewUserHandler(),
			NewServiceHandler(),
			NewGroupHandler(),
			NewTokenHandler(),
		},
	}
}

func (h *API) registerRoutes() {
	for _, handler := range h.handlers {
		handler.RegisterRoutes(h.router)
	}
}

func (h *API) cors() {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PATCH", "DELETE"}
	config.AllowHeaders = []string{"Authorization", "Content-Type"}
	h.router.Use(cors.New(config))
}

func (h *API) Run() {
	h.cors()
	h.registerRoutes()
	h.router.GET("/:noget", func(c *gin.Context) {
		c.String(http.StatusOK, "hello world!")
	})
	log.Printf("starting api on port %s...", os.Getenv("PORT"))
	err := http.ListenAndServe(":"+os.Getenv("PORT"), h.router)
	if err != nil {
		panic(err)
	}
}
