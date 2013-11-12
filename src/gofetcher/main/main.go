package main

import (
	"github.com/cocaine/cocaine-framework-go/cocaine"
	"github.com/m0sth8/gofetcher"
)

func main(){
	gofetcher := gofetcher.NewGofetcher()
	if gofetcher != nil{
		binds := map[string]cocaine.EventHandler{
			"get": gofetcher.GetHandler("GET"),
			"head": gofetcher.GetHandler("HEAD"),
			"post": gofetcher.GetHandler("POST"),
			"put": gofetcher.GetHandler("PUT"),
			"patch": gofetcher.GetHandler("PATCH"),
			"delete": gofetcher.GetHandler("DELETE"),

			"proxy":      gofetcher.HttpProxy,
		}

		Worker := cocaine.NewWorker()
		Worker.Loop(binds)
	}
}
