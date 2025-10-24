package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error %v", err)
	}
}
