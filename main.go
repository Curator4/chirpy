package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Curator4/chirpy/internal/auth"
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
	secret         string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type UserWithToken struct {
	User
	Token string `json:"token"`
}

type UserWithRefreshToken struct {
	UserWithToken
	RefreshToken string `json:"refresh_token"`
}

type Token struct {
	Token string `json:"token"`
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
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	// decode le email n password
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		errorMsg = fmt.Sprintf("error decoding parameters: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		errorMsg = fmt.Sprintf("could not hash password: %s", err)
		log.Print("errorMsg")
		respondWithError(w, 500, errorMsg)
		return
	}

	dbParams := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}

	// add user to le database
	dbUser, err := cfg.dbQueries.CreateUser(r.Context(), dbParams)
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
		Body string `json:"body"`
	}

	// jwt
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errorMsg = fmt.Sprintf("error getting jwt token: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		errorMsg = fmt.Sprintf("invalid JWT: %v", err)
		log.Print(errorMsg)
		respondWithError(w, 401, errorMsg)
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
		Body: params.Body,
		UserID: uuid.NullUUID{
			UUID:  userID,
			Valid: true,
		},
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
		log.Print(errorMsg)
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

func (cfg *apiConfig) login(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var err error
	var errorMsg string

	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	// decode le email n password
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		errorMsg = fmt.Sprintf("error decoding parameters: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		errorMsg = fmt.Sprintf("could not find user with email %s: %s", params.Email, err)
		log.Print(errorMsg)
		respondWithError(w, 404, errorMsg)
		return
	}

	authorized, err := auth.CheckPasswordHash(params.Password, dbUser.HashedPassword)
	if err != nil {
		errorMsg = fmt.Sprintf("failed to authorize: %s", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	if !authorized {
		errorMsg = "Incorrect email or password"
		log.Print(errorMsg)
		respondWithError(w, 401, errorMsg)
		return
	}

	mainUser := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	// JWT stuff
	jwtDuration := time.Duration(3600) * time.Second

	jwtTokenString, err := auth.MakeJWT(mainUser.ID, cfg.secret, jwtDuration)
	if err != nil {
		errorMsg = fmt.Sprintf("error in jwt token creation %v", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	mainUserWithToken := UserWithToken{
		User:  mainUser,
		Token: jwtTokenString,
	}

	refreshTokenDuration := time.Now().Add(60 * 24 * time.Hour)
	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		errorMsg = fmt.Sprintf("error in creation of refreshtoken %v", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}
	refreshTokenParams := database.CreateRefreshTokenParams{
		Token: refreshToken,
		UserID: uuid.NullUUID{
			UUID:  mainUser.ID,
			Valid: true,
		},
		ExpiresAt: refreshTokenDuration,
	}

	_, err = cfg.dbQueries.CreateRefreshToken(r.Context(), refreshTokenParams)

	mainUserWithRefreshToken := UserWithRefreshToken{
		UserWithToken: mainUserWithToken,
		RefreshToken:  refreshToken,
	}

	if err = respondWithJSON(w, 200, mainUserWithRefreshToken); err != nil {
		errorMsg = fmt.Sprintf("error marshalling JSON: %v", err)
		log.Print(errorMsg)
	}
}

func (cfg *apiConfig) refresh(w http.ResponseWriter, r *http.Request) {
	var err error
	var errorMsg string

	bearerToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errorMsg := fmt.Sprintf("authorization error: %v", err)
		log.Print(errorMsg)
		respondWithError(w, http.StatusUnauthorized, errorMsg)
		return
	}

	refreshToken, err := cfg.dbQueries.GetRefreshToken(r.Context(), bearerToken)
	if err != nil {
		errorMsg = fmt.Sprintf("no refresh token found: %v", err)
		log.Print(errorMsg)
		respondWithError(w, 401, errorMsg)
		return
	}
	if refreshToken.RevokedAt.Valid {
		errorMsg = "license has been revoked! %v"
		log.Print(errorMsg)
		respondWithError(w, 401, errorMsg)
		return
	}
	if time.Now().After(refreshToken.ExpiresAt) {
		errorMsg = "refresh token expired: %v"
		log.Print(errorMsg)
		respondWithError(w, 401, errorMsg)
		return
	}

	// JWT stuff
	jwtDuration := time.Duration(3600) * time.Second
	jwtTokenString, err := auth.MakeJWT(refreshToken.UserID.UUID, cfg.secret, jwtDuration)
	if err != nil {
		errorMsg = fmt.Sprintf("error in jwt token creation %v", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	jwtToken := Token{
		Token: jwtTokenString,
	}

	if err = respondWithJSON(w, 200, jwtToken); err != nil {
		errorMsg = fmt.Sprintf("error marshalling JSON %v", err)
		log.Print(errorMsg)
	}
}

func (cfg *apiConfig) revoke(w http.ResponseWriter, r *http.Request) {
	var err error
	var errorMsg string

	bearerToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errorMsg := fmt.Sprintf("authorization error: %v", err)
		log.Print(errorMsg)
		respondWithError(w, http.StatusUnauthorized, errorMsg)
		return
	}

	if err = cfg.dbQueries.RevokeRefreshToken(r.Context(), bearerToken); err != nil {
		errorMsg = fmt.Sprintf("error in refreshToken revokation %v", err)
		log.Print(errorMsg)
		respondWithError(w, 500, errorMsg)
		return
	}

	w.WriteHeader(204)
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
		secret:    os.Getenv("SECRET"),
	}

	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	fs := http.FileServer(http.Dir("."))
	app := http.StripPrefix("/app", fs)
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(app))

	mux.HandleFunc("POST /api/login", apiCfg.login)
	mux.HandleFunc("POST /api/refresh", apiCfg.refresh)
	mux.HandleFunc("POST /api/revoke", apiCfg.revoke)
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
