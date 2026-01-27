package main

import (
	"net/http"
	"log"
)

func main() {
	url := "https://3nrcij1tn8b4dn4107vqmtitckib6du2.oastify.com"

	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Request sent, status: %s\n", resp.Status)
}
