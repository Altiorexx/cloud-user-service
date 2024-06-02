package main

import (
	"log"

	"user.service.altiore.io/api"
	"user.service.altiore.io/config"
	"user.service.altiore.io/service"
	"user.service.altiore.io/types"
)

type App struct {
	API api.API
}

func InitApp() *App {
	return &App{
		API: api.NewAPI(&api.API_opts{
			Handlers: []types.Handler{
				api.NewMiddlewareHandler(&api.MiddlewareHandlerOpts{
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
				}),
				api.NewUserHandler(&api.UserHandlerOpts{
					Email: service.NewEmailService(),
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
				}),
				api.NewServiceHandler(),
				api.NewGroupHandler(&api.GroupHandlerOpts{
					Email: service.NewEmailService(),
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
				}),
				api.NewTokenHandler(),
			},
		}),
	}
}

func main() {
	log.Println("starting user service...")
	config.LoadEnvironmentVariables()
	app := InitApp()
	app.API.Run()
}
