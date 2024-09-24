package app

import (
	pb "SimpleMSA-gRPC/docs/grpc/gen"
	"SimpleMSA-gRPC/internal/domain/entity"
	"SimpleMSA-gRPC/internal/domain/repository"
	"SimpleMSA-gRPC/internal/domain/service"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

var db *sql.DB
var secretKey = []byte("jwt_token_example")

type server struct {
	pb.UnimplementedAuthServiceServer
}

type App struct {
	serv Service
}

type Service interface {
	Registration(user entity.User) error
	Authorization(user entity.User) error
}

type AuthResponse struct {
	Token string `json:"token"`
}

func generateToken(user entity.User) (string, error) {
	expirationTime := time.Now().Add(5 * time.Minute)
	claims := &jwt.StandardClaims{
		ExpiresAt: expirationTime.Unix(),
		IssuedAt:  time.Now().Unix(),
		Subject:   user.Login,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey)
}

func validateToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Проверка алгоритма
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})

	if err != nil || !token.Valid {
		return "", err
	}

	// Извлечение информации о пользователе из токена
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	var user entity.User
	user.Login = claims[user.Login].(string)
	return user.Login, nil
}

func (a *App) registrHandler(w http.ResponseWriter, r *http.Request) {
	var regUser entity.User
	err := json.NewDecoder(r.Body).Decode(&regUser)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = a.serv.Registration(regUser)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
	return
}

func (a *App) loginHandler(w http.ResponseWriter, r *http.Request) {
	var user entity.User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = a.serv.Authorization(user)
	if err != nil {
		log.Fatal(err)
	}
	token, err := generateToken(user)
	if err != nil {
		http.Error(w, "Could not generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{Token: token})
	return
}

func (s *server) Validate(ctx context.Context, req *pb.ValidateRequest) (*pb.ValidateResponse, error) {
	login, err := validateToken(req.GetToken())
	if err != nil {
		return nil, err
	}
	return &pb.ValidateResponse{Login: login}, err
}

func New() *App {
	return &App{}
}

func (a *App) Run(wg *sync.WaitGroup) {
	defer wg.Done()
	var err error
	connStr := "user=postgres password=pgpwd4habr dbname=postgres sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	repo := repository.NewPostgresUserRepository(db)
	serv := service.NewUserService(repo)
	a.serv = serv

	r := mux.NewRouter()

	r.HandleFunc("/reg", a.registrHandler).Methods("POST")
	r.HandleFunc("/login", a.loginHandler).Methods("POST")

	// Запуск HTTP-сервера в отдельной горутине
	go func() {
		log.Println("Starting HTTP server on port :8080")
		if err := http.ListenAndServe(":8080", r); err != nil {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Запуск gRPC-сервера
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
		return
	}

	s := grpc.NewServer()
	pb.RegisterAuthServiceServer(s, &server{})
	log.Println("Starting gRPC server on port :50051")

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
