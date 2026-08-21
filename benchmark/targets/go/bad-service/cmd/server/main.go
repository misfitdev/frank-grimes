package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    _ "github.com/lib/pq"
)

type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   string      `json:"error,omitempty"`
}

var db *sql.DB

func main() {
    connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s",
        os.Getenv("DB_HOST"),
        os.Getenv("DB_PORT"),
        os.Getenv("DB_USER"),
        os.Getenv("DB_PASSWORD"),
        os.Getenv("DB_NAME"),
    )

    var err error
    db, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal(err)
    }

    http.HandleFunc("/users", handleUsers)
    http.HandleFunc("/user/", handleUser)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    log.Printf("Server starting on port %s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }

    rows, err := db.Query("SELECT id, name, email FROM users")
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    users := []User{}
    for rows.Next() {
        var u User
        rows.Scan(&u.ID, &u.Name, &u.Email)
        users = append(users, u)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(Response{Success: true, Data: users})
}

func handleUser(w http.ResponseWriter, r *http.Request) {
    path := strings.TrimPrefix(r.URL.Path, "/user/")
    userID := path

    if r.Method == http.MethodGet {
        var u User
        err := db.QueryRow("SELECT id, name, email FROM users WHERE id = $1", userID).Scan(&u.ID, &u.Name, &u.Email)
        if err != nil {
            w.WriteHeader(http.StatusNotFound)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(Response{Success: true, Data: u})

    } else if r.Method == http.MethodPost {
        var u User
        err := json.NewDecoder(r.Body).Decode(&u)
        if err != nil {
            w.WriteHeader(http.StatusBadRequest)
            return
        }

        err = db.Exec("INSERT INTO users (name, email) VALUES ($1, $2)", u.Name, u.Email).Error
        if err != nil {
            w.WriteHeader(http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(Response{Success: true, Data: u})

    } else if r.Method == http.MethodPut {
        var u User
        err := json.NewDecoder(r.Body).Decode(&u)
        if err != nil {
            w.WriteHeader(http.StatusBadRequest)
            return
        }

        _, err = db.Exec("UPDATE users SET name = $1, email = $2 WHERE id = $3", u.Name, u.Email, userID)
        if err != nil {
            w.WriteHeader(http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(Response{Success: true, Data: u})

    } else if r.Method == http.MethodDelete {
        _, err := db.Exec("DELETE FROM users WHERE id = $1", userID)
        if err != nil {
            w.WriteHeader(http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusNoContent)
    }
}
