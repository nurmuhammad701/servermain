package main

import (
	"errors"
	"fmt"
	"log"
	"mainserver/logs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type Server struct {
	URL   string
	Alive bool
	mux   sync.RWMutex
}

type LoadBalancer struct {
	servers []*Server
	current int64
}

func NewLoadBalancer(serverURLs []string) *LoadBalancer {
	servers := make([]*Server, len(serverURLs))
	for i, url := range serverURLs {
		servers[i] = &Server{URL: url, Alive: true}
	}
	return &LoadBalancer{
		servers: servers,
	}
}

func (s *Server) SetAlive(alive bool) {
	s.mux.Lock()
	s.Alive = alive
	s.mux.Unlock()
}

func (s *Server) IsAlive() bool {
	s.mux.RLock()
	alive := s.Alive
	s.mux.RUnlock()
	return alive
}

func (lb *LoadBalancer) NextServer() *Server {
	serverCount := len(lb.servers)
	for i := 0; i < serverCount; i++ {
		next := atomic.AddInt64(&lb.current, 1) % int64(serverCount)
		if lb.servers[next].IsAlive() {
			return lb.servers[next]
		}
	}
	return nil
}

func (lb *LoadBalancer) HealthCheck() {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	for {
		for _, server := range lb.servers {
			alive, err := isServerAlive(client, server.URL+"/user/list")
			server.SetAlive(alive)
			if alive {
				fmt.Printf("Server %s: working\n", server.URL)
			} else {
				fmt.Printf("Server %s: not working (cause: %v)\n", server.URL, err)
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func isServerAlive(client *http.Client, url string) (bool, error) {
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, errors.New("Server status is not good")
	}
	return true, nil
}

func main() {
	logs := logs.NewLogger()
	serverURLs := []string{
		"http://13.38.70.151:8081",
		"http://15.237.51.177:8082",
	}
	lb := NewLoadBalancer(serverURLs)

	go lb.HealthCheck()

	r := gin.Default()

	r.Any("/*path", func(c *gin.Context) {
		server := lb.NextServer()
		if server == nil {
			c.String(http.StatusServiceUnavailable, "No available servers")
			return
		}

		url, err := url.Parse(server.URL)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		clientIP := c.ClientIP()
		log.Printf("Request from IP: %s", clientIP)

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.ServeHTTP(c.Writer, c.Request)
		log.Printf("Request forwarded to %s\n", server.URL)
		logs.Info(fmt.Sprintf("Request forwarded to %s. Request from IP: %s", server.URL, clientIP))
	})

	fmt.Println("Load Balancer is running on :8080")
	r.Run(":8080")
}
