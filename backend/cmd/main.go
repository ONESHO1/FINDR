package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	dl "github.com/ONESHO1/FINDR/backend/internal/songdownload"
)

func main(){
	// setup to get file and line number for error logs
	log.SetReportCaller(true)

	if len(os.Args) < 1 {
		fmt.Printf("Expected 'add' or 'findr' commands")
	}

	// load ENV
	_ = godotenv.Load()

	// for i, arg := range os.Args{
	// 	fmt.Printf("argument %d: %s\n", i, arg)
	// }

	switch os.Args[1] {
	// test - "https://open.spotify.com/track/4lH6nENd1y81jp7Yt9lTBX?si=31d16035bbd643c3"
	case "add":
		// TODO: Get audio file from file path
		// saveSong(os.Args[2])

		// get audio file from spotify link
		dl.GetSongFromSpotify(os.Args[2])
	case "findr":
		fmt.Println("stil havent implemented")
	default:
		fmt.Printf("Expected 'add' or 'findr' command")
	}
}