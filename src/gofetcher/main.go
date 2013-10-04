package main

import (
	"github.com/cocaine/cocaine-framework-go/cocaine"
//	"encoding/base64"
//	"fmt"
)

var (
	logger *cocaine.Logger
)

func main(){
	logger = cocaine.NewLogger()
	binds := map[string]cocaine.EventHandler{
		"get": Get,
		"httpget":      httpGet,

	}
	Worker := cocaine.NewWorker()
	Worker.Loop(binds)
}

//func main(){
//	resp, err := performRequest(&Request{method:"GET", url:"http://yandex.ru", timeout:10})
//	if err != nil {
//		fmt.Println(err)
//
//	} else {
//		fmt.Println(string(resp.body))
//
//	}
//}


//func Encode(encBuf, bin []byte, e64 *base64.Encoding) []byte {
//	maxEncLen := e64.EncodedLen(len(bin))
//	if encBuf == nil || len(encBuf) < maxEncLen {
//		encBuf = make([]byte, maxEncLen)
//	}
//	e64.Encode(encBuf, bin)
//	return encBuf[0:]
//}
//
//func Decode(decBuf, enc []byte, e64 *base64.Encoding) []byte {
//	maxDecLen := e64.DecodedLen(len(enc))
//	if decBuf == nil || len(decBuf) < maxDecLen {
//		decBuf = make([]byte, maxDecLen)
//	}
//	n, err := e64.Decode(decBuf, enc)
//	_ = err
//	return decBuf[0:n]
//}

//func main(){
//	e64 := base64.URLEncoding
//	enc := []byte("kq9odHRwOi8vaGFici5ydS_NE4g=")
//	dec := Decode(nil, enc, e64)
//	fmt.Println(dec)
//
//	requestBody := dec
//	var (
//		mh codec.MsgpackHandle
//		h = &mh
//	)
//	var res []interface{}
//	codec.NewDecoderBytes(requestBody, h).Decode(&res)
//	url := string(res[0].([]byte))
//	timeout := res[1].(uint64)
//	fmt.Println(url)
//	fmt.Println(timeout)
//}
