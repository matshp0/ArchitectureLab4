package integration

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"net/http"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sdfsdf
const baseAddress = "http://balancer:8090"

var client = http.Client{
	Timeout: 3 * time.Second,
}

var serversPool = []string{
	"server1:8080",
	"server2:8080",
	"server3:8080",
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, v := range input {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

func containsEach(a []string, b []string) bool {
	for _, item := range b {
		if slices.Contains(a, item) {
			continue
		} else {
			return false
		}
	}
	return true
}

func TestBalancer(t *testing.T) {
	if _, exists := os.LookupEnv("INTEGRATION_TEST"); !exists {
		t.Skip("Integration test is not enabled")
	}

	tests := []struct {
		name             string
		endpoint         string
		iterations       int
		serversToRespond int
		description      string
	}{
		{
			name:             "same endpoint 1",
			endpoint:         "/api/v1/some-data",
			iterations:       10,
			serversToRespond: 1,
			description:      "should all come from one server",
		},
		{
			name:             "same endpoint 2",
			endpoint:         "/api/v2/some-data",
			iterations:       10,
			serversToRespond: 1,
			description:      "should all come from one server",
		},
		{
			name:             "different endpoints",
			endpoint:         "",
			iterations:       50,
			serversToRespond: 3,
			description:      "should all come from different servers",
		},
	}

	for _, tt := range tests {
		var senders []string
		for i := range tt.iterations {
			var resp *http.Response
			var err error
			if tt.endpoint == "" {
				fmt.Println(tt.endpoint)
				resp, err = client.Get(fmt.Sprintf("%s/%d", baseAddress, i))
			} else {
				resp, err = client.Get(fmt.Sprintf("%s%s", baseAddress, tt.endpoint))
			}
			if err != nil {
				t.Error(err)
			}
			lb := resp.Header.Get("lb-from")
			senders = append(senders, lb)
		}
		unique := uniqueStrings(senders)
		assert.Equal(t, containsEach(serversPool, unique), true, "should contain servers only from server pool")
		assert.Equal(t, tt.serversToRespond, len(unique), tt.description)
	}
}

func BenchmarkBalancer(b *testing.B) {

	const (
		duration    = 5 * time.Second // Test duration
		concurrency = 5               // Number of concurrent workers
	)

	var (
		successCount uint64
		errorCount   uint64
		wg           sync.WaitGroup
	)

	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	fmt.Printf("Starting load test for %v with %d concurrent workers...\n", duration, concurrency)
	url := fmt.Sprintf("%s/%s", baseAddress, "/api/v1/some-data")
	startTime := time.Now()
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(startTime) < duration {
				resp, err := client.Get(fmt.Sprintf("%s/%d", url, rand.Int()))
				if err != nil {
					atomic.AddUint64(&errorCount, 1)
					continue
				}
				resp.Body.Close()
				if resp.StatusCode < 500 {
					atomic.AddUint64(&successCount, 1)
				} else {
					atomic.AddUint64(&errorCount, 1)
				}
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	b.Log(totalTime)
	b.Logf("Total requests: %d (%.1f RPS)\n", successCount+errorCount, float64(successCount)/totalTime.Seconds())
	b.Logf("Successful requests: %d\n", successCount)
	b.Logf("Failed requests: %d\n", errorCount)
	b.Logf("Error rate: %.2f%%\n", float64(errorCount)*100/float64(successCount+errorCount))
}
