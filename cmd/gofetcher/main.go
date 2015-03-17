package main

import (
	"github.com/cocaine/cocaine-framework-go/cocaine"
	"github.com/m0sth8/cocaine-gofetcher/gofetcher"
)

func main() {
	fetcher := gofetcher.NewGofetcher()
	if fetcher != nil {
		binds := map[string]cocaine.EventHandler{
			"get":    fetcher.GetHandler("GET"),
			"head":   fetcher.GetHandler("HEAD"),
			"post":   fetcher.GetHandler("POST"),
			"put":    fetcher.GetHandler("PUT"),
			"patch":  fetcher.GetHandler("PATCH"),
			"delete": fetcher.GetHandler("DELETE"),
		}

		worker, err := cocaine.NewWorker()
		if err != nil {
			panic(err)
		}

		worker.Run(binds)
	}
}
