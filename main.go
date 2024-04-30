package main

import (
	"log"

	"user.service.altiore.io/api"
	"user.service.altiore.io/config"
)

type App struct {
	API *api.API
}

func InitApp() *App {
	return &App{
		API: api.NewAPI(),
	}
}

func main() {
	log.Println("starting user service...")
	config.LoadEnvironmentVariables()
	app := InitApp()
	app.API.Run()
}
