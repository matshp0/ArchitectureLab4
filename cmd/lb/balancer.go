package main

import (
	"context"
	"flag"
	"fmt"
	"hash/maphash"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/matshp0/ArchitectureLab4/httptools"
	"github.com/matshp0/ArchitectureLab4/signal"
)

var (
	port         = flag.Int("port", 8090, "load balancer port")
	timeoutSec   = flag.Int("timeout-sec", 3, "request timeout time in seconds")
	https        = flag.Bool("https", false, "whether backends support HTTPs")
	traceEnabled = flag.Bool("trace", false, "whether to include tracing information into responses")
)

var (
	timeout     = time.Duration(*timeoutSec) * time.Second
	serversPool = []string{
		"server1:8080",
		"server2:8080",
		"server3:8080",
	}
	seed          = maphash.MakeSeed()
	availablePool []string
)

func removeElement(slice []string, value string) []string {
	var result []string
	for _, item := range slice {
		if item != value {
			result = append(result, item)
		}
	}
	return result
}

func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func scheme() string {
	if *https {
		return "https"
	}
	return "http"
}

func health(dst string) bool {
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	req, _ := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s://%s/health", scheme(), dst), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return true
}

func forward(dst string, rw http.ResponseWriter, r *http.Request) error {
	ctx, _ := context.WithTimeout(r.Context(), timeout)
	fwdRequest := r.Clone(ctx)
	fwdRequest.RequestURI = ""
	fwdRequest.URL.Host = dst
	fwdRequest.URL.Scheme = scheme()
	fwdRequest.Host = dst

	resp, err := http.DefaultClient.Do(fwdRequest)
	if err == nil {
		for k, values := range resp.Header {
			for _, value := range values {
				rw.Header().Add(k, value)
			}
		}
		if *traceEnabled {
			rw.Header().Set("lb-from", dst)
		}
		rw.WriteHeader(resp.StatusCode)
		defer resp.Body.Close()
		_, err := io.Copy(rw, resp.Body)
		if err != nil {
			log.Printf("Failed to write response: %s", err)
		}
		return nil
	} else {
		log.Printf("Failed to get response from %s: %s", dst, err)
		rw.WriteHeader(http.StatusServiceUnavailable)
		return err
	}
}

var frontendHandler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
	var h maphash.Hash
	h.SetSeed(seed)
	h.WriteString(r.URL.Path)
	target := h.Sum64() % uint64(len(availablePool))
	err := forward(availablePool[target], rw, r)
	if err != nil {
		fmt.Println("failed to forward", err)
	}
})

func main() {
	flag.Parse()
	availablePool = make([]string, len(serversPool))
	copy(availablePool, serversPool)

	for _, server := range serversPool {
		server := server
		go func() {
			for range time.Tick(10 * time.Second) {
				available := health(server)
				if available {
					if !contains(availablePool, server) {
						availablePool = append(availablePool, server)
					}
					continue
				}
				availablePool = removeElement(availablePool, server)
			}
		}()
	}

	frontend := httptools.CreateServer(*port, frontendHandler)

	log.Println("Starting load balancer...")
	log.Printf("Tracing support enabled: %t", *traceEnabled)
	frontend.Start()
	signal.WaitForTerminationSignal()
}
