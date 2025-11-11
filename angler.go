package angler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
)

var (
	ErrMissingURL        = errors.New("no URL specified")
	ErrMissingHttpMethod = errors.New("no HTTP verb/method specified")
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type SerializationFunc func(any) ([]byte, error)
type DeserializationFunc func(data []byte, v any) error
type RequestOption func(*Request)
type StatusHandlerFunc func(*http.Response) (any, error)

type Request struct {
	method               string
	url                  string
	contentType          string
	headers              map[string]string
	client               HTTPClient
	serialize            SerializationFunc
	deserialize          DeserializationFunc
	statusHandlers       map[int]StatusHandlerFunc
	defaultStatusHandler StatusHandlerFunc
	body                 any
}

// Fetch sends requests, serializes/deserializes bodies, and sets http clients
func Fetch[RT any](options ...RequestOption) (RT, error) {
	var resp RT
	req := &Request{
		contentType: "application/json",
		client:      http.DefaultClient,
		method:      http.MethodGet,
		serialize:   func(body any) ([]byte, error) { return json.Marshal(body) },
		deserialize: func(data []byte, v any) error { return json.Unmarshal(data, v) },
		defaultStatusHandler: func(response *http.Response) (any, error) {
			var reqBody []byte
			if response.Request != nil && response.Request.Body != nil {
				reqBody, _ = io.ReadAll(response.Request.Body)
			}
			respBody, _ := io.ReadAll(response.Body)
			log.Printf("[WARN] HANDLING UNKNOWN STATUS: %q, URL: %q,\n\tREQ_BODY: %q\n\tRESP_BODY: %q", response.Status, response.Request.URL, string(reqBody), string(respBody))
			return resp, nil
		},
	}

	for _, setter := range options {
		setter(req)
	}

	if req.url == "" {
		return resp, ErrMissingURL
	}

	if req.method == "" {
		return resp, ErrMissingHttpMethod
	}

	var reqBody []byte
	var err error

	if req.body != nil {
		reqBody, err = req.serialize(req.body)
		if err != nil {
			return resp, err
		}
	}

	var reqReader io.Reader

	if reqBody != nil {
		reqReader = bytes.NewBuffer(reqBody)
	}

	httpReq, err := http.NewRequest(req.method, req.url, reqReader)
	if err != nil {
		return resp, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for header, headerVal := range req.headers {
		httpReq.Header.Set(header, headerVal)
	}

	response, err := req.client.Do(httpReq)
	if err != nil {
		return resp, err
	}
	defer response.Body.Close()

	if !(response.StatusCode == 200 || response.StatusCode == 201) {
		handle, found := req.statusHandlers[response.StatusCode]
		if !found {
			handle = req.defaultStatusHandler
		}

		b, err := handle(response)
		if err != nil {
			return resp, err
		}
		resp, sameType := b.(RT)
		if !sameType && found {
			return resp, fmt.Errorf("%s HTTP Status handler does not return %T type value", response.Status, resp)
		} else if !sameType && !found {
			return resp, fmt.Errorf("default HTTP Status handler does not return %T type value", resp)
		} else {
			return resp, nil
		}
	}

	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return resp, err
	}

	err = req.deserialize(respBody, &resp)

	return resp, err
}

// RequestOptions functions for all Request struct fields

// WithMethod sets the HTTP method for the request
func WithMethod(method string) RequestOption {
	return func(r *Request) {
		r.method = method
	}
}

// WithURL sets the URL for the request
func WithURL(url string) RequestOption {
	return func(r *Request) {
		r.url = url
	}
}

// WithContentType sets the content type for the request
func WithContentType(contentType string) RequestOption {
	return func(r *Request) {
		r.contentType = contentType
	}
}

// WithHeaders sets the headers for the request
func WithHeaders(headers map[string]string) RequestOption {
	return func(r *Request) {
		r.headers = headers
	}
}

// WithHeader adds a single header to the request
func WithHeader(key, value string) RequestOption {
	return func(r *Request) {
		if r.headers == nil {
			r.headers = make(map[string]string)
		}
		r.headers[key] = value
	}
}

// WithClient sets a custom HTTP client for the request
func WithClient(client HTTPClient) RequestOption {
	return func(r *Request) {
		r.client = client
	}
}

// WithSerialize sets a custom serialization function for the request body
func WithSerialize(serialize SerializationFunc) RequestOption {
	return func(r *Request) {
		r.serialize = serialize
	}
}

// WithDeserialize sets a custom deserialization function for the response body
func WithDeserialize(deserialize DeserializationFunc) RequestOption {
	return func(r *Request) {
		r.deserialize = deserialize
	}
}

// WithStatusHandlers sets the status handlers map for the request
func WithStatusHandlers(handlers map[int]StatusHandlerFunc) RequestOption {
	return func(r *Request) {
		r.statusHandlers = handlers
	}
}

// WithStatusHandler adds a single status handler for a specific status code
func WithStatusHandler(statusCode int, handler StatusHandlerFunc) RequestOption {
	return func(r *Request) {
		if r.statusHandlers == nil {
			r.statusHandlers = make(map[int]StatusHandlerFunc)
		}
		r.statusHandlers[statusCode] = handler
	}
}

// WithDefaultStatusHandler sets the default status handler for unhandled status codes
func WithDefaultStatusHandler(handler StatusHandlerFunc) RequestOption {
	return func(r *Request) {
		r.defaultStatusHandler = handler
	}
}

// WithBody sets the request body
func WithBody(body any) RequestOption {
	return func(r *Request) {
		r.body = body
	}
}
