package main

import (
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/cors"
)

type JSONServer struct {
	store      Storage
	listenAddr string
}

func NewJSONServer(listenAddr string, store Storage) *JSONServer {
	return &JSONServer{
		store:      store,
		listenAddr: listenAddr,
	}
}

func (s *JSONServer) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /messages", withJWTAuth(makeHttpHandler(s.HandleGetMessages)))
	mux.HandleFunc("GET /users", withJWTAuth(makeHttpHandler(s.HandleGetUsers)))
	mux.HandleFunc("POST /signup", makeHttpHandler(s.HandleSignUp))
	mux.HandleFunc("POST /login", makeHttpHandler(s.HandleLogin))

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://chatclient:3000"},
		AllowCredentials: true,
		// Enable Debugging for testing, consider disabling in production
		Debug: true,
	})

	handler := c.Handler(mux)

	log.Printf("Mchat JSON server is live on: %s\n", s.listenAddr)
	return http.ListenAndServe(s.listenAddr, handler)
}

func (s *JSONServer) HandleGetMessages(w http.ResponseWriter, r *http.Request) *JSONServerError {
	messages, err := s.store.GetMessages()
	if err != nil {
		return &JSONServerError{
			code:  500,
			error: err.Error(),
		}
	}
	WriteJSON(w, http.StatusOK, messages)
	log.Printf("retrieved %d user messages", len(messages))
	return nil
}

func (s *JSONServer) HandleSignUp(w http.ResponseWriter, r *http.Request) *JSONServerError {
	reqData, err := UnmarshalUserJSON(r)
	if err != nil {
		log.Printf("user unmarshal error: %s\n", err)
		return &JSONServerError{
			code:  http.StatusBadRequest,
			error: "invalid request data",
		}
	}
	hashedPass, err := HashPassword(reqData.Password)
	if err != nil {
		return &JSONServerError{
			code:  http.StatusInternalServerError,
			error: err.Error(),
		}
	}
	reqData.Password = hashedPass
	// Create User
	if err := s.store.CreateUser(reqData); err != nil {
		return &JSONServerError{
			code:  http.StatusInternalServerError,
			error: err.Error(),
		}
	}
	usr := &UserJSONResponse{
		Id:       reqData.Id,
		Username: reqData.Username,
	}
	usr.Token, err = createJWT(reqData)
	if err != nil {
		return &JSONServerError{
			code:  http.StatusInternalServerError,
			error: err.Error(),
		}
	}
	// Return the user object and the jwt token
	log.Printf("created user with username: %s\n", usr.Username)
	WriteJSON(w, http.StatusCreated, usr)
	return nil
}

func (s *JSONServer) HandleLogin(w http.ResponseWriter, r *http.Request) *JSONServerError {
	reqData, err := UnmarshalUserJSON(r)
	if err != nil {
		log.Printf("user unmarshal error: %s\n", err)
		return &JSONServerError{
			code:  500,
			error: "invalid request data",
		}
	}
	// Check if user exists
	dbUsr, err := s.store.GetUser(reqData.Username)
	if err != nil {
		log.Printf("user unmarshal error: %s\n", err)
		return &JSONServerError{
			code:  http.StatusInternalServerError,
			error: err.Error(),
		}
	}
	// Validate Password
	valid := VerifyPassword(reqData.Password, dbUsr.Password)
	if !valid {
		return &JSONServerError{
			code:  http.StatusUnauthorized,
			error: "invalid credentials",
		}
	}

	usr := &UserJSONResponse{
		Username: dbUsr.Username,
		Id:       dbUsr.Id,
	}
	usr.Token, err = createJWT(dbUsr)
	if err != nil {
		log.Printf("jwt create err: %s\n", err)
		return &JSONServerError{
			code:  http.StatusInternalServerError,
			error: err.Error(),
		}
	}

	log.Printf("succesfully logged in: %s\n", usr.Username)
	WriteJSON(w, http.StatusOK, usr)
	return nil
}

func (s *JSONServer) HandleGetUsers(w http.ResponseWriter, r *http.Request) *JSONServerError {
	usrs, err := s.store.GetUsers()
	if err != nil {
		log.Printf("get users err: %s\n", err)
		return &JSONServerError{
			code:  500,
			error: err.Error(),
		}
	}
	WriteJSON(w, http.StatusOK, usrs)
	return nil
}

type JSONServerFunc func(w http.ResponseWriter, r *http.Request) *JSONServerError

type CustomClaims struct {
	Username string
	Password string
	jwt.RegisteredClaims
}

type JSONServerError struct {
	error string
	code  int
}

func (e *JSONServerError) Error() string {
	return e.error
}

func makeHttpHandler(serverFunc JSONServerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := serverFunc(w, r); err != nil {
			WriteJSON(w, err.code, err.Error())
			log.Printf("json api server err: %s\n", err)
			return
		}
	}
}

func withJWTAuth(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.Header.Get("Authorization")
		_, err := validateJWT(tokenString)
		if err != nil {
			log.Printf("jwt auth err: %s\n", err)
			WriteJSON(w, http.StatusUnauthorized, JSONServerError{
				code:  http.StatusUnauthorized,
				error: "invalid token",
			})
			return
		}

		handlerFunc(w, r)
	}
}
