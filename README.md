cocaine-gofetcher
=================

Urlfetcher for cocaine written on Golang

Supported methods is:

GET, HEAD, DELETE - format is [method(GET|HEAD|DELETE string), timeout(int), cookie(map[string]string), header(map[string][]string), followRedirect(bool,default is true, 10 redirect maximum)]


POST, PUT, PATCH - format is [method(GET|HEAD|DELETE string), body([]byte), timeout(int), cookie(map[string]string), header(map[string][]string), followRedirect(bool,default is true, 10 redirect maximum)]


Run make to build gofetcher application.