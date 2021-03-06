package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	. "github.com/lanceplarsen/go-vault-demo/config"
	. "github.com/lanceplarsen/go-vault-demo/dao"
	. "github.com/lanceplarsen/go-vault-demo/models"
	. "github.com/lanceplarsen/go-vault-demo/vault"
)

var config = Config{}
var dao = OrdersDAO{}
var vault = VaultConf{}

func AllOrdersEndpoint(w http.ResponseWriter, r *http.Request) {
	orders, err := dao.FindAll()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(orders) > 0 {
		respondWithJson(w, http.StatusOK, orders)
	} else {
		respondWithJson(w, http.StatusOK, map[string]string{"result": "No orders"})
	}
}

func CreateOrderEndpoint(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	//Respond with the updated order
	order, err := dao.Insert(order)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJson(w, http.StatusCreated, order)
}

func DeleteOrdersEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := dao.DeleteAll(); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJson(w, http.StatusOK, map[string]string{"result": "success"})
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJson(w, code, map[string]string{"error": msg})
}

func respondWithJson(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func init() {
	log.Println("Starting server initialization")
	//Get our config from the file
	config.Read()
	vault.Config = config

	log.Println("Starting vault initialization")
	//Vault init
	err := vault.InitVault()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Vault initialization complete")

	//Get our DB secrets
	secret, err := vault.GetSecret(config.DB.Role)
	if err != nil {
		log.Fatal(err)
	}
	//Start our Goroutine Renewal for the DB creds
	go vault.RenewSecret(secret)

	//DAO config
	dao.Url = config.DB.Server
	dao.Database = config.DB.Name
	dao.User = secret.Data["username"].(string)
	dao.Password = secret.Data["password"].(string)

	//Check our DB Conn
	log.Println("Starting DB initialization")
	err = dao.Connect()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("DB initialization complete")

	//Looks good
	log.Println("Server initialization complete")
}

func main() {
	//Router
	r := mux.NewRouter()
	r.HandleFunc("/api/orders", AllOrdersEndpoint).Methods("GET")
	r.HandleFunc("/api/orders", CreateOrderEndpoint).Methods("POST")
	r.HandleFunc("/api/orders", DeleteOrdersEndpoint).Methods("DELETE")
	log.Println("Server is now accepting requests on port 3000")
	//Catch SIGINT so we can revoke all our secrets gracefully. TODO
	var gracefulStop = make(chan os.Signal)
	//signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	go func() {
		sig := <-gracefulStop
		fmt.Printf("caught sig: %+v", sig)
		log.Println("Wait for 2 second to finish processing")
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()
	//Start server
	if err := http.ListenAndServe(":3000", r); err != nil {
		log.Fatal(err)
	}
}
