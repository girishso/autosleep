package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {

	http.HandleFunc("/", proxy)
	log.Fatal(http.ListenAndServe(":80", nil))
}

func proxy(w http.ResponseWriter, r *http.Request) {

	// change the request host to match the target
	u, _ := url.Parse("http://127.0.0.1:8080")
	log.Println(r.URL.String())
	proxy := http.StripPrefix("", httputil.NewSingleHostReverseProxy(u))
	// You can optionally capture/wrap the transport if that's necessary (for
	// instance, if the transport has been replaced by middleware). Example:
	// proxy.Transport = &myTransport{proxy.Transport}
	// proxy.Transport = &myTransport{}

	proxy.ServeHTTP(w, r)
}
