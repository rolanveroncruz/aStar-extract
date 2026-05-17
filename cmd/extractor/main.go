package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	//Load the .env file at app startup
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
		os.Exit(1)
	}
	ctx := context.Background()

}
