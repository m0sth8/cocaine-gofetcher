package gofetcher

import (
	"github.com/cocaine/cocaine-framework-go/cocaine"
	"github.com/ugorji/go/codec"
	"net/http"
	"io/ioutil"
	"strconv"
	"time"
	"fmt"
	"errors"
	"bytes"
	"io"
	"reflect"
)

const (
	DefaultTimeout = 5000
	DefaultFollowRedirects = true
)

type Gofetcher struct {
	logger *cocaine.Logger
}

type Cookies map[string]string

type Request struct {
	method    string
	url       string
	body	io.Reader
	timeout	int64
	cookies	Cookies
	headers http.Header
	followRedirects bool
}

type responseAndError struct{
	res *http.Response
	err error
}

type Response struct {
	httpResponse	*http.Response
	body	[]byte
	header	http.Header
}

func NewGofetcher() *Gofetcher {
	logger := cocaine.NewLogger()

	gofetcher := Gofetcher{logger}
	return &gofetcher
}

func noRedirect(_ *http.Request, via []*http.Request) error {
	if len(via) > 0 {
		return errors.New("stopped after first redirect")
	}
	return nil
}

func (gofetcher *Gofetcher) performRequest(request *Request) (*Response, error) {
	var (
		err 			error
		httpRequest    *http.Request
		httpResponse   *http.Response
		requestTimeout	time.Duration = time.Duration(request.timeout) * time.Millisecond
	)
	gofetcher.logger.Info(fmt.Sprintf("Request url %s, method %s, timeout %d", request.url, request.method, request.timeout))
	httpClient := &http.Client{}
	if request.followRedirects == false {
		httpClient.CheckRedirect = noRedirect
	}
	httpRequest, err = http.NewRequest(request.method, request.url, request.body)
	if err != nil {
		return nil, err
	}
	for name, value := range request.cookies{
		httpRequest.AddCookie(&http.Cookie{Name:name, Value:value})
	}
	httpRequest.Header = request.headers
	resultChan := make(chan responseAndError)
	go func (){
		res, err := httpClient.Do(httpRequest)
		if err != nil {
			resultChan <- responseAndError{nil, err}
		} else {
			resultChan <- responseAndError{res, err}
		}
	}()
	// http connection stay active after timeout exceeded, cause we can't close it using current client api.
	// Read more about timeouts: https://code.google.com/p/go/issues/detail?id=3362
	//
	select {
		case result := <- resultChan:
			httpResponse, err = result.res, result.err
		case <- time.After(requestTimeout):
			err = errors.New(fmt.Sprintf("Request timeout[%s] exceeded", requestTimeout.String()))
	}
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}
	response := &Response{httpResponse: httpResponse, body: body, header: httpResponse.Header}
	return response, nil

}

// Normal methods

func parseHeaders(rawHeaders map[string]interface {}) http.Header{
	headers := make(http.Header)
	for name, values := range rawHeaders {
		for _, value := range values.([]interface {}) {
			headers.Add(name, string(value.([]uint8))) // to transform in canonical form
		}
	}
	return headers
}

func parseCookies(rawCookie map[string]interface {}) Cookies{
	cookies := Cookies{}
	for key, value := range rawCookie {
		cookies[key] = string(value.([]uint8))
	}
	return cookies
}

func parseTimeout(rawTimeout interface {}) (timeout int64) {
	// is it possible to got timeout in int64 instead of uint64?
	switch rawTimeout.(type) {
	case uint64:
		timeout = int64(rawTimeout.(uint64))
	case int64:
		timeout = rawTimeout.(int64)
	}
	return timeout
}

func (gofetcher *Gofetcher) parseRequest(method string, requestBody []byte) (request *Request){
	var (
		mh codec.MsgpackHandle
		h = &mh
		timeout int64=DefaultTimeout
		cookies Cookies
		headers = make(http.Header)
		followRedirects bool=DefaultFollowRedirects
		body *bytes.Buffer
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

	request = &Request{method: method, url:url, timeout:timeout,
		followRedirects: followRedirects,
		cookies:cookies, headers:headers}
	if body != nil {
		request.body = body
	}
	return request
}

func (gofetcher *Gofetcher) writeResponse(response *cocaine.Response, resp *Response, err error){
	if err != nil {
		response.Write([]interface{}{false, err.Error(), 0, http.Header{}})
	} else{
		response.Write([]interface{}{true, resp.body, resp.httpResponse.StatusCode, resp.header})
	}
	response.Close()
}

func (gofetcher *Gofetcher) handler(method string, request *cocaine.Request, response *cocaine.Response){
	requestBody := <- request.Read()
	httpRequest := gofetcher.parseRequest(method, requestBody)
	resp, err := gofetcher.performRequest(httpRequest)
	gofetcher.writeResponse(response, resp, err)
}

func (gofetcher *Gofetcher) GetHandler(method string) func(request *cocaine.Request, response *cocaine.Response) {
	return func (request *cocaine.Request, response *cocaine.Response){
		gofetcher.handler(method, request, response)
	}
}

// Http methods

func (gofetcher *Gofetcher) writeHttpResponse(response *cocaine.Response, statusCode int,
												data interface{}, headers cocaine.Headers) {
	response.Write(cocaine.WriteHead(statusCode, headers))
	response.Write(data)
	response.Close()
}

func (gofetcher *Gofetcher) HttpProxy(request *cocaine.Request, response *cocaine.Response){
	gofetcher.logger.Info("Http handler requested")
	var (
		timeout int64 = DefaultTimeout
	)
	req, err := cocaine.UnpackProxyRequest(<-request.Read())
	if err != nil {
		gofetcher.writeHttpResponse(response, 500, "Could not unpack request to http request",
			cocaine.Headers{{"Content-Type", "text/html"}})
	} else {
		url := req.FormValue("url")
		timeoutArg := req.FormValue("timeout")
		if timeoutArg != "" {
			tout, _ := strconv.Atoi(timeoutArg)
			timeout = int64(tout)
		}
		httpRequest := Request{method:req.Method, url:url, timeout:timeout,
			followRedirects:DefaultFollowRedirects, headers: req.Header, body: req.Body}
		resp, err := gofetcher.performRequest(&httpRequest)
		if err != nil {
			gofetcher.writeHttpResponse(response, 500, err.Error(), cocaine.Headers{{"Content-Type", "text/html"}})
		} else {
			gofetcher.writeHttpResponse(response, 200, resp.body, cocaine.HttpHeaderToCocaineHeader(resp.header))
		}
	}
}

func (gofetcher *Gofetcher) HttpEcho(request *cocaine.Request, response *cocaine.Response){
	gofetcher.logger.Info("Http handler requested")
	req, err := cocaine.UnpackProxyRequest(<-request.Read())
	if err != nil {
		gofetcher.writeHttpResponse(response, 500, "Could not unpack request to http request",
			cocaine.Headers{{"Content-Type", "text/html"}})
	} else {
		text := req.FormValue("text")
		gofetcher.writeHttpResponse(response, 200, text, cocaine.Headers{{"Content-Type", "text/html"}})
	}
}
