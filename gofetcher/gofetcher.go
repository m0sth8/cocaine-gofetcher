package gofetcher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/cocaine/cocaine-framework-go/cocaine"
	"github.com/ugorji/go/codec"
)

const (
	DefaultTimeout         = 5000
	DefaultFollowRedirects = true
	KeepAliveTimeout       = 30
	PanicResponseErrorCode = 1
)

// took from httputil/reverseproxy.go
// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

type WarnError struct {
	err error
}

func (s *WarnError) Error() string { return s.err.Error() }

func NewWarn(err error) *WarnError {
	return &WarnError{err: err}
}

type Gofetcher struct {
	Logger    *cocaine.Logger
	Transport http.RoundTripper

	UserAgent string
}

type Cookies map[string]string

type Request struct {
	Method          string
	URL             string
	Body            io.Reader
	Timeout         int64
	Cookies         Cookies
	Headers         http.Header
	FollowRedirects bool
}

type responseAndError struct {
	res *http.Response
	err error
}

type Response struct {
	httpResponse *http.Response
	body         []byte
	header       http.Header
	runtime      time.Duration
}

func NewGofetcher() *Gofetcher {
	logger, err := cocaine.NewLogger()
	if err != nil {
		fmt.Printf("Could not initialize logger due to error: %v", err)
		return nil
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: KeepAliveTimeout * time.Second,
			DualStack: true,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	gofetcher := Gofetcher{logger, transport, ""}
	return &gofetcher
}

func (gofetcher *Gofetcher) SetUserAgent(userAgent string) {
	gofetcher.UserAgent = userAgent
}

type noRedirectError struct{}

func (nr noRedirectError) Error() string {
	return "stopped after first redirect"
}

func noRedirect(_ *http.Request, via []*http.Request) error {
	if len(via) > 0 {
		return noRedirectError{}
	}
	return nil
}

func (gofetcher *Gofetcher) PrepareRequest(request *Request) (*http.Request, *http.Client, error) {
	var (
		err            error
		httpRequest    *http.Request
		requestTimeout time.Duration = time.Duration(request.Timeout) * time.Millisecond
	)

	httpClient := &http.Client{
		Transport: gofetcher.Transport,
		Timeout:   requestTimeout,
	}
	if request.FollowRedirects == false {
		httpClient.CheckRedirect = noRedirect
	}

	httpRequest, err = http.NewRequest(request.Method, request.URL, request.Body)
	if err != nil {
		return nil, nil, err
	}
	for name, value := range request.Cookies {
		httpRequest.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	httpRequest.Header = request.Headers

	// Remove hop-by-hop headers to the backend.  Especially
	// important is "Connection" because we want a persistent
	// connection, regardless of what the client sent to us.  This
	// is modifying the same underlying map from req (shallow
	// copied above) so we only copy it if necessary.
	for _, h := range hopHeaders {
		httpRequest.Header.Del(h)
	}
	httpRequest.Header.Add("Connection", "keep-alive")
	httpRequest.Header.Add("Keep-Alive", fmt.Sprintf("%d", KeepAliveTimeout))

	if gofetcher.UserAgent != "" && len(httpRequest.Header["User-Agent"]) == 0 {
		httpRequest.Header.Set("User-Agent", gofetcher.UserAgent)
	}

	return httpRequest, httpClient, nil
}

func (gofetcher *Gofetcher) ExecuteRequest(req *http.Request, client *http.Client, attempt int) (*http.Response, error) {
	var (
		httpResponse *http.Response
		err          error
	)

	gofetcher.Logger.Infof("Requested url: %s, method: %s, timeout: %d, headers: %v, attempt: %d",
		req.URL.String(), req.Method, client.Timeout, req.Header, attempt)

	resultChan := make(chan responseAndError)
	started := time.Now()
	go func() {
		res, err := client.Do(req)
		resultChan <- responseAndError{res, err}
	}()
	// http connection stay active after timeout exceeded in go <1.3, cause we can't close it using current client api.
	// Read more about timeouts: https://code.google.com/p/go/issues/detail?id=3362
	//
	select {
	case result := <-resultChan:
		httpResponse, err = result.res, result.err
	case <-time.After(client.Timeout):
		err = errors.New(fmt.Sprintf("Request timeout[%s] exceeded", client.Timeout.String()))
		go func() {
			// close httpResponse when it ready
			result := <-resultChan
			if result.res != nil {
				result.res.Body.Close()
			}
		}()
	}
	if err != nil {
		if httpResponse == nil {
			if urlError, ok := err.(*url.Error); ok {
				// golang bug: golang.org/issue/3514
				if urlError.Err == io.EOF {
					gofetcher.Logger.Infof("Got EOF error while loading %s, attempt(%d)", req.URL.String(), attempt)
					if attempt == 1 {
						return gofetcher.ExecuteRequest(req, client, attempt+1)
					}
				}
			}
			return nil, NewWarn(err)
		} else {
			// special case for redirect failure (returns both response and error)
			// read more https://code.google.com/p/go/issues/detail?id=3795
			if urlError, ok := err.(*url.Error); ok {
				if _, ok := urlError.Err.(noRedirectError); ok {
					// when redirect is cancelled response body is closed
					// so put there stub body to not break our clients
					httpResponse.Body = ioutil.NopCloser(bytes.NewBuffer(nil))
				} else {
					// http client failed to redirect and this is not because we requested it to
					// usually it means that server sent response with redirect status code but omitted "Location" header
					return nil, NewWarn(err)
				}
			}
		}
	}

	for _, h := range hopHeaders {
		httpResponse.Header.Del(h)
	}

	runtime := time.Since(started)
	gofetcher.Logger.Info(fmt.Sprintf("Response code: %d, url: %s, runtime: %v",
		httpResponse.StatusCode, req.URL.String(), runtime))
	return httpResponse, nil

}

// Normal methods

func parseHeaders(rawHeaders map[string]interface{}) http.Header {
	headers := make(http.Header)
	for name, values := range rawHeaders {
		for _, value := range values.([]interface{}) {
			headers.Add(name, string(value.([]uint8))) // to transform in canonical form
		}
	}
	return headers
}

func parseCookies(rawCookie map[string]interface{}) Cookies {
	cookies := Cookies{}
	for key, value := range rawCookie {
		cookies[key] = string(value.([]uint8))
	}
	return cookies
}

func parseTimeout(rawTimeout interface{}) (timeout int64) {
	// is it possible to got timeout in int64 instead of uint64?
	switch rawTimeout.(type) {
	case uint64:
		timeout = int64(rawTimeout.(uint64))
	case int64:
		timeout = rawTimeout.(int64)
	}
	return timeout
}

func (gofetcher *Gofetcher) ParseRequest(method string, requestBody []byte) (request *Request) {
	var (
		mh              codec.MsgpackHandle
		h                     = &mh
		timeout         int64 = DefaultTimeout
		cookies         Cookies
		headers              = make(http.Header)
		followRedirects bool = DefaultFollowRedirects
		body            *bytes.Buffer
	)
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))
	var res []interface{}
	codec.NewDecoderBytes(requestBody, h).Decode(&res)
	url := string(res[0].([]uint8))
	switch {
	case method == "GET" || method == "HEAD" || method == "DELETE":
		if len(res) > 1 {
			timeout = parseTimeout(res[1])
		}
		if len(res) > 2 {
			cookies = parseCookies(res[2].(map[string]interface{}))
		}
		if len(res) > 3 {
			headers = parseHeaders(res[3].(map[string]interface{}))
		}
		if len(res) > 4 {
			followRedirects = res[4].(bool)
		}
	case method == "POST" || method == "PUT" || method == "PATCH":
		if len(res) > 1 {
			body = bytes.NewBuffer(res[1].([]byte))
		}
		if len(res) > 2 {
			timeout = parseTimeout(res[2])
		}
		if len(res) > 3 {
			cookies = parseCookies(res[3].(map[string]interface{}))
		}
		if len(res) > 4 {
			headers = parseHeaders(res[4].(map[string]interface{}))
		}
		if len(res) > 5 {
			followRedirects = res[5].(bool)
		}
	}

	request = &Request{Method: method, URL: url, Timeout: timeout,
		FollowRedirects: followRedirects,
		Cookies:         cookies, Headers: headers}
	if body != nil {
		request.Body = body
	}
	return request
}

