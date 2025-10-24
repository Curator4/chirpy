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

	mux.Handle("/", http.FileServer(http.Dir(".")))

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error %v", err)
	}
}
