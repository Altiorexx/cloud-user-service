package main

import (
	"log"

	"user.service.altiore.io/api"
	"user.service.altiore.io/config"
	"user.service.altiore.io/repository"
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
					Core: repository.NewCoreRepository(&repository.CoreRepositoryOpts{
						Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{
							Key: "1",
						}),
						Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
							Email: service.NewEmailService(),
						}, "1"),
					}, "1"),
					Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{
						Key: "1",
					}),
					Log: repository.NewLogRepository(&repository.LogRepositoryOpts{Key: "1"}),
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
					Token: service.NewTokenService(nil),
				}),
				api.NewUserHandler(&api.UserHandlerOpts{
					Core: repository.NewCoreRepository(&repository.CoreRepositoryOpts{
						Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{
							Key: "1",
						}),
						Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
							Email: service.NewEmailService(),
						}, "1"),
					}, "1"),
					Email: service.NewEmailService(),
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
				}),
				api.NewServiceHandler(&api.ServiceHandlerOpts{
					Core: repository.NewCoreRepository(&repository.CoreRepositoryOpts{
						Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{
							Key: "1",
						}),
						Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
							Email: service.NewEmailService(),
						}, "1"),
					}, "1"),
				}),
				api.NewGroupHandler(&api.GroupHandlerOpts{
					Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{
						Key: "1",
					}),
					Core: repository.NewCoreRepository(&repository.CoreRepositoryOpts{
						Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{
							Key: "1",
						}),
						Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
							Email: service.NewEmailService(),
						}, "1"),
					}, "1"),
					Email: service.NewEmailService(),
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
				}),
				api.NewTokenHandler(&api.TokenHandlerOpts{
					Core: repository.NewCoreRepository(&repository.CoreRepositoryOpts{
						Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{
							Key: "1",
						}),
						Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
							Email: service.NewEmailService(),
						}, "1"),
					}, "1"),
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
				}),
				api.NewLogHandler(&api.LogHandlerOpts{
					Log: repository.NewLogRepository(&repository.LogRepositoryOpts{Key: "1"}),
				}),
				api.NewInternalHandler(&api.InternalHandlerOpts{
					Core: repository.NewCoreRepository(&repository.CoreRepositoryOpts{
						Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
							Email: service.NewEmailService(),
						}, "1"),
					}, "1"),
					Role: repository.NewRoleRepository(&repository.RoleRepositoryOpts{Key: "1"}),
					Log:  repository.NewLogRepository(&repository.LogRepositoryOpts{Key: "1"}),
					Firebase: service.NewFirebaseService(&service.FirebaseServiceOpts{
						Email: service.NewEmailService(),
					}, "1"),
				}),
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