func (gofetcher *Gofetcher) WriteError(response *cocaine.Response, request *Request, err error) {
	if _, casted := err.(*WarnError); casted {
		gofetcher.Logger.Warnf("Error occured: %v, while downloading %s",
			err.Error(), request.URL)
	} else {
		gofetcher.Logger.Errf("Error occured: %v, while downloading %s",
			err.Error(), request.URL)
	}
	response.Write([]interface{}{false, err.Error(), 0, http.Header{}})
}

func (gofetcher *Gofetcher) WriteResponse(response *cocaine.Response, request *Request, resp *http.Response, body []byte) {
	response.Write([]interface{}{true, body, resp.StatusCode, resp.Header})
}

func (gofetcher *Gofetcher) handler(method string, request *cocaine.Request, response *cocaine.Response) {
	defer func() {
		if r := recover(); r != nil {
			var errMsg string
			if err := r.(*error); err == nil {
				errMsg = "Unknown error has occured."
			} else {
				errMsg = (*err).Error()
			}
			gofetcher.Logger.Errf("Error occured: %s.", errMsg)
			response.ErrorMsg(PanicResponseErrorCode, errMsg)
		}
		response.Close()
	}()

	requestBody := <-request.Read()
	httpRequest := gofetcher.ParseRequest(method, requestBody)

	req, client, err := gofetcher.PrepareRequest(httpRequest)
	if err != nil {
		gofetcher.WriteError(response, httpRequest, err)
		return
	}

	resp, err := gofetcher.ExecuteRequest(req, client, 1)
	if err != nil {
		gofetcher.WriteError(response, httpRequest, err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		gofetcher.WriteError(response, httpRequest, err)
		return
	}

	gofetcher.WriteResponse(response, httpRequest, resp, body)
}

func (gofetcher *Gofetcher) GetHandler(method string) func(request *cocaine.Request, response *cocaine.Response) {
	return func(request *cocaine.Request, response *cocaine.Response) {
		gofetcher.handler(method, request, response)
	}
}

// Http methods

func (gofetcher *Gofetcher) HttpProxy(res http.ResponseWriter, req *http.Request) {
	var (
		timeout int64 = DefaultTimeout
		resp    *http.Response
	)
	url := req.FormValue("url")
	timeoutArg := req.FormValue("timeout")
	if timeoutArg != "" {
		tout, _ := strconv.Atoi(timeoutArg)
		timeout = int64(tout)
	}
	httpRequest := &Request{Method: req.Method, URL: url, Timeout: timeout,
		FollowRedirects: DefaultFollowRedirects, Headers: req.Header, Body: req.Body}
	prepReq, prepClient, err := gofetcher.PrepareRequest(httpRequest)
	if err == nil {
		resp, err = gofetcher.ExecuteRequest(prepReq, prepClient, 1)
	}

	if err != nil {
		res.Header().Set("Content-Type", "text/html")
		res.WriteHeader(500)
		res.Write([]byte(err.Error()))
		if _, casted := err.(*WarnError); casted {
			gofetcher.Logger.Warnf("Gofetcher error: %v", err)
		} else {
			gofetcher.Logger.Errf("Gofetcher error: %v", err)
		}

	} else {
		for key, values := range resp.Header {
			for _, value := range values {
				res.Header().Add(key, value)
			}
		}
		res.WriteHeader(200)
		if _, err := io.Copy(res, resp.Body); err != nil {
			gofetcher.Logger.Errf("Error: %v", err)
		}
	}
}

func (gofetcher *Gofetcher) HttpEcho(res http.ResponseWriter, req *http.Request) {
	gofetcher.Logger.Info("Http echo handler requested")
	text := req.FormValue("text")
	res.Header().Set("Content-Type", "text/html")
	res.WriteHeader(200)
	res.Write([]byte(text))
}
