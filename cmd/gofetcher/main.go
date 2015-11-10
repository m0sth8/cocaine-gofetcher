package main

import (
	"github.com/cocaine/cocaine-framework-go/cocaine"
	"github.com/m0sth8/cocaine-gofetcher/gofetcher"
	"github.com/m0sth8/cocaine-gofetcher/gogen/gofetcher/version"
)

func on_ping(request *cocaine.Request, response *cocaine.Response) {
	defer response.Close()
	response.Write("gofetcher: ping reply")
}

func main(){
	fetcher := gofetcher.NewGofetcher(version.GetAppVersion())
	if fetcher != nil{
		binds := map[string]cocaine.EventHandler{
			"get": fetcher.GetHandler("GET"),
			"head": fetcher.GetHandler("HEAD"),
			"post": fetcher.GetHandler("POST"),
			"put": fetcher.GetHandler("PUT"),
			"patch": fetcher.GetHandler("PATCH"),
			"delete": fetcher.GetHandler("DELETE"),
			"ping": on_ping,
		}


		if worker, err := cocaine.NewWorker(); err == nil{
			worker.Loop(binds)
		}else{
			panic(err)
		}
	}
}

