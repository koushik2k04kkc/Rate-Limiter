package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type client struct {
	count       int
	windowStart time.Time
}

type Limiter struct {
	mu      sync.Mutex
	clients map[string]*client
	max     int
	window  time.Duration
}

func NewLimiter(max int, window time.Duration) *Limiter {
	l := &Limiter{
		clients: make(map[string]*client),
		max:     max,
		window:  window,
	}
	go l.cleanup()
	return l
}

// Decision lock er bhitore hoy, handler lock er baire cholbe
func (l *Limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	c, ok := l.clients[ip]
	if !ok || now.Sub(c.windowStart) >= l.window {
		l.clients[ip] = &client{count: 1, windowStart: now}
		return true
	}
	if c.count >= l.max {
		return false
	}
	c.count++
	return true
}

// Purano entry delete kore memory leak rodh kore
func (l *Limiter) cleanup() {
	for range time.Tick(l.window) {
		l.mu.Lock()
		for ip, c := range l.clients {
			if time.Since(c.windowStart) >= l.window {
				delete(l.clients, ip)
			}
		}
		l.mu.Unlock()
	}
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !l.allow(ip) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	limiter := NewLimiter(5, time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Request successful")
	})

	fmt.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", limiter.Middleware(mux)))
}
// for i in {1..7}; do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/; done