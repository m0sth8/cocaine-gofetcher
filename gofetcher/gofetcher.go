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
	"strconv"
	"time"
	"github.com/cocaine/cocaine-framework-go/vendor/src/github.com/ugorji/go/codec"
	"github.com/cocaine/cocaine-framework-go/cocaine"
	"github.com/m0sth8/cocaine-gofetcher/gogen/gofetcher"
	"github.com/m0sth8/cocaine-gofetcher/gogen/gofetcher/get"
	"github.com/m0sth8/cocaine-gofetcher/gogen/gofetcher/post"
	"github.com/m0sth8/cocaine-gofetcher/app/common"
)

const (
	DefaultTimeout         = 5000
	DefaultFollowRedirects = true
	KeepAliveTimeout       = 30
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

var (
	mh codec.MsgpackHandle
	h  = &mh
)

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

func NewGofetcher(appVersion string) *Gofetcher {
	logger, err := cocaine.NewLogger()
	if err != nil {
		fmt.Printf("Could not initialize logger due to error: %v", err)
		return nil
	}
	logger.Debugf(" ********************** Gofetcher with version '" + appVersion + "'")
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

	gofetcher.Logger.Infof("Requested url: %s, method: %s, timeout: %d, headers: %v, attempt: %d, body: %v",
		req.URL.String(), req.Method, client.Timeout, req.Header, attempt, req.Body)

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

/*func (gofetcher *Gofetcher) ParseRequest(method string, requestBody []byte) (request *Request) {
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
}*/

func unpack(buf []byte, unpacked interface{}) error {
	return codec.NewDecoderBytes(buf, h).Decode(unpacked)
}


func (gofetcher *Gofetcher) parseGetRequest(requestBuffer []byte) (get.RequestBundle, get.ResponseBundle) {
	var msg get.RequestBundle
	err := unpack(requestBuffer, &msg)
	if err == nil {
		reply := get.NewResponse(msg.Request.Id)
		reply.Response.ResponseResult.Error.Code = common.UnknownError
		reply.Response.ResponseResult.Error.Message = "Unknown error"
		reply.Response.ResponseResult.ResponseBody.Headers = make(map[string][]string)
		return msg, reply
	} else {
		gofetcher.Logger.Errf("on get: failed to parse request %v", err)
		reply := get.NewResponse("")
		reply.Response.ResponseResult.Error.Code = common.RequestParseError
		reply.Response.ResponseResult.Error.Message = fmt.Sprintf("Failed to parse get-request: %v", err)
		reply.Response.ResponseResult.ResponseBody.Headers = make(map[string][]string)
		return msg, reply
	}
}

func (gofetcher *Gofetcher) parsePostRequest(requestBuffer []byte) (post.RequestBundle, post.ResponseBundle) {
	var msg post.RequestBundle
	err := unpack(requestBuffer, &msg)
	if err == nil {
		reply := post.NewResponse(msg.Request.Id)
		reply.Response.ResponseResult.Error.Code = common.UnknownError
		reply.Response.ResponseResult.Error.Message = "Unknown error"
		reply.Response.ResponseResult.ResponseBody.Headers = make(map[string][]string)
		return msg, reply
	} else {
		gofetcher.Logger.Errf("on post: failed to parse request %v", err)
		reply := post.NewResponse("")
		reply.Response.ResponseResult.Error.Code = common.RequestParseError
		reply.Response.ResponseResult.Error.Message = fmt.Sprintf("Failed to parse post-request: %v", err)
		reply.Response.ResponseResult.ResponseBody.Headers = make(map[string][]string)
		return msg, reply
	}
}

func (gofetcher *Gofetcher) logError(request *Request, err error) {
	if _, casted := err.(*WarnError); casted {
		gofetcher.Logger.Warnf("Error occured: %v, while downloading %s",
			err.Error(), request.URL)
	} else {
		gofetcher.Logger.Errf("Error occured: %v, while downloading %s",
			err.Error(), request.URL)
	}
}

type HandlerArgumentsFactory func (requestBuffer []byte, response *cocaine.Response)(request *Request, defereFunc func()(), setError func(code int, message string)(), setResult func(resp *http.Response, body []byte)())

func (gof *Gofetcher) handler(argumentsFactory HandlerArgumentsFactory, request *cocaine.Request, response *cocaine.Response) {
	requestBuffer := <-request.Read()
	httpRequest, defereFunc, setError, setResult := argumentsFactory(requestBuffer, response)
	/*func (requestBuffer []byte, response *cocaine.Response)(request *Request, defereFunc func()(), setError func(code int, message string)(), setResult func(resp *http.Response, body []byte)()) {
		//gofetcher.logger
		msg, reply := gof.parseGetRequest(requestBuffer)
		request = &Request{Method: "GET", URL: msg.Request.Params.Url, Timeout: msg.Request.Params.Timeout,
			FollowRedirects: msg.Request.Params.FollowRedirects,
			Cookies:         Cookies{}, Headers: make(http.Header)}
		defereFunc = func() {
			if r := recover(); r != nil {
				gof.Logger.Debugf("on_grab_avatar: recovered from: %v.", r)
				reply.Response.ResponseResult.Error.Code = common.UnknownError
				reply.Response.ResponseResult.Error.Message = "Unknown error"
				response.ErrorMsg(gofetcher.SetErrCode(reply.Response.ResponseResult.Error.Code), reply.Response.ResponseResult.Error.Message)
			} else {
				if reply.Response.ResponseResult.Error.Code == common.Ok {
					gof.Logger.Debug("on get: OK, writing response.")
					response.Write(&reply)
				} else {
					gof.Logger.Debug("on get: NOT OK, sending error.")
					response.ErrorMsg(gofetcher.SetErrCode(reply.Response.ResponseResult.Error.Code), reply.Response.ResponseResult.Error.Message)
				}
			}
			gof.Logger.Debugf("on get: RESULT MESSAGE: %s.", reply.Response.ResponseResult.Error.Message)
			gof.Logger.Debug("on get: Closing response.")
			response.Close()
		}
		setError = func(code int, message string) {
			reply.Response.ResponseResult.Error.Code = code
			reply.Response.ResponseResult.Error.Message = message
		}

		setResult = func(resp *http.Response, body []byte) {
			reply.Response.ResponseResult.ResponseBody.Done = true
			reply.Response.ResponseResult.ResponseBody.StatusCode = int64(resp.StatusCode)
			reply.Response.ResponseResult.ResponseBody.Body = body
		}
		return request, defereFunc, setError, setResult
	}(requestBuffer, response)*/
	//msg, reply := gofetcher.parseRequest(requestBuffer)

	defer defereFunc()

	req, client, err := gof.PrepareRequest(httpRequest)
	if err != nil {
		setError(1001, "Failed to prepare request")
		gof.logError(httpRequest, err)
		return
	}

	resp, err := gof.ExecuteRequest(req, client, 1)
	if err != nil {
		setError(1002, "Request failed")
		gof.logError(httpRequest, err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		setError(1003, "Failed to get body from response")
		gof.logError(httpRequest, err)
		return
	}
	gof.Logger.Debug("on get: DONE, Setting OK.")
	setError(common.Ok, "Succeeded")

	setResult(resp, body)
}

func (gof *Gofetcher) GetHandler(method string) func(request *cocaine.Request, response *cocaine.Response) {
	var argumentsFactory HandlerArgumentsFactory

	switch method {
		case "GET": {
			argumentsFactory = func (requestBuffer []byte, response *cocaine.Response)(request *Request, defereFunc func()(), setError func(code int, message string)(), setResult func(resp *http.Response, body []byte)()) {
				//gofetcher.logger
				msg, reply := gof.parseGetRequest(requestBuffer)
				var headers http.Header
				if msg.Request.Params.Headers == nil {
					headers = make(http.Header)
				} else {
					headers = msg.Request.Params.Headers
				}
				request = &Request{Method: "GET", URL: msg.Request.Params.Url, Timeout: msg.Request.Params.Timeout,
					FollowRedirects: msg.Request.Params.FollowRedirects,
					Cookies:         Cookies{}, Headers: headers}
				defereFunc = func() {
					if r := recover(); r != nil {
						gof.Logger.Debugf("on get: recovered from: %v.", r)
						reply.Response.ResponseResult.Error.Code = common.UnknownError
						reply.Response.ResponseResult.Error.Message = "Unknown error"
						response.ErrorMsg(gofetcher.SetErrCode(reply.Response.ResponseResult.Error.Code), reply.Response.ResponseResult.Error.Message)
					} else {
						if reply.Response.ResponseResult.Error.Code == common.Ok {
							gof.Logger.Debug("on get: OK, writing response.")
							response.Write(&reply)
						} else {
							gof.Logger.Debug("on get: NOT OK, sending error.")
							response.ErrorMsg(gofetcher.SetErrCode(reply.Response.ResponseResult.Error.Code), reply.Response.ResponseResult.Error.Message)
						}
					}
					gof.Logger.Debugf("on get: RESULT MESSAGE: %s.", reply.Response.ResponseResult.Error.Message)
					gof.Logger.Debug("on get: Closing response.")
					response.Close()
				}
				setError = func(code int, message string) {
					reply.Response.ResponseResult.Error.Code = code
					reply.Response.ResponseResult.Error.Message = message
				}

				setResult = func(resp *http.Response, body []byte) {
					reply.Response.ResponseResult.ResponseBody.Done = true
					reply.Response.ResponseResult.ResponseBody.StatusCode = int64(resp.StatusCode)
					reply.Response.ResponseResult.ResponseBody.Body = body
				}
				return request, defereFunc, setError, setResult
			}
		}
	case "POST": {
		argumentsFactory = func (requestBuffer []byte, response *cocaine.Response)(request *Request, defereFunc func()(), setError func(code int, message string)(), setResult func(resp *http.Response, body []byte)()) {
			//gofetcher.logger
			msg, reply := gof.parsePostRequest(requestBuffer)
			var headers http.Header
			if msg.Request.Params.Headers == nil {
				headers = make(http.Header)
			} else {
				headers = msg.Request.Params.Headers
			}
			request = &Request{Method: "POST", URL: msg.Request.Params.Url, Timeout: msg.Request.Params.Timeout,
				FollowRedirects: msg.Request.Params.FollowRedirects,
				Cookies:         Cookies{}, Headers: headers}
			if msg.Request.Params.Body != nil {
				request.Body = bytes.NewBuffer(msg.Request.Params.Body)
			}
			defereFunc = func() {
				if r := recover(); r != nil {
					gof.Logger.Debugf("on post: recovered from: %v.", r)
					reply.Response.ResponseResult.Error.Code = common.UnknownError
					reply.Response.ResponseResult.Error.Message = "Unknown error"
					response.ErrorMsg(gofetcher.SetErrCode(reply.Response.ResponseResult.Error.Code), reply.Response.ResponseResult.Error.Message)
				} else {
					if reply.Response.ResponseResult.Error.Code == common.Ok {
						gof.Logger.Debug("on post: OK, writing response.")
						response.Write(&reply)
					} else {
						gof.Logger.Debug("on post: NOT OK, sending error.")
						response.ErrorMsg(gofetcher.SetErrCode(reply.Response.ResponseResult.Error.Code), reply.Response.ResponseResult.Error.Message)
					}
				}
				gof.Logger.Debugf("on post: RESULT MESSAGE: %s.", reply.Response.ResponseResult.Error.Message)
				gof.Logger.Debug("on post: Closing response.")
				response.Close()
			}
			setError = func(code int, message string) {
				reply.Response.ResponseResult.Error.Code = code
				reply.Response.ResponseResult.Error.Message = message
			}

			setResult = func(resp *http.Response, body []byte) {
				reply.Response.ResponseResult.ResponseBody.Done = true
				reply.Response.ResponseResult.ResponseBody.StatusCode = int64(resp.StatusCode)
				reply.Response.ResponseResult.ResponseBody.Body = body
			}
			return request, defereFunc, setError, setResult
		}
	}
	}
	
	return func(request *cocaine.Request, response *cocaine.Response) {
		gof.handler(argumentsFactory, request, response)
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
