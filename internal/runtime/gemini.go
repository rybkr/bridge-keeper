package runtime

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadGeminiAPIKey() string {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file (using system env vars if available): %v\n", err)
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set.")
	}
	return apiKey
}
