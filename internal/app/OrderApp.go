package app

import (
	pb "SimpleMSA-gRPC/docs/grpc/gen"
	"SimpleMSA-gRPC/internal/domain/entity"
	"SimpleMSA-gRPC/internal/domain/repository"
	"SimpleMSA-gRPC/internal/domain/service"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"sync"
)

type OrderService interface {
	OrderStatus(order entity.Order) (entity.Order, error)
}

type OrderApp struct {
	OderServ   OrderService
	AuthClient pb.AuthServiceClient // gRPC клиент для аутентификации
}

func (o *OrderApp) getUserFromToken(token string) (entity.User, error) {
	ctx := context.Background()

	// Создание запроса на аутентификацию
	req := &pb.ValidateRequest{Token: token}

	// Вызов gRPC метода для валидации токена
	resp, err := o.AuthClient.Validate(ctx, req)
	if err == nil {
		return entity.User{}, err
	}
	// Преобразование ответа в объект User
	return entity.User{Login: resp.Login}, nil
}

func (o *OrderApp) OrderStatusHandler(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Authorization token is required", http.StatusUnauthorized)
		return
	}
	var order entity.Order
	err := json.NewDecoder(r.Body).Decode(&order)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
	order, err = o.OderServ.OrderStatus(order)

	var user entity.User
	user, err = o.getUserFromToken(token)
	if err != nil {
		http.Error(w, "Could not retrieve user", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user.Login)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order.Status)
}

func NewOrder() *OrderApp {
	return &OrderApp{}
}

func (o *OrderApp) Run(wg *sync.WaitGroup) {
	defer wg.Done()
	var err error
	connStr := "user=postgres password=pgpwd4habr dbname=postgres sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	orderRepo := repository.NewOrderRepository(db)
	orderServ := service.NewOrderService(orderRepo)
	o.OderServ = orderServ

	r := mux.NewRouter()
	r.HandleFunc("/order", o.OrderStatusHandler)

	log.Println("Starting server on :8081")
	log.Fatal(http.ListenAndServe(":8081", r))
}
