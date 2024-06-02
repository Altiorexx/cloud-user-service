package api

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"user.service.altiore.io/types"

	"github.com/gin-contrib/cors"
)

type API interface {
	Run()
}

type API_opts struct {
	Handlers []types.Handler
}

type API_impl struct {
	router   *gin.Engine
	handlers []types.Handler
}

func NewAPI(opts *API_opts) *API_impl {
	return &API_impl{
		router:   gin.Default(),
		handlers: opts.Handlers,
	}
}

func (h *API_impl) registerRoutes() {
	for _, handler := range h.handlers {
		handler.RegisterRoutes(h.router)
	}
}

func (h *API_impl) cors() {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PATCH", "DELETE"}
	config.AllowHeaders = []string{"Authorization", "Content-Type"}
	h.router.Use(cors.New(config))
}

func (h *API_impl) Run() {
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
