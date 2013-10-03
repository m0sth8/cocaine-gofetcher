package main

import (
	"cocaine"
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
