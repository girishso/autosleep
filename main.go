package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/fsouza/go-dockerclient"
)

var client *docker.Client

func main() {
	endpoint := "unix:///var/run/docker.sock"
	client, _ = docker.NewClient(endpoint)

	http.HandleFunc("/", proxy)
	log.Fatal(http.ListenAndServe(":80", nil))
}

type ContainerPort struct {
	Name string
	Port string
}

func proxy(w http.ResponseWriter, r *http.Request) {

	hostContainer := map[string]ContainerPort{
		"foo.local.info":  ContainerPort{Name: "foo", Port: "3000"},
		"bar.local.info":  ContainerPort{Name: "bar", Port: "3001"},
		"test.local.info": ContainerPort{Name: "test", Port: "3002"},
	}

	// change the request host to match the target
	u, _ := url.Parse("http://127.0.0.1:8080")

	log.Println(hostContainer[r.Host])

	log.Println("check if container is running...", hostContainer[r.Host])
	container, er := client.InspectContainer(hostContainer[r.Host].Name)

	if er != nil {
		log.Println("Error: ", er)
	}

	if !container.State.Running {
		log.Println("starting container: ", hostContainer[r.Host].Name)

		var hostConfig docker.HostConfig

		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
			docker.Port("80/tcp"): {{HostPort: hostContainer[r.Host].Port}},
		}
		if err := client.StartContainer(hostContainer[r.Host].Name, &hostConfig); err != nil {
			log.Println("Error: ", err)
		}
		fmt.Println("started container! :)")
	}

	proxy := http.StripPrefix("", httputil.NewSingleHostReverseProxy(u))
	// You can optionally capture/wrap the transport if that's necessary (for
	// instance, if the transport has been replaced by middleware). Example:
	// proxy.Transport = &myTransport{proxy.Transport}
	// proxy.Transport = &myTransport{}

	proxy.ServeHTTP(w, r)
}
