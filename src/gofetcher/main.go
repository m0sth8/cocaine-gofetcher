package main

import (
	"github.com/cocaine/cocaine-framework-go/cocaine"
)

var (
	logger *cocaine.Logger
)

func main(){
	logger = cocaine.NewLogger()
	binds := map[string]cocaine.EventHandler{
		"get": GetHandler("GET"),
		"head": GetHandler("HEAD"),
		"post": GetHandler("POST"),
		"put": GetHandler("PUT"),
		"patch": GetHandler("PATCH"),
		"delete": GetHandler("DELETE"),

		"proxy":      HttpProxy,

	}
	Worker := cocaine.NewWorker()
	Worker.Loop(binds)
}
