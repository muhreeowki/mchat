package main

import (
	"fmt"
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
	mux.HandleFunc("POST /signup", withJWTAuth(makeHttpHandler(s.HandleSignUp)))
	mux.HandleFunc("POST /login", withJWTAuth(makeHttpHandler(s.HandleLogin)))

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:3000/login"},
		AllowCredentials: true,
		// Enable Debugging for testing, consider disabling in production
		Debug: true,
	})

	handler := c.Handler(mux)

	fmt.Printf("Mchat JSON server is live on: %s\n", s.listenAddr)
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
	return nil
}

func (s *JSONServer) HandleSignUp(w http.ResponseWriter, r *http.Request) *JSONServerError {
	reqData, err := UnmarshalUserJSON(r)
	if err != nil {
		fmt.Println(err)
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
	usr.Token, err = CreateJWT(reqData)
	if err != nil {
		return &JSONServerError{
			code:  http.StatusInternalServerError,
			error: err.Error(),
		}
	}
	// Return the user object and the jwt token
	WriteJSON(w, http.StatusCreated, usr)
	return nil
}

func (s *JSONServer) HandleLogin(w http.ResponseWriter, r *http.Request) *JSONServerError {
	reqData, err := UnmarshalUserJSON(r)
	if err != nil {
		return &JSONServerError{
			code:  500,
			error: err.Error(),
		}
	}
	// Check if user exists
	dbUsr, err := s.store.GetUser(reqData.Username)
	if err != nil {
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

	usr := &User{
		Username: dbUsr.Username,
		Id:       dbUsr.Id,
	}
	usr.Token, err = CreateJWT(dbUsr)
	if err != nil {
		return &JSONServerError{
			code:  http.StatusInternalServerError,
			error: err.Error(),
		}
	}

	WriteJSON(w, http.StatusOK, usr)
	return nil
}

func (s *JSONServer) HandleGetUsers(w http.ResponseWriter, r *http.Request) *JSONServerError {
	usrs, err := s.store.GetUsers()
	if err != nil {
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

func withJWTAuth(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("checking JWT Token")
		handlerFunc(w, r)
	}
}

func makeHttpHandler(serverFunc JSONServerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := serverFunc(w, r); err != nil {
			WriteJSON(w, err.code, err.Error())
			return
		}
	}
}
