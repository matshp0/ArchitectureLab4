package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRoundTripper struct {
	mock.Mock
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func TestRemoveElement(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		value    string
		expected []string
	}{
		{
			name:     "remove middle element",
			slice:    []string{"a", "b", "c"},
			value:    "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "remove first element",
			slice:    []string{"a", "b", "c"},
			value:    "a",
			expected: []string{"b", "c"},
		},
		{
			name:     "remove last element",
			slice:    []string{"a", "b", "c"},
			value:    "c",
			expected: []string{"a", "b"},
		},
		{
			name:     "element not present",
			slice:    []string{"a", "b", "c"},
			value:    "d",
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeElement(tt.slice, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		value    string
		expected bool
	}{
		{
			name:     "contains element",
			slice:    []string{"a", "b", "c"},
			value:    "b",
			expected: true,
		},
		{
			name:     "does not contain element",
			slice:    []string{"a", "b", "c"},
			value:    "d",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHealth(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	client := &http.Client{
		Transport: mockTransport,
	}

	oldClient := http.DefaultClient
	http.DefaultClient = client
	defer func() { http.DefaultClient = oldClient }()

	tests := []struct {
		name        string
		statusCode  int
		err         error
		expected    bool
		description string
	}{
		{
			name:        "healthy server",
			statusCode:  http.StatusOK,
			err:         nil,
			expected:    true,
			description: "should return true for healthy server",
		},
		{
			name:        "unhealthy server - non-200 status",
			statusCode:  http.StatusInternalServerError,
			err:         nil,
			expected:    false,
			description: "should return false for non-200 status",
		},
		{
			name:        "unhealthy server - request error",
			statusCode:  0,
			err:         http.ErrHandlerTimeout,
			expected:    false,
			description: "should return false when request fails",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			resp := &http.Response{
				StatusCode: tt.statusCode,
			}

			mockTransport.On("RoundTrip", mock.AnythingOfType("*http.Request")).Return(resp, tt.err).Once()

			result := health("test-server:8080")

			assert.Equal(t, tt.expected, result, tt.description)
			mockTransport.AssertExpectations(t)
		})
	}
}

func TestForward(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	client := &http.Client{
		Transport: mockTransport,
		Timeout:   time.Second * 3,
	}

	oldClient := http.DefaultClient
	http.DefaultClient = client
	defer func() { http.DefaultClient = oldClient }()

	tests := []struct {
		name           string
		mockResponse   *http.Response
		mockError      error
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful forward",
			mockResponse: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"X-Test": []string{"value"}},
				Body:       http.NoBody,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "forward error",
			mockResponse:   &http.Response{},
			mockError:      http.ErrHandlerTimeout,
			expectedStatus: http.StatusServiceUnavailable,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockTransport.On("RoundTrip", mock.AnythingOfType("*http.Request")).Return(tt.mockResponse, tt.mockError).Once()

			req := httptest.NewRequest("GET", "http://example.com", nil)
			tt.mockResponse.Request = req
			rec := httptest.NewRecorder()

			err := forward("test-server:8080", rec, req)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedStatus, rec.Result().StatusCode)

			if tt.mockResponse != nil {
				for k, v := range tt.mockResponse.Header {
					assert.Equal(t, v, rec.Result().Header[k])
				}
			}

			mockTransport.AssertExpectations(t)
		})
	}
}
