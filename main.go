package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"

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
	next := atomic.AddInt64(&lb.current, 1) % int64(len(lb.servers))
	return lb.servers[next]
}

func main() {
	serverURLs := []string{
		"http://13.38.70.151:8081",
		"http://15.237.51.177:8082",
	}
	lb := NewLoadBalancer(serverURLs)

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

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.ServeHTTP(c.Writer, c.Request)
		log.Printf("Request forwarded to %s\n", server.URL)
	})

	fmt.Println("Load Balancer is running on :8080")
	r.Run(":8080")
}
