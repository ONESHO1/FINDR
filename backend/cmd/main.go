package main

import (
	"os"

	"github.com/joho/godotenv"

	"github.com/ONESHO1/FINDR/backend/internal/log"
	dl "github.com/ONESHO1/FINDR/backend/internal/songdownload"
	"github.com/ONESHO1/FINDR/backend/internal/match"
)

func main(){
	// load ENV
	_ = godotenv.Load()

	log.Init()

	if len(os.Args) < 2 {
		log.Logger.Fatal("Expected 'add' or 'findr' commands")
	}

	// for i, arg := range os.Args{
	// 	fmt.Printf("argument %d: %s\n", i, arg)
	// }

	switch os.Args[1] {
	// test - "https://open.spotify.com/track/4lH6nENd1y81jp7Yt9lTBX?si=31d16035bbd643c3"
	case "add":
		// TODO: Get audio file from file path
		// saveSong(os.Args[2])

		if len(os.Args) < 3 {
			log.Logger.Fatal("Missing Spotify link for 'add' command")
		}

		// get audio file from spotify link
		dl.GetSongFromSpotify(os.Args[2])
	case "findr":
		// log.Logger.Info("still havent implemented")
		match.RecordAndFind()
	default:
		log.Logger.Fatalf("Unknown command: %s. Expected 'add' or 'findr'", os.Args[1])
	}
}