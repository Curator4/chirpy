package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Curator4/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const port = "8080"

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID     `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Body      string        `json:"body"`
	UserID    uuid.NullUUID `json:"user_id"`
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

func (cfg *apiConfig) reset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(403)
		return
	}
	cfg.fileserverHits.Store(int32(0))
	cfg.dbQueries.Reset(r.Context())
	w.WriteHeader(200)
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var err error
	var errorMsg string

	type parameters struct {
		Email string `json:"email"`
	}

	// decode le email
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		errorMsg = fmt.Sprintf("error decoding parameters: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	// add user to le database
	dbUser, err := cfg.dbQueries.CreateUser(r.Context(), params.Email)
	if err != nil {
		errorMsg = fmt.Sprintf("database error, could not create user: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	// map type database.User to type main.User ???
	mainUser := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	// prepare response
	if err = respondWithJSON(w, 201, mainUser); err != nil {
		errorMsg = fmt.Sprintf("error marshalling JSON: %s", err)
		log.Print(errorMsg)
	}
}

func (cfg *apiConfig) chirp(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var err error
	var errorMsg string

	type parameters struct {
		Body   string        `json:"body"`
		UserID uuid.NullUUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		errorMsg = fmt.Sprintf("error decoding parameters: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	dbChirpParams := database.CreateChirpParams{
		Body:   params.Body,
		UserID: params.UserID,
	}

	dbChirp, err := cfg.dbQueries.CreateChirp(r.Context(), dbChirpParams)
	if err != nil {
		errorMsg = fmt.Sprintf("database error, could not create chirp: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	mainChirp := Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}

	if err = respondWithJSON(w, 201, mainChirp); err != nil {
		errorMsg = fmt.Sprintf("error marshalling json: %s", err)
		log.Print(errorMsg)
	}
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	var err error
	var errorMsg string

	dbChirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		errorMsg = fmt.Sprintf("database error, could not get chirps: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	var mainChirps []Chirp
	var mainChirp Chirp

	for _, dbChirp := range dbChirps {
		mainChirp = Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		}
		mainChirps = append(mainChirps, mainChirp)
	}

	if err = respondWithJSON(w, 200, mainChirps); err != nil {
		log.Print("error in marshaling...")
	}
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, r *http.Request) {
	var err error
	var errorMsg string

	idStr := r.PathValue("chirpID")
	id, err := uuid.Parse(idStr)
	if err != nil {
		errorMsg = fmt.Sprintf("invalid id: %s", err)
		log.Printf(errorMsg)
		respondWithError(w, 400, errorMsg)
		return
	}

	dbChirp, err := cfg.dbQueries.GetChirp(r.Context(), id)
	if err != nil {
		errorMsg = fmt.Sprintf("could not find id: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 404, errorMsg)
		return
	}

	mainChirp := Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}

	if err = respondWithJSON(w, 200, mainChirp); err != nil {
		log.Print("error in marshalling..")
	}
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("db error")
	}

	apiCfg := apiConfig{
		dbQueries: database.New(db),
		platform:  os.Getenv("PLATFORM"),
	}

	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	fs := http.FileServer(http.Dir("."))
	app := http.StripPrefix("/app", fs)
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(app))

	mux.HandleFunc("GET /api/healthz", ready)
	mux.HandleFunc("GET /api/chirps", apiCfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)
	mux.HandleFunc("POST /api/validate_chirp", validate)
	mux.HandleFunc("POST /api/users", apiCfg.createUser)
	mux.HandleFunc("POST /api/chirps", apiCfg.chirp)

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
