package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
)

const port = "8080"

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	html := `
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>
	`
	fmt.Fprintf(w, html, cfg.fileserverHits.Load())
}

func (cfg *apiConfig) reset(w http.ResponseWriter, _ *http.Request) {
	cfg.fileserverHits.Store(int32(0))
	w.WriteHeader(http.StatusOK)
}

func main() {
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	apiCfg := apiConfig{}

	fs := http.FileServer(http.Dir("."))
	app := http.StripPrefix("/app", fs)
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(app))

	mux.HandleFunc("GET /api/healthz", ready)
	mux.HandleFunc("POST /api/validate_chirp", validate)

	mux.HandleFunc("GET /admin/metrics", apiCfg.metrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.reset)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error %v", err)
	}
}

func ready(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// encode/decode json
func validate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var errorMsg string
	var err error

	// first define the expected struct stucture
	type parameters struct {
		Body string `json:"body"`
	}

	// also define the json to be returned, error/valid
	type returnVals struct {
		CleanedBody string `json:"cleaned_body"`
	}

	// then define a "decoder", the empty struct, and try decoding, throw error if fail
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err = decoder.Decode(&params); err != nil {
		errorMsg = fmt.Sprintf("error decoding parameters: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	// check length
	if len(params.Body) > 140 {
		errorMsg = "chirp is too long"
		log.Print(errorMsg)
		respondWithError(w, 400, errorMsg)
		return
	}

	// prepare response, marshal, send with helper
	respBody := returnVals{
		CleanedBody: censorProfanity(params.Body),
	}
	if err = respondWithJSON(w, 200, respBody); err != nil {
		errorMsg = fmt.Sprintf("error marshalling JSON: %s", err)
		log.Print(errorMsg)
		return
	}

}

// json helpers
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) error {
	response, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	w.Write(response)
	return nil
}

func respondWithError(w http.ResponseWriter, code int, msg string) error {
	return respondWithJSON(w, code, map[string]string{"error": msg})
}

// profanity helper, should have used map here ofc for O(1)
func censorProfanity(body string) string {
	badWords := [3]string{"kerfuffle", "sharbert", "fornax"}
	splitBody := strings.Split(body, " ")

	for i, element := range splitBody {
		for _, badWord := range badWords {
			if strings.ToLower(element) == badWord {
				splitBody[i] = "****"
			}
		}
	}

	return strings.Join(splitBody, " ")
}
